package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/identity"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/operation"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/outbox"
	platformpostgres "github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/platform/postgres"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/migrations"
)

func TestRepositoryOperationTransactionAndOutboxLifecycle(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adminConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	adminDatabase := stdlib.OpenDB(*adminConfig)
	defer adminDatabase.Close()
	if err := adminDatabase.PingContext(ctx); err != nil {
		t.Fatalf("ping integration PostgreSQL: %v", err)
	}

	randomID, err := identity.NewUUID()
	if err != nil {
		t.Fatalf("generate test schema ID: %v", err)
	}
	schema := "control_plane_test_" + randomID[:8]
	if _, err := adminDatabase.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", schema)); err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = adminDatabase.ExecContext(cleanupContext, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schema))
	})

	testConfig := adminConfig.Copy()
	if testConfig.RuntimeParams == nil {
		testConfig.RuntimeParams = map[string]string{}
	}
	testConfig.RuntimeParams["search_path"] = schema
	database := stdlib.OpenDB(*testConfig)
	defer database.Close()
	if err := platformpostgres.ApplyMigrations(
		ctx,
		database,
		migrations.Files,
		platformpostgres.MigrationOptions{
			LockTimeout:      5 * time.Second,
			StatementTimeout: 10 * time.Second,
		},
	); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}

	repository := NewRepository(database)

	rollbackTransaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin rollback transaction: %v", err)
	}
	rolledBack, err := repository.CreateInTx(ctx, rollbackTransaction, operation.CreateParams{
		Kind:      "instance.create",
		Target:    operation.ResourceRef{Type: "instance", ID: "instance-rollback"},
		Retryable: true,
		RequestID: "rollback-request",
	})
	if err != nil {
		t.Fatalf("CreateInTx() rollback setup error = %v", err)
	}
	var operationCount int
	var eventCount int
	if err := rollbackTransaction.QueryRowContext(
		ctx,
		"SELECT count(*) FROM operations WHERE id = $1",
		rolledBack.ID,
	).Scan(&operationCount); err != nil {
		t.Fatalf("count uncommitted operation: %v", err)
	}
	if err := rollbackTransaction.QueryRowContext(
		ctx,
		"SELECT count(*) FROM outbox_events WHERE aggregate_id = $1",
		rolledBack.ID,
	).Scan(&eventCount); err != nil {
		t.Fatalf("count uncommitted outbox event: %v", err)
	}
	if operationCount != 1 || eventCount != 1 {
		t.Fatalf("uncommitted operation/outbox counts = %d/%d, want 1/1", operationCount, eventCount)
	}
	if err := rollbackTransaction.Rollback(); err != nil {
		t.Fatalf("rollback transaction: %v", err)
	}
	if _, err := repository.GetByID(ctx, rolledBack.ID); !errors.Is(err, operation.ErrNotFound) {
		t.Fatalf("GetByID() after rollback error = %v, want operation.ErrNotFound", err)
	}

	commitTransaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin commit transaction: %v", err)
	}
	created, err := repository.CreateInTx(ctx, commitTransaction, operation.CreateParams{
		Kind:      "instance.create",
		Target:    operation.ResourceRef{Type: "instance", ID: "instance-commit"},
		Retryable: true,
		RequestID: "commit-request",
	})
	if err != nil {
		_ = commitTransaction.Rollback()
		t.Fatalf("CreateInTx() commit setup error = %v", err)
	}
	if err := commitTransaction.Commit(); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}
	loaded, err := repository.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID() after commit error = %v", err)
	}
	if loaded.ID != created.ID || loaded.Status != operation.StatusQueued || loaded.Target.ID != "instance-commit" {
		t.Fatalf("loaded operation = %#v", loaded)
	}

	events, err := repository.Claim(ctx, outbox.ClaimParams{
		WorkerID:      "integration-worker",
		Limit:         10,
		LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if len(events) != 1 || events[0].AggregateID != created.ID || events[0].Attempts != 1 {
		t.Fatalf("first claimed events = %#v", events)
	}
	if _, err := database.ExecContext(
		ctx,
		"UPDATE outbox_events SET locked_until = now() - interval '1 second' WHERE id = $1",
		events[0].ID,
	); err != nil {
		t.Fatalf("expire first claim: %v", err)
	}

	reclaimed, err := repository.Claim(ctx, outbox.ClaimParams{
		WorkerID:      "integration-worker",
		Limit:         10,
		LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("reclaim event: %v", err)
	}
	if len(reclaimed) != 1 || reclaimed[0].Attempts != 2 {
		t.Fatalf("reclaimed events = %#v, want generation 2", reclaimed)
	}
	if err := repository.MarkDelivered(ctx, reclaimed[0].ID, "integration-worker", 1); !errors.Is(err, outbox.ErrNotFound) {
		t.Fatalf("stale MarkDelivered() error = %v, want outbox.ErrNotFound", err)
	}

	failure := strings.Repeat("\u754c", maxOutboxErrorRunes+1)
	if err := repository.MarkFailed(
		ctx,
		reclaimed[0].ID,
		"integration-worker",
		reclaimed[0].Attempts,
		failure,
		time.Millisecond,
	); err != nil {
		t.Fatalf("MarkFailed() error = %v", err)
	}
	var recordedFailure string
	if err := database.QueryRowContext(
		ctx,
		"SELECT last_error FROM outbox_events WHERE id = $1",
		reclaimed[0].ID,
	).Scan(&recordedFailure); err != nil {
		t.Fatalf("read truncated outbox failure: %v", err)
	}
	if got := len([]rune(recordedFailure)); got != maxOutboxErrorRunes {
		t.Fatalf("failure rune count = %d, want %d", got, maxOutboxErrorRunes)
	}
	if _, err := database.ExecContext(
		ctx,
		"UPDATE outbox_events SET available_at = now() - interval '1 second' WHERE id = $1",
		reclaimed[0].ID,
	); err != nil {
		t.Fatalf("make failed event available: %v", err)
	}

	retried, err := repository.Claim(ctx, outbox.ClaimParams{
		WorkerID:      "integration-worker",
		Limit:         10,
		LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("claim retry: %v", err)
	}
	if len(retried) != 1 || retried[0].Attempts != 3 {
		t.Fatalf("retried events = %#v, want generation 3", retried)
	}
	if err := repository.MarkDelivered(
		ctx,
		retried[0].ID,
		"integration-worker",
		retried[0].Attempts,
	); err != nil {
		t.Fatalf("MarkDelivered() error = %v", err)
	}

	deadLetterOperation, err := repository.Create(ctx, operation.CreateParams{
		Kind:      "instance.delete",
		Target:    operation.ResourceRef{Type: "instance", ID: "instance-dead-letter"},
		Retryable: true,
		RequestID: "dead-letter-request",
	})
	if err != nil {
		t.Fatalf("Create() dead-letter setup error = %v", err)
	}
	deadLetterEvents, err := repository.Claim(ctx, outbox.ClaimParams{
		WorkerID:      "dead-letter-worker",
		Limit:         10,
		LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("claim dead-letter setup event: %v", err)
	}
	if len(deadLetterEvents) != 1 || deadLetterEvents[0].AggregateID != deadLetterOperation.ID {
		t.Fatalf("dead-letter setup events = %#v", deadLetterEvents)
	}
	if err := repository.MarkDeadLettered(
		ctx,
		deadLetterEvents[0].ID,
		"dead-letter-worker",
		deadLetterEvents[0].Attempts,
		"maximum attempts reached",
	); err != nil {
		t.Fatalf("MarkDeadLettered() error = %v", err)
	}
	remaining, err := repository.Claim(ctx, outbox.ClaimParams{
		WorkerID:      "dead-letter-worker",
		Limit:         10,
		LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("claim after dead-letter: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("claim after dead-letter = %#v, want no events", remaining)
	}
}

func TestRepositoryRejectsSubMillisecondOutboxDurations(t *testing.T) {
	repository := NewRepository(nil)
	if _, err := repository.Claim(context.Background(), outbox.ClaimParams{
		WorkerID:      "worker",
		Limit:         1,
		LeaseDuration: time.Microsecond,
	}); err == nil {
		t.Fatal("Claim() error = nil, want minimum lease validation error")
	}
	if err := repository.MarkFailed(
		context.Background(),
		"00000000-0000-4000-8000-000000000001",
		"worker",
		1,
		"failure",
		time.Microsecond,
	); err == nil {
		t.Fatal("MarkFailed() error = nil, want minimum backoff validation error")
	}
}
