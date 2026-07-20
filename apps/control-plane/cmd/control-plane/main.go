package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/config"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/httpapi"
	platformpostgres "github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/platform/postgres"
	storagepostgres "github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/storage/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid control-plane configuration", "error", err)
		os.Exit(1)
	}

	startupContext, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	database, err := platformpostgres.Open(startupContext, cfg)
	cancelStartup()
	if err != nil {
		logger.Error("PostgreSQL startup check failed", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	repository := storagepostgres.NewRepository(database)
	handler := httpapi.NewHandler(httpapi.Dependencies{
		Logger:           logger,
		Readiness:        database,
		Operations:       repository,
		ReadinessTimeout: cfg.ReadinessTimeout,
		Info: httpapi.SystemInfo{
			Product:      "gpu-container-cloud",
			APIVersion:   "v1",
			Version:      cfg.Version,
			Commit:       cfg.Commit,
			Stage:        cfg.Stage,
			Architecture: "modular-monolith",
			Persistence:  "postgresql",
			AgentHealthPolicy: httpapi.AgentHealthPolicy{
				HeartbeatIntervalSeconds: float64(cfg.AgentHealthPolicy.HeartbeatInterval) / float64(time.Second),
				DegradedAfterSeconds:     float64(cfg.AgentHealthPolicy.DegradedAfter) / float64(time.Second),
				OfflineAfterSeconds:      float64(cfg.AgentHealthPolicy.OfflineAfter) / float64(time.Second),
			},
			Capabilities: []string{"operations", "transactional-outbox", "audit-foundation", "engine-ports", "agent-health-policy"},
		},
	})
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("GPU cloud control plane started",
			"address", cfg.HTTPAddr,
			"version", cfg.Version,
			"commit", cfg.Commit,
		)
		serverErrors <- server.ListenAndServe()
	}()

	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-shutdownContext.Done():
		logger.Info("control-plane shutdown requested")
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("control-plane HTTP server failed", "error", err)
			os.Exit(1)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("control-plane graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("GPU cloud control plane stopped")
}
