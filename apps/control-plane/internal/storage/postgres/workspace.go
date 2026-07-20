package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/identity"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/workspace"
)

var workspaceNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

func (repository *Repository) CreateWorkspace(ctx context.Context, params workspace.CreateParams) (tenancy.Acceptance, error) {
	if !identity.IsUUID(params.ProjectID) || !identity.IsUUID(params.ClusterID) || !identity.IsUUID(params.AcceleratorProfileID) {
		return tenancy.Acceptance{}, workspace.ErrNotFound
	}
	name := strings.TrimSpace(params.Name)
	if name == "" || len(name) > 63 || !workspaceNamePattern.MatchString(name) {
		return tenancy.Acceptance{}, fmt.Errorf("workspace name is invalid: %w", workspace.ErrInvalid)
	}
	id, err := identity.NewUUID()
	if err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("generate workspace ID: %w", err)
	}
	return repository.acceptMutation(ctx, params.Mutation, acceptedMutationSpec{
		kind: "workspace.create", resourceType: "workspace", resourceID: id, eventType: "workspace.created",
		scopeType: string(tenancy.ScopeProject), scopeID: params.ProjectID,
		eventFields: map[string]any{"projectId": params.ProjectID, "clusterId": params.ClusterID, "name": name},
		apply: func(ctx context.Context, tx *sql.Tx, now time.Time) error {
			var namespace string
			var gpuCount int
			if err := tx.QueryRowContext(ctx, `SELECT namespace_name FROM projects WHERE id = $1`, params.ProjectID).Scan(&namespace); errors.Is(err, sql.ErrNoRows) {
				return workspace.ErrNotFound
			} else if err != nil {
				return fmt.Errorf("load workspace project: %w", err)
			}
			if err := tx.QueryRowContext(ctx, `SELECT gpu_count FROM accelerator_profiles WHERE id = $1 AND enabled = true`, params.AcceleratorProfileID).Scan(&gpuCount); errors.Is(err, sql.ErrNoRows) {
				return workspace.ErrNotFound
			} else if err != nil {
				return fmt.Errorf("load workspace accelerator profile: %w", err)
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO workspaces (id, project_id, cluster_id, accelerator_profile_id, name, gpu_count, namespace_name, desired_state, observed_state, provisioning_state, generation, manifest_work_name, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,'running','pending','pending',1,$8,$9,$9)`, id, params.ProjectID, params.ClusterID, params.AcceleratorProfileID, name, gpuCount, namespace, workspace.WorkName(id), now)
			return mapWorkspaceWriteError(err)
		},
	})
}

func (repository *Repository) GetWorkspace(ctx context.Context, id string) (workspace.Workspace, error) {
	if !identity.IsUUID(id) {
		return workspace.Workspace{}, workspace.ErrNotFound
	}
	return scanWorkspace(repository.database.QueryRowContext(ctx, workspaceSelect+" WHERE w.id = $1", id))
}

func (repository *Repository) SetWorkspaceDesiredState(ctx context.Context, params workspace.SetDesiredStateParams) (tenancy.Acceptance, error) {
	if !identity.IsUUID(params.WorkspaceID) {
		return tenancy.Acceptance{}, workspace.ErrNotFound
	}
	if params.DesiredState != workspace.DesiredRunning && params.DesiredState != workspace.DesiredStopped && params.DesiredState != workspace.DesiredTerminated {
		return tenancy.Acceptance{}, workspace.ErrInvalid
	}
	var projectID string
	if err := repository.database.QueryRowContext(ctx, `SELECT project_id::text FROM workspaces WHERE id = $1`, params.WorkspaceID).Scan(&projectID); errors.Is(err, sql.ErrNoRows) {
		return tenancy.Acceptance{}, workspace.ErrNotFound
	} else if err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("load workspace project: %w", err)
	}
	return repository.acceptMutation(ctx, params.Mutation, acceptedMutationSpec{
		kind: "workspace.desired-state.update", resourceType: "workspace", resourceID: params.WorkspaceID, eventType: "workspace.desired-state.updated",
		scopeType: string(tenancy.ScopeProject), scopeID: projectID,
		eventFields: map[string]any{"desiredState": params.DesiredState},
		apply: func(ctx context.Context, tx *sql.Tx, now time.Time) error {
			_, err := tx.ExecContext(ctx, `UPDATE workspaces SET desired_state = $2, generation = generation + 1, updated_at = $3 WHERE id = $1`, params.WorkspaceID, params.DesiredState, now)
			return mapWorkspaceWriteError(err)
		},
	})
}

const workspaceSelect = `SELECT w.id::text, w.project_id::text, w.cluster_id::text, w.accelerator_profile_id::text, w.name, w.gpu_count, w.namespace_name, w.desired_state, w.observed_state, w.provisioning_state, w.conditions, w.generation, w.observed_generation, w.manifest_work_name, w.created_at, w.updated_at FROM workspaces w`

type workspaceScanner interface{ Scan(...any) error }

func scanWorkspace(row workspaceScanner) (workspace.Workspace, error) {
	var result workspace.Workspace
	var conditions []byte
	if err := row.Scan(&result.ID, &result.ProjectID, &result.ClusterID, &result.AcceleratorProfileID, &result.Name, &result.GPUCount, &result.NamespaceName, &result.DesiredState, &result.ObservedState, &result.ProvisioningState, &conditions, &result.Generation, &result.ObservedGeneration, &result.ManifestWorkName, &result.CreatedAt, &result.UpdatedAt); errors.Is(err, sql.ErrNoRows) {
		return workspace.Workspace{}, workspace.ErrNotFound
	} else if err != nil {
		return workspace.Workspace{}, fmt.Errorf("scan workspace: %w", err)
	}
	if err := json.Unmarshal(conditions, &result.Conditions); err != nil {
		return workspace.Workspace{}, fmt.Errorf("decode workspace conditions: %w", err)
	}
	return result, nil
}

func mapWorkspaceWriteError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "duplicate key") {
		return workspace.ErrConflict
	}
	if strings.Contains(err.Error(), "violates foreign key") {
		return workspace.ErrNotFound
	}
	return err
}
