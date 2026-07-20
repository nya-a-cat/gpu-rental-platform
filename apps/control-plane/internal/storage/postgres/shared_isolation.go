package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/operation"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/sharedisolation"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

func (repository *Repository) LoadProjectForIsolation(
	ctx context.Context,
	projectID string,
) (sharedisolation.ProjectSnapshot, error) {
	project, err := repository.GetProject(ctx, projectID)
	if err != nil {
		return sharedisolation.ProjectSnapshot{}, err
	}
	quota, err := repository.GetQuota(ctx, projectID, sharedisolation.GPUQuotaResourceClass)
	if errors.Is(err, tenancy.ErrNotFound) {
		return sharedisolation.ProjectSnapshot{Project: project}, nil
	}
	if err != nil {
		return sharedisolation.ProjectSnapshot{}, fmt.Errorf("get project GPU quota: %w", err)
	}
	return sharedisolation.ProjectSnapshot{Project: project, GPUQuota: quota.HardLimit}, nil
}

func (repository *Repository) StartSharedIsolation(
	ctx context.Context,
	state sharedisolation.ReconcileState,
) error {
	now := repository.now().UTC()
	condition := tenancy.Condition{
		Type:               "SharedIsolationReady",
		Status:             "False",
		Reason:             "ManifestWorkApplying",
		Message:            fmt.Sprintf("Applying shared isolation through ManifestWork %s on managed cluster %s.", state.WorkName, state.ClusterID),
		LastTransitionTime: now,
	}
	step := operation.Step{
		Name:      "apply-shared-isolation",
		Status:    operation.StatusRunning,
		Progress:  25,
		StartedAt: &now,
		Detail:    "Applying Namespace, RBAC, ResourceQuota, NetworkPolicy and Restricted Pod Security.",
	}
	return repository.writeSharedIsolationState(ctx, state, sharedIsolationWrite{
		observedState:      "pending",
		provisioningState:  "provisioning",
		condition:          condition,
		operationStatus:    operation.StatusRunning,
		operationStep:      step,
		operationProgress:  25,
		operationRetryable: true,
	})
}

func (repository *Repository) CompleteSharedIsolation(
	ctx context.Context,
	state sharedisolation.ReconcileState,
) error {
	now := repository.now().UTC()
	condition := tenancy.Condition{
		Type:               "SharedIsolationReady",
		Status:             "True",
		Reason:             "ManifestWorkAvailable",
		Message:            fmt.Sprintf("Shared isolation is active through ManifestWork %s on managed cluster %s.", state.WorkName, state.ClusterID),
		LastTransitionTime: now,
	}
	step := operation.Step{
		Name:       "apply-shared-isolation",
		Status:     operation.StatusSucceeded,
		Progress:   100,
		StartedAt:  &now,
		FinishedAt: &now,
		Detail:     "The managed cluster reports the shared-isolation ManifestWork as Applied and Available.",
	}
	return repository.writeSharedIsolationState(ctx, state, sharedIsolationWrite{
		observedState:      "active",
		provisioningState:  "succeeded",
		condition:          condition,
		observedGeneration: state.ProjectGeneration,
		appliedGPUQuota:    state.GPUQuota,
		operationStatus:    operation.StatusSucceeded,
		operationStep:      step,
		operationProgress:  100,
		operationRetryable: false,
		finishedAt:         &now,
	})
}

func (repository *Repository) FailSharedIsolation(
	ctx context.Context,
	state sharedisolation.ReconcileState,
	failure error,
	terminal bool,
) error {
	now := repository.now().UTC()
	message := truncateRunes(failure.Error(), 2048)
	condition := tenancy.Condition{
		Type:               "SharedIsolationReady",
		Status:             "False",
		Reason:             "ManifestWorkReconcileFailed",
		Message:            message,
		LastTransitionTime: now,
	}
	step := operation.Step{
		Name:       "apply-shared-isolation",
		Status:     operation.StatusFailed,
		Progress:   100,
		StartedAt:  &now,
		FinishedAt: &now,
		Detail:     message,
	}
	structuredError := &operation.StructuredError{
		Code:    "shared_isolation_reconcile_failed",
		Message: "Shared isolation reconciliation failed.",
		Detail:  message,
		Fields: map[string]any{
			"clusterId":    state.ClusterID,
			"manifestWork": state.WorkName,
		},
	}
	return repository.writeSharedIsolationState(ctx, state, sharedIsolationWrite{
		observedState:      "unknown",
		provisioningState:  "failed",
		condition:          condition,
		operationStatus:    operation.StatusFailed,
		operationStep:      step,
		operationProgress:  100,
		operationRetryable: !terminal,
		operationError:     structuredError,
		finishedAt:         &now,
	})
}

type sharedIsolationWrite struct {
	observedState      string
	provisioningState  string
	condition          tenancy.Condition
	observedGeneration int64
	appliedGPUQuota    int64
	operationStatus    operation.Status
	operationStep      operation.Step
	operationProgress  int
	operationRetryable bool
	operationError     *operation.StructuredError
	finishedAt         *time.Time
}

func (repository *Repository) writeSharedIsolationState(
	ctx context.Context,
	state sharedisolation.ReconcileState,
	write sharedIsolationWrite,
) error {
	conditions, err := json.Marshal([]tenancy.Condition{write.condition})
	if err != nil {
		return fmt.Errorf("encode project conditions: %w", err)
	}
	steps, err := json.Marshal([]operation.Step{write.operationStep})
	if err != nil {
		return fmt.Errorf("encode operation steps: %w", err)
	}
	var operationError any
	if write.operationError != nil {
		encoded, err := json.Marshal(write.operationError)
		if err != nil {
			return fmt.Errorf("encode operation error: %w", err)
		}
		operationError = encoded
	}

	transaction, err := repository.database.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin shared-isolation state transaction: %w", err)
	}
	defer transaction.Rollback()

	projectResult, err := transaction.ExecContext(ctx, `
UPDATE projects
SET
  target_cluster_id = $2,
  manifest_work_name = $3,
  observed_state = $4,
  provisioning_state = $5,
  conditions = $6,
  observed_generation = CASE WHEN $7 > 0 THEN $7 ELSE observed_generation END,
  applied_gpu_quota = CASE WHEN $7 > 0 THEN $8 ELSE applied_gpu_quota END,
  last_reconciled_at = $9,
  updated_at = $9
WHERE id = $1`,
		state.ProjectID,
		state.ClusterID,
		state.WorkName,
		write.observedState,
		write.provisioningState,
		conditions,
		write.observedGeneration,
		write.appliedGPUQuota,
		write.condition.LastTransitionTime,
	)
	if err != nil {
		return fmt.Errorf("update project shared-isolation state: %w", err)
	}
	if err := requireAffectedResource(projectResult, tenancy.ErrNotFound); err != nil {
		return err
	}

	operationResult, err := transaction.ExecContext(ctx, `
UPDATE operations
SET
  status = $2,
  steps = $3,
  progress = $4,
  retryable = $5,
  error = $6,
  started_at = COALESCE(started_at, $7),
  finished_at = $8,
  updated_at = $7
WHERE id = $1`,
		state.OperationID,
		write.operationStatus,
		steps,
		write.operationProgress,
		write.operationRetryable,
		operationError,
		write.condition.LastTransitionTime,
		write.finishedAt,
	)
	if err != nil {
		return fmt.Errorf("update shared-isolation operation: %w", err)
	}
	if err := requireAffectedResource(operationResult, operation.ErrNotFound); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit shared-isolation state transaction: %w", err)
	}
	return nil
}

func requireAffectedResource(result sql.Result, notFound error) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read affected resource rows: %w", err)
	}
	if affected == 0 {
		return notFound
	}
	return nil
}

var _ sharedisolation.Repository = (*Repository)(nil)
