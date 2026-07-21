package workspace

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/outbox"
)

type OutboxRepository interface {
	Claim(context.Context, outbox.ClaimParams) ([]outbox.Event, error)
	MarkDelivered(context.Context, string, string, int) error
	MarkFailed(context.Context, string, string, int, string, time.Duration) error
	MarkDeadLettered(context.Context, string, string, int, string) error
}

type RunnerConfig struct {
	WorkerID         string
	PollInterval     time.Duration
	ReconcileTimeout time.Duration
	MaxAttempts      int
}

type Runner struct {
	logger     *slog.Logger
	outbox     OutboxRepository
	reconciler *Reconciler
	config     RunnerConfig
}

func NewRunner(logger *slog.Logger, outboxRepository OutboxRepository, reconciler *Reconciler, config RunnerConfig) (*Runner, error) {
	if logger == nil || outboxRepository == nil || reconciler == nil {
		return nil, fmt.Errorf("workspace runner dependencies are required")
	}
	if config.WorkerID == "" || config.PollInterval <= 0 || config.ReconcileTimeout <= config.PollInterval {
		return nil, fmt.Errorf("workspace runner configuration is invalid")
	}
	if config.MaxAttempts == 0 {
		config.MaxAttempts = 8
	}
	if config.MaxAttempts < 1 {
		return nil, fmt.Errorf("workspace maximum attempts must be greater than zero")
	}
	return &Runner{logger: logger, outbox: outboxRepository, reconciler: reconciler, config: config}, nil
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
	for _, eventType := range []string{"workspace.created", "workspace.desired-state.updated", "workspace.snapshot.created"} {
		events, err := runner.outbox.Claim(ctx, outbox.ClaimParams{WorkerID: runner.config.WorkerID + ":" + eventType, EventType: eventType, Limit: 1, LeaseDuration: runner.config.ReconcileTimeout + 30*time.Second})
		if err != nil {
			if ctx.Err() == nil {
				runner.logger.Error("claim workspace event", "eventType", eventType, "error", err)
			}
			continue
		}
		for _, event := range events {
			runner.process(ctx, eventType, event)
		}
	}
}

func (runner *Runner) process(ctx context.Context, eventType string, event outbox.Event) {
	reconcileContext, cancel := context.WithTimeout(ctx, runner.config.ReconcileTimeout)
	defer cancel()
	terminal := event.Attempts >= runner.config.MaxAttempts
	err := runner.reconciler.Reconcile(reconcileContext, event, terminal)
	workerID := runner.config.WorkerID + ":" + eventType
	if err == nil {
		if markErr := runner.outbox.MarkDelivered(ctx, event.ID, workerID, event.Attempts); markErr != nil && ctx.Err() == nil {
			runner.logger.Error("mark workspace event delivered", "eventID", event.ID, "error", markErr)
		}
		return
	}
	if terminal {
		if markErr := runner.outbox.MarkDeadLettered(ctx, event.ID, workerID, event.Attempts, err.Error()); markErr != nil && ctx.Err() == nil {
			runner.logger.Error("dead-letter workspace event", "eventID", event.ID, "error", markErr)
		}
		return
	}
	backoff := workspaceRetryBackoff(event.Attempts)
	if markErr := runner.outbox.MarkFailed(ctx, event.ID, workerID, event.Attempts, err.Error(), backoff); markErr != nil && ctx.Err() == nil {
		runner.logger.Error("schedule workspace retry", "eventID", event.ID, "error", markErr)
	}
}

func workspaceRetryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	exponent := attempt - 1
	if exponent > 5 {
		exponent = 5
	}
	return time.Duration(1<<exponent) * time.Second
}
