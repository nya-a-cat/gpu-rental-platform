package inventorysync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/catalog"
)

type Synchronizer interface {
	Sync(context.Context, catalog.Cluster) error
}

type RunnerConfig struct {
	PollInterval time.Duration
	SyncTimeout  time.Duration
}

type Runner struct {
	logger       *slog.Logger
	repository   Repository
	synchronizer Synchronizer
	config       RunnerConfig
}

func NewRunner(logger *slog.Logger, repository Repository, synchronizer Synchronizer, config RunnerConfig) (*Runner, error) {
	if logger == nil || repository == nil || synchronizer == nil {
		return nil, errors.New("inventory sync runner dependencies are required")
	}
	if config.PollInterval <= 0 || config.SyncTimeout <= 0 {
		return nil, errors.New("inventory sync runner intervals must be greater than zero")
	}
	return &Runner{logger: logger, repository: repository, synchronizer: synchronizer, config: config}, nil
}

func (runner *Runner) Run(ctx context.Context) {
	ticker := time.NewTicker(runner.config.PollInterval)
	defer ticker.Stop()
	for {
		runner.poll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (runner *Runner) poll(ctx context.Context) {
	clusters, err := runner.repository.ListClusters(ctx)
	if err != nil {
		if ctx.Err() == nil {
			runner.logger.Error("list registered clusters for inventory sync", "error", err)
		}
		return
	}
	for _, cluster := range clusters {
		if ctx.Err() != nil {
			return
		}
		syncContext, cancel := context.WithTimeout(ctx, runner.config.SyncTimeout)
		err := runner.synchronizer.Sync(syncContext, cluster)
		cancel()
		if err != nil && ctx.Err() == nil {
			runner.logger.Warn("synchronize cluster inventory", "cluster", cluster.ManagedClusterName, "error", fmt.Sprint(err))
		}
	}
}
