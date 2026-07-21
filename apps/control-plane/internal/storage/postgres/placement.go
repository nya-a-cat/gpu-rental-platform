package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/identity"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/placement"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

func (repository *Repository) CreatePlacement(ctx context.Context, params placement.CreateParams) (tenancy.Acceptance, error) {
	if !identity.IsUUID(params.ProjectID) || !identity.IsUUID(params.AcceleratorProfileID) || params.Quantity <= 0 || params.Quantity > 1024 {
		return tenancy.Acceptance{}, placement.ErrInvalid
	}
	traits, err := json.Marshal(params.Traits)
	if err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("encode placement traits: %w", err)
	}
	decisionID, err := identity.NewUUID()
	if err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("generate placement decision ID: %w", err)
	}
	eventFields := map[string]any{"projectId": params.ProjectID, "acceleratorProfileId": params.AcceleratorProfileID, "quantity": params.Quantity}
	return repository.acceptMutation(ctx, params.Mutation, acceptedMutationSpec{
		kind: "placement.reserve", resourceType: "placement-decision", resourceID: decisionID, eventType: "placement.reserved",
		scopeType: string(tenancy.ScopeProject), scopeID: params.ProjectID, eventFields: eventFields, completeImmediately: true,
		apply: func(ctx context.Context, transaction *sql.Tx, now time.Time) error {
			var tenantID string
			if err := transaction.QueryRowContext(ctx, `SELECT tenant_id::text FROM projects WHERE id = $1`, params.ProjectID).Scan(&tenantID); errors.Is(err, sql.ErrNoRows) {
				return placement.ErrNotFound
			} else if err != nil {
				return fmt.Errorf("load placement project: %w", err)
			}
			var poolID, clusterID, nodePoolID string
			err := transaction.QueryRowContext(ctx, `SELECT id::text, cluster_id::text, node_pool_id::text FROM capacity_pools WHERE enabled = true AND accelerator_profile_id = $1 AND total - reserved - allocated >= $2 ORDER BY total - reserved - allocated DESC, id FOR UPDATE SKIP LOCKED LIMIT 1`, params.AcceleratorProfileID, params.Quantity).Scan(&poolID, &clusterID, &nodePoolID)
			if errors.Is(err, sql.ErrNoRows) {
				return placement.ErrCapacity
			} else if err != nil {
				return fmt.Errorf("select placement capacity: %w", err)
			}
			if _, err := transaction.ExecContext(ctx, `UPDATE capacity_pools SET reserved = reserved + $2, generation = generation + 1, updated_at = $3 WHERE id = $1`, poolID, params.Quantity, now); err != nil {
				return mapCatalogWriteError(err, "capacity pool")
			}
			if _, err := transaction.ExecContext(ctx, `INSERT INTO placement_decisions (id, project_id, capacity_pool_id, cluster_id, node_pool_id, accelerator_profile_id, quantity, traits, status, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'reserved',$9,$9)`, decisionID, params.ProjectID, poolID, clusterID, nodePoolID, params.AcceleratorProfileID, params.Quantity, traits, now); err != nil {
				return mapCatalogWriteError(err, "placement decision")
			}
			eventFields["tenantId"] = tenantID
			eventFields["capacityPoolId"] = poolID
			eventFields["clusterId"] = clusterID
			eventFields["nodePoolId"] = nodePoolID
			return nil
		},
	})
}

func (repository *Repository) GetPlacement(ctx context.Context, id string) (placement.Decision, error) {
	if !identity.IsUUID(id) {
		return placement.Decision{}, placement.ErrNotFound
	}
	var result placement.Decision
	if err := repository.database.QueryRowContext(ctx, `SELECT id::text, project_id::text, capacity_pool_id::text, cluster_id::text, node_pool_id::text, accelerator_profile_id::text, quantity, traits, status, created_at, updated_at FROM placement_decisions WHERE id = $1`, id).Scan(&result.ID, &result.ProjectID, &result.CapacityPoolID, &result.ClusterID, &result.NodePoolID, &result.AcceleratorProfileID, &result.Quantity, &result.Traits, &result.Status, &result.CreatedAt, &result.UpdatedAt); errors.Is(err, sql.ErrNoRows) {
		return placement.Decision{}, placement.ErrNotFound
	} else if err != nil {
		return placement.Decision{}, fmt.Errorf("get placement decision: %w", err)
	}
	return result, nil
}
