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

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/authn"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/authorization"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/config"
	fleetocm "github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/fleet/ocm"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/httpapi"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/inventorysync"
	platformpostgres "github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/platform/postgres"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/sharedisolation"
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
	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	capabilities := []string{"operations", "transactional-outbox", "audit-foundation", "engine-ports", "agent-health-policy", "tenancy", "postgres-rbac", "quota-reservations", "resource-catalog", "placement-inventory", "gpu-workspace-api"}
	var isolationRunner *sharedisolation.Runner
	var inventoryRunner *inventorysync.Runner
	if cfg.OCM.Enabled {
		fleetManager, err := fleetocm.NewClient(fleetocm.Config{
			HubURL:         cfg.OCM.HubURL,
			TokenFile:      cfg.OCM.TokenFile,
			CAFile:         cfg.OCM.CAFile,
			ClientCertFile: cfg.OCM.ClientCertFile,
			ClientKeyFile:  cfg.OCM.ClientKeyFile,
			PollInterval:   cfg.OCM.PollInterval,
		})
		if err != nil {
			logger.Error("initialize OCM fleet manager", "error", err)
			os.Exit(1)
		}
		reconciler, err := sharedisolation.NewReconciler(
			repository,
			fleetManager,
			cfg.OCM.DefaultClusterID,
			cfg.OCM.AddonInstallNamespace,
			cfg.OCM.AddonServiceAccount,
		)
		if err != nil {
			logger.Error("initialize shared-isolation reconciler", "error", err)
			os.Exit(1)
		}
		workerID := cfg.ServiceName
		if hostname, hostnameErr := os.Hostname(); hostnameErr == nil && hostname != "" {
			workerID += ":" + hostname
		}
		isolationRunner, err = sharedisolation.NewRunner(logger, repository, reconciler, sharedisolation.RunnerConfig{
			WorkerID:         workerID,
			PollInterval:     cfg.OCM.PollInterval,
			ReconcileTimeout: cfg.OCM.ReconcileTimeout,
			MaxAttempts:      cfg.OCM.MaxAttempts,
		})
		if err != nil {
			logger.Error("initialize shared-isolation runner", "error", err)
			os.Exit(1)
		}
		inventoryReconciler, err := inventorysync.NewReconciler(repository, fleetManager)
		if err != nil {
			logger.Error("initialize inventory reconciler", "error", err)
			os.Exit(1)
		}
		inventoryRunner, err = inventorysync.NewRunner(logger, repository, inventoryReconciler, inventorysync.RunnerConfig{
			PollInterval: cfg.OCM.PollInterval,
			SyncTimeout:  cfg.OCM.ReconcileTimeout,
		})
		if err != nil {
			logger.Error("initialize inventory sync runner", "error", err)
			os.Exit(1)
		}
		capabilities = append(capabilities, "ocm-manifestwork", "shared-project-isolation", "gpu-inventory-sync")
	}

	var authenticator authn.Authenticator
	if cfg.BreakGlassAdminToken != "" {
		authenticator, err = authn.NewBreakGlassAuthenticator(cfg.BreakGlassAdminSubject, cfg.BreakGlassAdminToken)
		if err != nil {
			logger.Error("invalid break-glass authentication configuration", "error", err)
			os.Exit(1)
		}
	} else {
		logger.Warn("authenticated tenancy API is disabled because BREAK_GLASS_ADMIN_TOKEN is unset")
	}
	authorizationEngine := authorization.NewPostgresEngine(database)
	handler := httpapi.NewHandler(httpapi.Dependencies{
		Logger:           logger,
		Readiness:        database,
		Operations:       repository,
		Tenancy:          repository,
		Catalog:          repository,
		Workspace:        repository,
		Authenticator:    authenticator,
		Authorization:    authorizationEngine,
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
			Capabilities: capabilities,
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

	if isolationRunner != nil {
		go isolationRunner.Run(shutdownContext)
		logger.Info("shared-isolation reconciler started", "managedCluster", cfg.OCM.DefaultClusterID)
	}
	if inventoryRunner != nil {
		go inventoryRunner.Run(shutdownContext)
		logger.Info("GPU inventory synchronizer started", "pollInterval", cfg.OCM.PollInterval.String())
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
