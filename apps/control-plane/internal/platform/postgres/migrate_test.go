package postgres

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestDiscoverMigrationsOrdersUpFiles(t *testing.T) {
	files := fstest.MapFS{
		"000002_second.up.sql":  {Data: []byte("SELECT 2")},
		"000001_first.up.sql":   {Data: []byte("SELECT 1")},
		"000001_first.down.sql": {Data: []byte("SELECT 0")},
		"README.md":             {Data: []byte("ignored")},
	}

	items, err := discoverMigrations(files)
	if err != nil {
		t.Fatalf("discoverMigrations() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].version != "000001_first" || items[1].version != "000002_second" {
		t.Fatalf("versions = %q, %q", items[0].version, items[1].version)
	}
	expected := sha256.Sum256([]byte("SELECT 1"))
	if items[0].checksum != hex.EncodeToString(expected[:]) {
		t.Fatalf("checksum = %q, want %q", items[0].checksum, hex.EncodeToString(expected[:]))
	}
	if string(items[0].contents) != "SELECT 1" {
		t.Fatalf("contents = %q, want SELECT 1", items[0].contents)
	}
}

func TestDiscoverMigrationsNormalizesLineEndings(t *testing.T) {
	variants := map[string][]byte{
		"lf":   []byte("SELECT 1;\nSELECT 2;\n"),
		"crlf": []byte("SELECT 1;\r\nSELECT 2;\r\n"),
		"cr":   []byte("SELECT 1;\rSELECT 2;\r"),
	}

	var expectedChecksum string
	for name, contents := range variants {
		t.Run(name, func(t *testing.T) {
			items, err := discoverMigrations(fstest.MapFS{
				"000001_first.up.sql": {Data: contents},
			})
			if err != nil {
				t.Fatalf("discoverMigrations() error = %v", err)
			}
			if got := string(items[0].contents); got != "SELECT 1;\nSELECT 2;\n" {
				t.Fatalf("normalized contents = %q", got)
			}
			if expectedChecksum == "" {
				expectedChecksum = items[0].checksum
			}
			if items[0].checksum != expectedChecksum {
				t.Fatalf("checksum = %q, want %q", items[0].checksum, expectedChecksum)
			}
		})
	}
}

func TestValidateMigrationOptionsRequiresMillisecondTimeouts(t *testing.T) {
	valid := MigrationOptions{LockTimeout: time.Millisecond, StatementTimeout: time.Millisecond}
	if err := validateMigrationOptions(valid); err != nil {
		t.Fatalf("valid options error = %v", err)
	}
	if err := validateMigrationOptions(MigrationOptions{
		LockTimeout: time.Microsecond, StatementTimeout: time.Second,
	}); err == nil {
		t.Fatal("sub-millisecond lock timeout error = nil")
	}
	if err := validateMigrationOptions(MigrationOptions{
		LockTimeout: time.Second, StatementTimeout: time.Microsecond,
	}); err == nil {
		t.Fatal("sub-millisecond statement timeout error = nil")
	}
}

func TestValidateAppliedMigrationDetectsDrift(t *testing.T) {
	if err := validateAppliedMigration("000001_first", sql.NullString{String: "same", Valid: true}, "same"); err != nil {
		t.Fatalf("matching checksum error = %v", err)
	}
	err := validateAppliedMigration("000001_first", sql.NullString{String: "old", Valid: true}, "new")
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("drift error = %v, want checksum mismatch", err)
	}
}

func TestValidateAppliedMigrationRejectsMissingChecksum(t *testing.T) {
	err := validateAppliedMigration("000001_first", sql.NullString{}, "new")
	if err == nil || !strings.Contains(err.Error(), "no recorded checksum") {
		t.Fatalf("missing checksum error = %v", err)
	}
}
