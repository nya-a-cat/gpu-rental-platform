package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	migrationLockName      = "gpu-cloud-control-plane-schema-migrations"
	migrationUnlockTimeout = 3 * time.Second
)

type MigrationOptions struct {
	LockTimeout      time.Duration
	StatementTimeout time.Duration
}

type migration struct {
	version  string
	path     string
	contents []byte
	checksum string
}

func ApplyMigrations(
	ctx context.Context,
	database *sql.DB,
	files fs.FS,
	options MigrationOptions,
) (returnErr error) {
	if err := validateMigrationOptions(options); err != nil {
		return err
	}
	ordered, err := discoverMigrations(files)
	if err != nil {
		return err
	}

	connection, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve migration connection: %w", err)
	}
	defer connection.Close()

	if err := configureMigrationSession(ctx, connection, options); err != nil {
		return err
	}
	lockHeld := false
	defer func() {
		releaseContext, cancelRelease := context.WithTimeout(context.Background(), migrationUnlockTimeout)
		defer cancelRelease()
		if err := releaseMigrationSession(releaseContext, connection, lockHeld); err != nil && returnErr == nil {
			returnErr = err
		}
	}()
	if _, err := connection.ExecContext(ctx, "SELECT pg_advisory_lock(hashtext($1))", migrationLockName); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	lockHeld = true

	if _, err := connection.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS control_plane_schema_migrations (
  version text PRIMARY KEY,
  checksum text NOT NULL,
  applied_at timestamptz NOT NULL DEFAULT now()
)`); err != nil {
		return fmt.Errorf("create schema migrations table: %w", err)
	}
	if _, err := connection.ExecContext(ctx,
		"ALTER TABLE control_plane_schema_migrations ADD COLUMN IF NOT EXISTS checksum text",
	); err != nil {
		return fmt.Errorf("ensure schema migration checksum column: %w", err)
	}

	for _, item := range ordered {
		var recordedChecksum sql.NullString
		err := connection.QueryRowContext(ctx,
			"SELECT checksum FROM control_plane_schema_migrations WHERE version = $1",
			item.version,
		).Scan(&recordedChecksum)
		if err == nil {
			if err := validateAppliedMigration(item.version, recordedChecksum, item.checksum); err != nil {
				return err
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check migration %s: %w", item.version, err)
		}

		transaction, err := connection.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", item.version, err)
		}
		if _, err := transaction.ExecContext(ctx, string(item.contents)); err != nil {
			transaction.Rollback()
			return fmt.Errorf("apply migration %s: %w", item.version, err)
		}
		if _, err := transaction.ExecContext(ctx,
			"INSERT INTO control_plane_schema_migrations (version, checksum) VALUES ($1, $2)",
			item.version,
			item.checksum,
		); err != nil {
			transaction.Rollback()
			return fmt.Errorf("record migration %s: %w", item.version, err)
		}
		if err := transaction.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", item.version, err)
		}
	}
	if _, err := connection.ExecContext(ctx,
		"ALTER TABLE control_plane_schema_migrations ALTER COLUMN checksum SET NOT NULL",
	); err != nil {
		return fmt.Errorf("enforce schema migration checksums: %w", err)
	}
	return nil
}

func validateMigrationOptions(options MigrationOptions) error {
	if options.LockTimeout < time.Millisecond {
		return errors.New("migration lock timeout must be at least 1ms")
	}
	if options.StatementTimeout < time.Millisecond {
		return errors.New("migration statement timeout must be at least 1ms")
	}
	return nil
}

func configureMigrationSession(
	ctx context.Context,
	connection *sql.Conn,
	options MigrationOptions,
) error {
	if _, err := connection.ExecContext(ctx, `
SELECT
  set_config('lock_timeout', $1, false),
  set_config('statement_timeout', $2, false)`,
		postgresTimeoutValue(options.LockTimeout),
		postgresTimeoutValue(options.StatementTimeout),
	); err != nil {
		return fmt.Errorf("configure migration timeouts: %w", err)
	}
	return nil
}

func releaseMigrationSession(ctx context.Context, connection *sql.Conn, lockHeld bool) error {
	var unlocked bool
	var unlockErr error
	if lockHeld {
		unlockErr = connection.QueryRowContext(
			ctx,
			"SELECT pg_advisory_unlock(hashtext($1))",
			migrationLockName,
		).Scan(&unlocked)
	}
	_, resetErr := connection.ExecContext(ctx, `
SELECT
  set_config('lock_timeout', '0', false),
  set_config('statement_timeout', '0', false)`)

	var result error
	if unlockErr != nil {
		result = errors.Join(result, fmt.Errorf("release migration lock: %w", unlockErr))
	} else if lockHeld && !unlocked {
		result = errors.Join(result, errors.New("release migration lock: lock was not held by the migration session"))
	}
	if resetErr != nil {
		result = errors.Join(result, fmt.Errorf("reset migration timeouts: %w", resetErr))
	}
	return result
}

func postgresTimeoutValue(value time.Duration) string {
	return strconv.FormatInt(value.Milliseconds(), 10) + "ms"
}

func discoverMigrations(files fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	result := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		version := strings.TrimSuffix(entry.Name(), ".up.sql")
		if version == "" {
			return nil, fmt.Errorf("migration %q has no version", entry.Name())
		}
		contents, err := fs.ReadFile(files, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		contents = normalizeSQLLineEndings(contents)
		sum := sha256.Sum256(contents)
		result = append(result, migration{
			version:  version,
			path:     entry.Name(),
			contents: contents,
			checksum: hex.EncodeToString(sum[:]),
		})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no .up.sql migrations found")
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].version < result[right].version
	})
	return result, nil
}

func normalizeSQLLineEndings(contents []byte) []byte {
	normalized := bytes.ReplaceAll(contents, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(normalized, []byte("\r"), []byte("\n"))
}

func validateAppliedMigration(version string, recorded sql.NullString, current string) error {
	if !recorded.Valid || recorded.String == "" {
		return fmt.Errorf("migration %s has no recorded checksum; manual migration review is required", version)
	}
	if recorded.String != current {
		return fmt.Errorf(
			"migration %s checksum mismatch: database=%s source=%s",
			version,
			recorded.String,
			current,
		)
	}
	return nil
}
