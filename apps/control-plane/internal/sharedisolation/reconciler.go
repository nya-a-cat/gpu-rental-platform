package sharedisolation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/outbox"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/ports"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

type ProjectSnapshot struct {
	Project  tenancy.Project
	GPUQuota int64
}

type Repository interface {
	LoadProjectForIsolation(context.Context, string) (ProjectSnapshot, error)
	StartSharedIsolation(context.Context, ReconcileState) error
	CompleteSharedIsolation(context.Context, ReconcileState) error
	FailSharedIsolation(context.Context, ReconcileState, error, bool) error
}

type ReconcileState struct {
	ProjectID         string
	OperationID       string
	ClusterID         string
	WorkName          string
	ProjectGeneration int64
	GPUQuota          int64
}

type Reconciler struct {
	repository            Repository
	fleet                 ports.FleetManager
	clusterID             string
	addonInstallNamespace string
	addonServiceAccount   string
}

func NewReconciler(
	repository Repository,
	fleet ports.FleetManager,
	clusterID string,
	addonInstallNamespace string,
	addonServiceAccount string,
) (*Reconciler, error) {
	if repository == nil || fleet == nil {
		return nil, errors.New("shared-isolation repository and fleet manager are required")
	}
	if strings.TrimSpace(clusterID) == "" ||
		strings.TrimSpace(addonInstallNamespace) == "" ||
		strings.TrimSpace(addonServiceAccount) == "" {
		return nil, errors.New("shared-isolation cluster and Add-on identities are required")
	}
	return &Reconciler{
		repository:            repository,
		fleet:                 fleet,
		clusterID:             clusterID,
		addonInstallNamespace: addonInstallNamespace,
		addonServiceAccount:   addonServiceAccount,
	}, nil
}

func (reconciler *Reconciler) Reconcile(ctx context.Context, event outbox.Event, terminal bool) error {
	projectID, operationID, err := eventIdentity(event)
	if err != nil {
		return err
	}
	snapshot, err := reconciler.repository.LoadProjectForIsolation(ctx, projectID)
	if err != nil {
		return fmt.Errorf("load project isolation state: %w", err)
	}
	work, err := BuildWork(ManifestInput{
		Project:               snapshot.Project,
		OperationID:           operationID,
		ClusterID:             reconciler.clusterID,
		GPUQuota:              snapshot.GPUQuota,
		AddonInstallNamespace: reconciler.addonInstallNamespace,
		AddonServiceAccount:   reconciler.addonServiceAccount,
	})
	if err != nil {
		return err
	}
	state := ReconcileState{
		ProjectID:         projectID,
		OperationID:       operationID,
		ClusterID:         reconciler.clusterID,
		WorkName:          work.WorkID,
		ProjectGeneration: snapshot.Project.Generation,
		GPUQuota:          snapshot.GPUQuota,
	}

	ready := snapshot.Project.TargetClusterID != nil &&
		*snapshot.Project.TargetClusterID == state.ClusterID &&
		snapshot.Project.ManifestWorkName != nil &&
		*snapshot.Project.ManifestWorkName == state.WorkName &&
		snapshot.Project.ObservedState == "active" &&
		snapshot.Project.ProvisioningState == "succeeded" &&
		snapshot.Project.ObservedGeneration == snapshot.Project.Generation &&
		snapshot.Project.AppliedGPUQuota == snapshot.GPUQuota
	if ready {
		return reconciler.repository.CompleteSharedIsolation(ctx, state)
	}

	if err := reconciler.repository.StartSharedIsolation(ctx, state); err != nil {
		return fmt.Errorf("start shared-isolation reconciliation: %w", err)
	}
	result, applyErr := reconciler.fleet.ApplyWork(ctx, work)
	if applyErr == nil && (!result.Applied || !result.Available) {
		applyErr = errors.New("ManifestWork did not reach Applied and Available")
	}
	if applyErr != nil {
		failure := fmt.Errorf("apply shared-isolation ManifestWork: %w", applyErr)
		if stateErr := reconciler.repository.FailSharedIsolation(ctx, state, failure, terminal); stateErr != nil {
			return errors.Join(failure, stateErr)
		}
		return failure
	}
	if err := reconciler.repository.CompleteSharedIsolation(ctx, state); err != nil {
		return fmt.Errorf("complete shared-isolation reconciliation: %w", err)
	}
	return nil
}

func eventIdentity(event outbox.Event) (string, string, error) {
	var payload struct {
		ResourceID  string `json:"resourceId"`
		ProjectID   string `json:"projectId"`
		OperationID string `json:"operationId"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return "", "", fmt.Errorf("decode %s event: %w", event.EventType, err)
	}
	projectID := payload.ProjectID
	if event.EventType == "project.created" {
		projectID = payload.ResourceID
	}
	if projectID == "" || payload.OperationID == "" {
		return "", "", fmt.Errorf("%s event is missing project or operation identity", event.EventType)
	}
	return projectID, payload.OperationID, nil
}
