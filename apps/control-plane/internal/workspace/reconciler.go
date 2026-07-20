package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/outbox"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/ports"
)

type ReconcileState struct {
	WorkspaceID string
	OperationID string
	ClusterID   string
	WorkName    string
	Generation  int64
}

type ReconcileRepository interface {
	LoadWorkspace(context.Context, string) (Workspace, error)
	StartWorkspace(context.Context, ReconcileState) error
	CompleteWorkspace(context.Context, ReconcileState) error
	FailWorkspace(context.Context, ReconcileState, error, bool) error
}

type Reconciler struct {
	repository ReconcileRepository
	fleet      ports.FleetManager
}

func NewReconciler(repository ReconcileRepository, fleet ports.FleetManager) (*Reconciler, error) {
	if repository == nil || fleet == nil {
		return nil, errors.New("workspace repository and fleet manager are required")
	}
	return &Reconciler{repository: repository, fleet: fleet}, nil
}

func (reconciler *Reconciler) Reconcile(ctx context.Context, event outbox.Event, terminal bool) error {
	workspaceID, operationID, err := eventIdentity(event)
	if err != nil {
		return err
	}
	snapshot, err := reconciler.repository.LoadWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("load workspace: %w", err)
	}
	work, err := BuildWork(ManifestInput{Workspace: snapshot, OperationID: operationID})
	if err != nil {
		return err
	}
	state := ReconcileState{WorkspaceID: workspaceID, OperationID: operationID, ClusterID: snapshot.ClusterID, WorkName: work.WorkID, Generation: snapshot.Generation}
	if snapshot.ObservedGeneration == snapshot.Generation && snapshot.ProvisioningState == "succeeded" && observedMatchesDesired(snapshot) {
		return reconciler.repository.CompleteWorkspace(ctx, state)
	}
	if err := reconciler.repository.StartWorkspace(ctx, state); err != nil {
		return fmt.Errorf("start workspace reconciliation: %w", err)
	}
	result, applyErr := reconciler.fleet.ApplyWork(ctx, work)
	if applyErr == nil && (!result.Applied || !result.Available) {
		applyErr = errors.New("ManifestWork did not reach Applied and Available")
	}
	if applyErr != nil {
		failure := fmt.Errorf("apply workspace ManifestWork: %w", applyErr)
		if stateErr := reconciler.repository.FailWorkspace(ctx, state, failure, terminal); stateErr != nil {
			return errors.Join(failure, stateErr)
		}
		return failure
	}
	if err := reconciler.repository.CompleteWorkspace(ctx, state); err != nil {
		return fmt.Errorf("complete workspace reconciliation: %w", err)
	}
	return nil
}

func observedMatchesDesired(snapshot Workspace) bool {
	switch snapshot.DesiredState {
	case DesiredRunning:
		return snapshot.ObservedState == "running"
	case DesiredStopped:
		return snapshot.ObservedState == "stopped"
	case DesiredTerminated:
		return snapshot.ObservedState == "terminated"
	default:
		return false
	}
}

func eventIdentity(event outbox.Event) (string, string, error) {
	var payload struct {
		ResourceID  string `json:"resourceId"`
		OperationID string `json:"operationId"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return "", "", fmt.Errorf("decode %s event: %w", event.EventType, err)
	}
	if strings.TrimSpace(payload.ResourceID) == "" || strings.TrimSpace(payload.OperationID) == "" {
		return "", "", fmt.Errorf("%s event is missing workspace or operation identity", event.EventType)
	}
	return payload.ResourceID, payload.OperationID, nil
}
