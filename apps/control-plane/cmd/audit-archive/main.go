package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/auditarchive"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	now := time.Now().UTC()
	cfg, err := auditarchive.LoadRuntimeConfig(os.Getenv, now)
	if err != nil {
		logger.Error("invalid audit archive configuration", "error", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.CommandTimeout)
	defer cancel()

	database, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("create audit archive database pool", "error", err)
		os.Exit(1)
	}
	defer database.Close()
	if err := database.Ping(ctx); err != nil {
		logger.Error("audit archive database readiness failed", "error", err)
		os.Exit(1)
	}
	store, err := auditarchive.NewS3Store(auditarchive.S3Config{
		Endpoint:  cfg.Endpoint,
		AccessKey: cfg.AccessKey,
		SecretKey: cfg.SecretKey,
		Region:    cfg.Region,
	})
	if err != nil {
		logger.Error("create audit archive S3 client", "error", err)
		os.Exit(1)
	}
	result, err := auditarchive.Archive(ctx, cfg.Archive, auditarchive.NewPostgresSource(database), store, now)
	if err != nil {
		logger.Error("audit archive failed", "error", err)
		os.Exit(1)
	}
	logger.Info("audit archive completed",
		"status", result.Status,
		"objectKey", result.ObjectKey,
		"rows", result.Rows,
		"bytes", result.Bytes,
		"sha256", result.SHA256,
		"periodStart", result.PeriodStart.Format(time.RFC3339),
		"periodEnd", result.PeriodEnd.Format(time.RFC3339),
		"retentionUntil", result.RetentionUntil.Format(time.RFC3339),
	)
}
