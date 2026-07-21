package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
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
			storageGiB := params.StorageGiB
			if storageGiB == 0 {
				storageGiB = 20
			}
			if storageGiB < 1 || storageGiB > 16384 {
				return fmt.Errorf("workspace storage capacity must be between 1 and 16384 GiB: %w", workspace.ErrInvalid)
			}
			if err := adjustWorkspaceQuota(ctx, tx, params.ProjectID, gpuCount); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO workspaces (id, project_id, cluster_id, accelerator_profile_id, name, gpu_count, storage_gib, namespace_name, desired_state, observed_state, provisioning_state, generation, manifest_work_name, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'running','pending','pending',1,$9,$10,$10)`, id, params.ProjectID, params.ClusterID, params.AcceleratorProfileID, name, gpuCount, storageGiB, namespace, workspace.WorkName(id), now)
			return mapWorkspaceWriteError(err)
		},
	})
}

func (repository *Repository) GetWorkspace(ctx context.Context, id string) (workspace.Workspace, error) {
	if !identity.IsUUID(id) {
		return workspace.Workspace{}, workspace.ErrNotFound
	}
	result, err := scanWorkspace(repository.database.QueryRowContext(ctx, workspaceSelect+" WHERE w.id = $1", id))
	if err != nil {
		return workspace.Workspace{}, err
	}
	result.Snapshots, err = repository.listWorkspaceSnapshots(ctx, id)
	if err != nil {
		return workspace.Workspace{}, err
	}
	return result, nil
}

func (repository *Repository) LoadWorkspace(ctx context.Context, id string) (workspace.Workspace, error) {
	return repository.GetWorkspace(ctx, id)
}

func (repository *Repository) CreateSnapshot(ctx context.Context, params workspace.CreateSnapshotParams) (tenancy.Acceptance, error) {
	if !identity.IsUUID(params.WorkspaceID) {
		return tenancy.Acceptance{}, workspace.ErrNotFound
	}
	name := strings.TrimSpace(params.Name)
	if name == "" || len(name) > 63 || !workspaceNamePattern.MatchString(name) {
		return tenancy.Acceptance{}, fmt.Errorf("snapshot name is invalid: %w", workspace.ErrInvalid)
	}
	id, err := identity.NewUUID()
	if err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("generate snapshot ID: %w", err)
	}
	var projectID string
	if err := repository.database.QueryRowContext(ctx, `SELECT project_id::text FROM workspaces WHERE id = $1`, params.WorkspaceID).Scan(&projectID); errors.Is(err, sql.ErrNoRows) {
		return tenancy.Acceptance{}, workspace.ErrNotFound
	} else if err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("load snapshot project: %w", err)
	}
	return repository.acceptMutation(ctx, params.Mutation, acceptedMutationSpec{
		kind: "workspace.snapshot.create", resourceType: "workspace-snapshot", resourceID: id, eventType: "workspace.snapshot.created",
		scopeType: string(tenancy.ScopeProject), scopeID: projectID, eventFields: map[string]any{"workspaceId": params.WorkspaceID, "name": name},
		apply: func(ctx context.Context, tx *sql.Tx, now time.Time) error {
			var desiredState workspace.DesiredState
			if err := tx.QueryRowContext(ctx, `SELECT desired_state FROM workspaces WHERE id = $1`, params.WorkspaceID).Scan(&desiredState); errors.Is(err, sql.ErrNoRows) {
				return workspace.ErrNotFound
			} else if err != nil {
				return fmt.Errorf("load workspace for snapshot: %w", err)
			}
			if desiredState == workspace.DesiredTerminated {
				return workspace.ErrNotFound
			}
			returnErr := error(nil)
			_, returnErr = tx.ExecContext(ctx, `INSERT INTO workspace_snapshots (id, workspace_id, name, source_pvc_name, state, created_at, updated_at) VALUES ($1,$2,$3,$4,'pending',$5,$5)`, id, params.WorkspaceID, name, workspace.WorkName(params.WorkspaceID)+"-data", now)
			return mapWorkspaceWriteError(returnErr)
		},
	})
}

func (repository *Repository) GetSnapshot(ctx context.Context, workspaceID, snapshotID string) (workspace.Snapshot, error) {
	if !identity.IsUUID(workspaceID) || !identity.IsUUID(snapshotID) {
		return workspace.Snapshot{}, workspace.ErrNotFound
	}
	return scanSnapshot(repository.database.QueryRowContext(ctx, `SELECT id::text, workspace_id::text, name, source_pvc_name, state, created_at, updated_at FROM workspace_snapshots WHERE workspace_id = $1 AND id = $2`, workspaceID, snapshotID))
}

func (repository *Repository) listWorkspaceSnapshots(ctx context.Context, workspaceID string) ([]workspace.Snapshot, error) {
	rows, err := repository.database.QueryContext(ctx, `SELECT id::text, workspace_id::text, name, source_pvc_name, state, created_at, updated_at FROM workspace_snapshots WHERE workspace_id = $1 ORDER BY created_at, id`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list workspace snapshots: %w", err)
	}
	defer rows.Close()
	result := []workspace.Snapshot{}
	for rows.Next() {
		snapshot, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace snapshots: %w", err)
	}
	return result, nil
}

func scanSnapshot(row workspaceScanner) (workspace.Snapshot, error) {
	var result workspace.Snapshot
	if err := row.Scan(&result.ID, &result.WorkspaceID, &result.Name, &result.SourcePVCName, &result.State, &result.CreatedAt, &result.UpdatedAt); errors.Is(err, sql.ErrNoRows) {
		return workspace.Snapshot{}, workspace.ErrNotFound
	} else if err != nil {
		return workspace.Snapshot{}, fmt.Errorf("scan workspace snapshot: %w", err)
	}
	return result, nil
}

func (repository *Repository) StartWorkspace(ctx context.Context, state workspace.ReconcileState) error {
	if !identity.IsUUID(state.WorkspaceID) || state.Generation < 1 {
		return workspace.ErrNotFound
	}
	result, err := repository.database.ExecContext(ctx, `UPDATE workspaces SET provisioning_state = 'provisioning', observed_state = 'unknown', updated_at = $2 WHERE id = $1 AND generation = $3`, state.WorkspaceID, repository.now().UTC(), state.Generation)
	if err != nil {
		return fmt.Errorf("start workspace: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("inspect workspace start: %w", err)
	} else if affected == 0 {
		return workspace.ErrConflict
	}
	return nil
}

func (repository *Repository) CompleteWorkspace(ctx context.Context, state workspace.ReconcileState) error {
	if !identity.IsUUID(state.WorkspaceID) || state.Generation < 1 {
		return workspace.ErrNotFound
	}
	now := repository.now().UTC()
	_, err := repository.database.ExecContext(ctx, `
UPDATE workspaces
SET observed_state = CASE desired_state WHEN 'running' THEN 'running' WHEN 'stopped' THEN 'stopped' ELSE 'terminated' END,
    provisioning_state = 'succeeded',
    observed_generation = generation,
    updated_at = $2
WHERE id = $1 AND generation = $3`, state.WorkspaceID, now, state.Generation)
	if err != nil {
		return fmt.Errorf("complete workspace: %w", err)
	}
	_, err = repository.database.ExecContext(ctx, `UPDATE workspace_snapshots SET state = 'succeeded', updated_at = $2 WHERE workspace_id = $1 AND state = 'pending'`, state.WorkspaceID, now)
	if err != nil {
		return fmt.Errorf("complete workspace snapshots: %w", err)
	}
	return nil
}

func (repository *Repository) FailWorkspace(ctx context.Context, state workspace.ReconcileState, reconcileErr error, terminal bool) error {
	if !identity.IsUUID(state.WorkspaceID) || state.Generation < 1 {
		return workspace.ErrNotFound
	}
	message := "workspace reconciliation failed"
	if reconcileErr != nil {
		message = reconcileErr.Error()
	}
	if len(message) > 1024 {
		message = message[:1024]
	}
	conditions, err := json.Marshal([]tenancy.Condition{{Type: "Ready", Status: "False", Reason: "ReconciliationFailed", Message: message, LastTransitionTime: repository.now().UTC()}})
	if err != nil {
		return fmt.Errorf("encode workspace failure condition: %w", err)
	}
	provisioning := "provisioning"
	if terminal {
		provisioning = "failed"
	}
	_, err = repository.database.ExecContext(ctx, `UPDATE workspaces SET provisioning_state = $2, conditions = $3, updated_at = $4 WHERE id = $1 AND generation = $5`, state.WorkspaceID, provisioning, conditions, repository.now().UTC(), state.Generation)
	if err != nil {
		return fmt.Errorf("fail workspace: %w", err)
	}
	return nil
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
			var projectID string
			var gpuCount int
			var currentState workspace.DesiredState
			if err := tx.QueryRowContext(ctx, `SELECT project_id::text, gpu_count, desired_state FROM workspaces WHERE id = $1 FOR UPDATE`, params.WorkspaceID).Scan(&projectID, &gpuCount, &currentState); errors.Is(err, sql.ErrNoRows) {
				return workspace.ErrNotFound
			} else if err != nil {
				return fmt.Errorf("lock workspace desired state: %w", err)
			}
			delta := workspaceQuotaDelta(currentState, params.DesiredState, gpuCount)
			if delta != 0 {
				if err := adjustWorkspaceQuota(ctx, tx, projectID, delta); err != nil {
					return err
				}
			}
			_, err := tx.ExecContext(ctx, `UPDATE workspaces SET desired_state = $2, generation = generation + 1, updated_at = $3 WHERE id = $1`, params.WorkspaceID, params.DesiredState, now)
			return mapWorkspaceWriteError(err)
		},
	})
}

func (repository *Repository) CreateAccessToken(ctx context.Context, params workspace.CreateAccessTokenParams) (workspace.AccessToken, error) {
	if !identity.IsUUID(params.WorkspaceID) {
		return workspace.AccessToken{}, workspace.ErrNotFound
	}
	if params.AccessType != workspace.AccessSSH && params.AccessType != workspace.AccessWebTerminal && params.AccessType != workspace.AccessJupyter {
		return workspace.AccessToken{}, workspace.ErrInvalid
	}
	if params.TTL <= 0 {
		params.TTL = 10 * time.Minute
	}
	if params.TTL > time.Hour {
		return workspace.AccessToken{}, workspace.ErrInvalid
	}
	if err := validateMutationContext(params.Mutation); err != nil {
		return workspace.AccessToken{}, err
	}
	transaction, err := repository.database.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return workspace.AccessToken{}, fmt.Errorf("begin access token transaction: %w", err)
	}
	defer transaction.Rollback()
	scope := "workspace.access-token:" + params.Mutation.PrincipalID
	if _, err := transaction.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, scope+":"+params.Mutation.IdempotencyKey); err != nil {
		return workspace.AccessToken{}, fmt.Errorf("lock access token idempotency key: %w", err)
	}
	var recordedHash string
	var responseBody []byte
	err = transaction.QueryRowContext(ctx, `SELECT request_hash, response_body FROM idempotency_records WHERE scope = $1 AND idempotency_key = $2`, scope, params.Mutation.IdempotencyKey).Scan(&recordedHash, &responseBody)
	if err == nil {
		if recordedHash != params.Mutation.RequestHash {
			return workspace.AccessToken{}, tenancy.ErrIdempotencyConflict
		}
		var replay workspace.AccessToken
		if err := json.Unmarshal(responseBody, &replay); err != nil {
			return workspace.AccessToken{}, fmt.Errorf("decode access token replay: %w", err)
		}
		if err := transaction.Commit(); err != nil {
			return workspace.AccessToken{}, fmt.Errorf("commit access token replay: %w", err)
		}
		return replay, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return workspace.AccessToken{}, fmt.Errorf("load access token idempotency: %w", err)
	}
	var projectID string
	if err := transaction.QueryRowContext(ctx, `SELECT project_id::text FROM workspaces WHERE id = $1`, params.WorkspaceID).Scan(&projectID); errors.Is(err, sql.ErrNoRows) {
		return workspace.AccessToken{}, workspace.ErrNotFound
	} else if err != nil {
		return workspace.AccessToken{}, fmt.Errorf("load workspace for access token: %w", err)
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return workspace.AccessToken{}, fmt.Errorf("generate access token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	hash := sha256.Sum256([]byte(token))
	tokenID, err := identity.NewUUID()
	if err != nil {
		return workspace.AccessToken{}, fmt.Errorf("generate access token ID: %w", err)
	}
	now := repository.now().UTC()
	issued := workspace.AccessToken{ID: tokenID, WorkspaceID: params.WorkspaceID, AccessType: params.AccessType, Token: token, ExpiresAt: now.Add(params.TTL)}
	if _, err := transaction.ExecContext(ctx, `INSERT INTO workspace_access_tokens (id, workspace_id, access_type, token_hash, expires_at, created_by, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`, tokenID, params.WorkspaceID, params.AccessType, fmt.Sprintf("%x", hash[:]), issued.ExpiresAt, params.Mutation.PrincipalID, now); err != nil {
		return workspace.AccessToken{}, mapWorkspaceWriteError(err)
	}
	if err := repository.insertDomainEventInTx(ctx, transaction, "workspace", params.WorkspaceID, "workspace.access-token.issued", map[string]any{"resourceId": tokenID, "workspaceId": params.WorkspaceID, "accessType": params.AccessType, "expiresAt": issued.ExpiresAt}, now); err != nil {
		return workspace.AccessToken{}, err
	}
	if err := repository.insertAuditEventInTx(ctx, transaction, params.Mutation, "workspace.access-token.issue", "workspace-access-token", tokenID, string(tenancy.ScopeProject), projectID, now); err != nil {
		return workspace.AccessToken{}, err
	}
	responseBody, err = json.Marshal(issued)
	if err != nil {
		return workspace.AccessToken{}, fmt.Errorf("encode access token response: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `INSERT INTO idempotency_records (scope, idempotency_key, request_hash, response_status, response_headers, response_body, resource_type, resource_id, expires_at, created_at) VALUES ($1,$2,$3,201,'{}'::jsonb,$4,'workspace-access-token',$5,$6,$7)`, scope, params.Mutation.IdempotencyKey, params.Mutation.RequestHash, responseBody, tokenID, now.Add(24*time.Hour), now); err != nil {
		return workspace.AccessToken{}, fmt.Errorf("record access token idempotency: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return workspace.AccessToken{}, fmt.Errorf("commit access token: %w", err)
	}
	return issued, nil
}

func (repository *Repository) RevokeAccessToken(ctx context.Context, params workspace.RevokeAccessTokenParams) (tenancy.Acceptance, error) {
	if !identity.IsUUID(params.WorkspaceID) || !identity.IsUUID(params.TokenID) {
		return tenancy.Acceptance{}, workspace.ErrNotFound
	}
	var projectID string
	if err := repository.database.QueryRowContext(ctx, `SELECT project_id::text FROM workspaces WHERE id = $1`, params.WorkspaceID).Scan(&projectID); errors.Is(err, sql.ErrNoRows) {
		return tenancy.Acceptance{}, workspace.ErrNotFound
	} else if err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("load workspace for token revocation: %w", err)
	}
	return repository.acceptMutation(ctx, params.Mutation, acceptedMutationSpec{
		kind: "workspace.access-token.revoke", resourceType: "workspace-access-token", resourceID: params.TokenID,
		eventType: "workspace.access-token.revoked", scopeType: string(tenancy.ScopeProject), scopeID: projectID,
		eventFields: map[string]any{"workspaceId": params.WorkspaceID},
		apply: func(ctx context.Context, tx *sql.Tx, now time.Time) error {
			result, err := tx.ExecContext(ctx, `UPDATE workspace_access_tokens SET revoked_at = $3 WHERE id = $1 AND workspace_id = $2 AND revoked_at IS NULL`, params.TokenID, params.WorkspaceID, now)
			if err != nil {
				return mapWorkspaceWriteError(err)
			}
			count, err := result.RowsAffected()
			if err != nil {
				return fmt.Errorf("inspect access token revocation: %w", err)
			}
			if count == 0 {
				return workspace.ErrNotFound
			}
			return nil
		},
	})
}

func (repository *Repository) InspectAccessToken(ctx context.Context, token string) (workspace.AccessTokenInfo, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return workspace.AccessTokenInfo{}, workspace.ErrNotFound
	}
	hash := sha256.Sum256([]byte(token))
	var result workspace.AccessTokenInfo
	err := repository.database.QueryRowContext(ctx, `SELECT id::text, workspace_id::text, access_type, expires_at FROM workspace_access_tokens WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > $2`, fmt.Sprintf("%x", hash[:]), repository.now().UTC()).Scan(&result.ID, &result.WorkspaceID, &result.AccessType, &result.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return workspace.AccessTokenInfo{}, workspace.ErrNotFound
	}
	if err != nil {
		return workspace.AccessTokenInfo{}, fmt.Errorf("inspect access token: %w", err)
	}
	return result, nil
}

func workspaceQuotaDelta(current, desired workspace.DesiredState, gpuCount int) int {
	if current == desired || gpuCount <= 0 {
		return 0
	}
	wasAllocated := current == workspace.DesiredRunning
	willAllocate := desired == workspace.DesiredRunning
	if willAllocate && !wasAllocated {
		return gpuCount
	}
	if wasAllocated && !willAllocate {
		return -gpuCount
	}
	return 0
}

func adjustWorkspaceQuota(ctx context.Context, tx *sql.Tx, projectID string, delta int) error {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		result, err := tx.ExecContext(ctx, `UPDATE project_quotas SET allocated = allocated + $2, generation = generation + 1, updated_at = $3 WHERE project_id = $1 AND resource_class = 'gpu.nvidia.full' AND allocated + reserved + $2 <= hard_limit`, projectID, delta, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("reserve workspace GPU quota: %w", err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect workspace GPU quota: %w", err)
		}
		if count == 0 {
			return workspace.ErrQuotaExceeded
		}
		return nil
	}
	_, err := tx.ExecContext(ctx, `UPDATE project_quotas SET allocated = GREATEST(allocated + $2, 0), generation = generation + 1, updated_at = $3 WHERE project_id = $1 AND resource_class = 'gpu.nvidia.full'`, projectID, delta, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("release workspace GPU quota: %w", err)
	}
	return nil
}

const workspaceSelect = `SELECT w.id::text, w.project_id::text, w.cluster_id::text, w.accelerator_profile_id::text, w.name, w.gpu_count, w.storage_gib, w.namespace_name, w.desired_state, w.observed_state, w.provisioning_state, w.conditions, w.generation, w.observed_generation, w.manifest_work_name, w.created_at, w.updated_at FROM workspaces w`

type workspaceScanner interface{ Scan(...any) error }

func scanWorkspace(row workspaceScanner) (workspace.Workspace, error) {
	var result workspace.Workspace
	var conditions []byte
	if err := row.Scan(&result.ID, &result.ProjectID, &result.ClusterID, &result.AcceleratorProfileID, &result.Name, &result.GPUCount, &result.StorageGiB, &result.NamespaceName, &result.DesiredState, &result.ObservedState, &result.ProvisioningState, &conditions, &result.Generation, &result.ObservedGeneration, &result.ManifestWorkName, &result.CreatedAt, &result.UpdatedAt); errors.Is(err, sql.ErrNoRows) {
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
