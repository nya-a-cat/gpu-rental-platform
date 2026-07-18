package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/config"
	platformpostgres "github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/platform/postgres"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/migrations"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	args := os.Args[1:]
	if len(args) > 1 || (len(args) == 1 && args[0] != "up") {
		logger.Error("unsupported migrate command", "usage", "control-plane-migrate [up]")
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid migration configuration", "error", err)
		os.Exit(1)
	}

	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	migrationContext, cancelMigration := context.WithTimeout(signalContext, cfg.MigrationTimeout)
	defer cancelMigration()

	database, err := platformpostgres.Open(migrationContext, cfg)
	if err != nil {
		logger.Error("PostgreSQL startup check failed", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := platformpostgres.ApplyMigrations(
		migrationContext,
		database,
		migrations.Files,
		platformpostgres.MigrationOptions{
			LockTimeout:      cfg.MigrationLockTimeout,
			StatementTimeout: cfg.MigrationStatementTimeout,
		},
	); err != nil {
		logger.Error("control-plane migration failed", "error", err)
		os.Exit(1)
	}
	logger.Info("control-plane migrations applied")
}
