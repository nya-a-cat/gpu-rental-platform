package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/catalog"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/clusterstate"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/identity"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

func (repository *Repository) GetResourceClass(ctx context.Context, name string) (catalog.ResourceClass, error) {
	var result catalog.ResourceClass
	err := repository.database.QueryRowContext(ctx, `
SELECT name, unit, description, created_at FROM resource_classes WHERE name = $1`, name).Scan(
		&result.Name, &result.Unit, &result.Description, &result.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return catalog.ResourceClass{}, catalog.ErrNotFound
	}
	if err != nil {
		return catalog.ResourceClass{}, fmt.Errorf("get resource class: %w", err)
	}
	return result, nil
}

func (repository *Repository) CreateCluster(ctx context.Context, params catalog.CreateClusterParams) (tenancy.Acceptance, error) {
	params.ManagedClusterName = strings.TrimSpace(params.ManagedClusterName)
	params.DisplayName = strings.TrimSpace(params.DisplayName)
	if err := catalog.ValidateCluster(params.ManagedClusterName, params.DisplayName); err != nil {
		return tenancy.Acceptance{}, err
	}
	clusterID, err := identity.NewUUID()
	if err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("generate cluster ID: %w", err)
	}
	return repository.acceptMutation(ctx, params.Mutation, acceptedMutationSpec{
		kind: "cluster.create", resourceType: "cluster", resourceID: clusterID,
		eventType: "cluster.created", scopeType: "system", completeImmediately: true,
		eventFields: map[string]any{"managedClusterName": params.ManagedClusterName},
		apply: func(ctx context.Context, tx *sql.Tx, now time.Time) error {
			_, err := tx.ExecContext(ctx, `
INSERT INTO clusters (
  id, managed_cluster_name, display_name, management_state, connection_state,
  connected, schedulable, inventory_fresh, execution_healthy, conditions,
  generation, created_at, updated_at
) VALUES ($1, $2, $3, 'enabled', 'offline', false, false, false, false, '[]'::jsonb, 1, $4, $4)`,
				clusterID, params.ManagedClusterName, params.DisplayName, now)
			if err != nil {
				return mapCatalogWriteError(err, "cluster")
			}
			_, err = tx.ExecContext(ctx, `
INSERT INTO resource_providers (id, cluster_id, parent_id, provider_type, name, generation, created_at, updated_at)
VALUES ($1, $1, NULL, 'cluster', $2, 1, $3, $3)`, clusterID, params.ManagedClusterName, now)
			return mapCatalogWriteError(err, "cluster resource provider")
		},
	})
}

func (repository *Repository) GetCluster(ctx context.Context, clusterID string) (catalog.Cluster, error) {
	if !identity.IsUUID(clusterID) {
		return catalog.Cluster{}, catalog.ErrNotFound
	}
	return scanCluster(repository.database.QueryRowContext(ctx, clusterSelect+" WHERE id = $1", clusterID))
}

func (repository *Repository) ReplaceClusterInventory(ctx context.Context, params catalog.ReplaceInventoryParams) (tenancy.Acceptance, error) {
	if !identity.IsUUID(params.ClusterID) {
		return tenancy.Acceptance{}, catalog.ErrNotFound
	}
	if err := catalog.ValidateInventory(params); err != nil {
		return tenancy.Acceptance{}, err
	}
	return repository.acceptMutation(ctx, params.Mutation, acceptedMutationSpec{
		kind: "cluster.inventory.replace", resourceType: "cluster", resourceID: params.ClusterID,
		eventType: "cluster.inventory.replaced", scopeType: "system", completeImmediately: true,
		eventFields: map[string]any{"expectedGeneration": params.ExpectedGeneration, "sourceGeneration": params.SourceGeneration},
		apply: func(ctx context.Context, tx *sql.Tx, now time.Time) error {
			var currentGeneration int64
			var currentEpoch sql.NullString
			var currentSequence int64
			var lastInventory sql.NullTime
			err := tx.QueryRowContext(ctx, `
SELECT inventory_generation, agent_epoch, report_sequence, last_inventory_at
FROM clusters WHERE id = $1 FOR UPDATE`, params.ClusterID).Scan(
				&currentGeneration, &currentEpoch, &currentSequence, &lastInventory,
			)
			if errors.Is(err, sql.ErrNoRows) {
				return catalog.ErrNotFound
			}
			if err != nil {
				return fmt.Errorf("lock cluster inventory: %w", err)
			}
			if currentGeneration != params.ExpectedGeneration {
				return catalog.ErrGenerationConflict
			}
			if currentEpoch.Valid && currentEpoch.String == params.AgentEpoch && params.ReportSequence <= uint64(currentSequence) {
				return catalog.ErrStaleReport
			}
			if lastInventory.Valid && params.ObservedAt.Before(lastInventory.Time) {
				return catalog.ErrStaleReport
			}
			generation := currentGeneration + 1
			for _, pool := range params.NodePools {
				poolID, err := upsertProvider(ctx, tx, params.ClusterID, params.ClusterID, "node_pool", pool.Name, generation, now)
				if err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `
INSERT INTO node_pools (id, cluster_id, name, management_state, last_seen_inventory_generation, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $6)
ON CONFLICT (id) DO UPDATE SET management_state = EXCLUDED.management_state,
  last_seen_inventory_generation = EXCLUDED.last_seen_inventory_generation, updated_at = EXCLUDED.updated_at`,
					poolID, params.ClusterID, pool.Name, pool.ManagementState, generation, now); err != nil {
					return mapCatalogWriteError(err, "node pool")
				}
				for _, node := range pool.Nodes {
					nodeID, err := upsertProvider(ctx, tx, params.ClusterID, poolID, "node", opaqueProviderName("node", node.OpaqueKey), generation, now)
					if err != nil {
						return err
					}
					if _, err := tx.ExecContext(ctx, `
INSERT INTO nodes (id, cluster_id, node_pool_id, opaque_key, management_state, health_state, schedulable, last_seen_inventory_generation, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
ON CONFLICT (id) DO UPDATE SET node_pool_id = EXCLUDED.node_pool_id, management_state = EXCLUDED.management_state,
  health_state = EXCLUDED.health_state, schedulable = EXCLUDED.schedulable,
  last_seen_inventory_generation = EXCLUDED.last_seen_inventory_generation, updated_at = EXCLUDED.updated_at`,
						nodeID, params.ClusterID, poolID, node.OpaqueKey, node.ManagementState, node.HealthState, node.Schedulable, generation, now); err != nil {
						return mapCatalogWriteError(err, "node")
					}
					if err := replaceTraits(ctx, tx, nodeID, node.Traits, now); err != nil {
						return err
					}
					for _, device := range node.GPUDevices {
						deviceID, err := upsertProvider(ctx, tx, params.ClusterID, nodeID, "gpu_device", opaqueProviderName("gpu", node.OpaqueKey+"\x00"+device.OpaqueKey), generation, now)
						if err != nil {
							return err
						}
						if _, err := tx.ExecContext(ctx, `
INSERT INTO gpu_devices (
  id, cluster_id, node_id, opaque_key, resource_class, model, memory_mib,
  accelerator_mode, health_state, allocatable, last_seen_inventory_generation, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $12)
ON CONFLICT (id) DO UPDATE SET node_id = EXCLUDED.node_id, resource_class = EXCLUDED.resource_class,
  model = EXCLUDED.model, memory_mib = EXCLUDED.memory_mib, accelerator_mode = EXCLUDED.accelerator_mode,
  health_state = EXCLUDED.health_state, allocatable = EXCLUDED.allocatable,
  last_seen_inventory_generation = EXCLUDED.last_seen_inventory_generation, updated_at = EXCLUDED.updated_at`,
							deviceID, params.ClusterID, nodeID, device.OpaqueKey, device.ResourceClass, device.Model,
							device.MemoryMiB, device.AcceleratorMode, device.HealthState, device.Allocatable, generation, now); err != nil {
							return mapCatalogWriteError(err, "GPU device")
						}
						if err := replaceTraits(ctx, tx, deviceID, device.Traits, now); err != nil {
							return err
						}
					}
				}
			}
			if err := markStaleCatalogResources(ctx, tx, params.ClusterID, generation, now); err != nil {
				return err
			}
			if err := refreshCatalogInventories(ctx, tx, params.ClusterID, generation, now); err != nil {
				return err
			}
			status, err := clusterstate.Evaluate(clusterstate.Signals{
				Now: now, LastHeartbeatAt: params.ObservedAt, LastInventoryAt: params.ObservedAt,
				ManuallySchedulable: true, ExecutionHealthy: params.ExecutionHealthy, Fenced: params.Fenced,
			}, clusterstate.DefaultThresholds())
			if err != nil {
				return fmt.Errorf("evaluate cluster state: %w", err)
			}
			_, err = tx.ExecContext(ctx, `
UPDATE clusters SET connection_state = $2, connected = $3, schedulable = $4, inventory_fresh = $5,
  execution_healthy = $6, fenced = $7, agent_epoch = $8, report_sequence = $9,
  fencing_token = $10, fencing_enabled = $11, inventory_generation = $12, source_generation = $13,
  last_heartbeat_at = $14, last_inventory_at = $14, generation = generation + 1, updated_at = $15
WHERE id = $1`, params.ClusterID, status.ConnectionState, status.Connected, status.Schedulable,
				status.InventoryFresh, status.ExecutionHealthy, params.Fenced, params.AgentEpoch, params.ReportSequence,
				params.FencingToken, params.FencingEnabled, generation, params.SourceGeneration, params.ObservedAt, now)
			if err != nil {
				return fmt.Errorf("update cluster inventory state: %w", err)
			}
			return nil
		},
	})
}

func (repository *Repository) GetClusterInventory(ctx context.Context, clusterID string) (catalog.ClusterInventory, error) {
	cluster, err := repository.GetCluster(ctx, clusterID)
	if err != nil {
		return catalog.ClusterInventory{}, err
	}
	result := catalog.ClusterInventory{Cluster: cluster, NodePools: []catalog.NodePool{}, Nodes: []catalog.Node{}, GPUDevices: []catalog.GPUDevice{}, Inventories: []catalog.Inventory{}}

	poolRows, err := repository.database.QueryContext(ctx, `
SELECT np.id::text, np.cluster_id::text, np.name, np.management_state, rp.generation,
  np.last_seen_inventory_generation, np.created_at, np.updated_at
FROM node_pools np JOIN resource_providers rp ON rp.id = np.id
WHERE np.cluster_id = $1 ORDER BY np.name, np.id`, clusterID)
	if err != nil {
		return catalog.ClusterInventory{}, fmt.Errorf("list node pools: %w", err)
	}
	for poolRows.Next() {
		var item catalog.NodePool
		if err := poolRows.Scan(&item.ID, &item.ClusterID, &item.Name, &item.ManagementState, &item.Generation, &item.LastSeenGeneration, &item.CreatedAt, &item.UpdatedAt); err != nil {
			poolRows.Close()
			return catalog.ClusterInventory{}, fmt.Errorf("scan node pool: %w", err)
		}
		result.NodePools = append(result.NodePools, item)
	}
	if err := poolRows.Err(); err != nil {
		poolRows.Close()
		return catalog.ClusterInventory{}, fmt.Errorf("iterate node pools: %w", err)
	}
	poolRows.Close()

	nodeRows, err := repository.database.QueryContext(ctx, `
SELECT n.id::text, n.cluster_id::text, n.node_pool_id::text, n.opaque_key, n.management_state,
  n.health_state, n.schedulable, rp.generation, n.last_seen_inventory_generation, n.created_at, n.updated_at
FROM nodes n JOIN resource_providers rp ON rp.id = n.id
WHERE n.cluster_id = $1 ORDER BY n.node_pool_id, n.opaque_key, n.id`, clusterID)
	if err != nil {
		return catalog.ClusterInventory{}, fmt.Errorf("list nodes: %w", err)
	}
	for nodeRows.Next() {
		var item catalog.Node
		if err := nodeRows.Scan(&item.ID, &item.ClusterID, &item.NodePoolID, &item.OpaqueKey, &item.ManagementState,
			&item.HealthState, &item.Schedulable, &item.Generation, &item.LastSeenGeneration, &item.CreatedAt, &item.UpdatedAt); err != nil {
			nodeRows.Close()
			return catalog.ClusterInventory{}, fmt.Errorf("scan node: %w", err)
		}
		item.Traits, err = repository.loadTraits(ctx, item.ID)
		if err != nil {
			nodeRows.Close()
			return catalog.ClusterInventory{}, err
		}
		result.Nodes = append(result.Nodes, item)
	}
	if err := nodeRows.Err(); err != nil {
		nodeRows.Close()
		return catalog.ClusterInventory{}, fmt.Errorf("iterate nodes: %w", err)
	}
	nodeRows.Close()

	deviceRows, err := repository.database.QueryContext(ctx, `
SELECT g.id::text, g.cluster_id::text, g.node_id::text, g.opaque_key, g.resource_class, g.model,
  g.memory_mib, g.accelerator_mode, g.health_state, g.allocatable, rp.generation,
  g.last_seen_inventory_generation, g.created_at, g.updated_at
FROM gpu_devices g JOIN resource_providers rp ON rp.id = g.id
WHERE g.cluster_id = $1 ORDER BY g.node_id, g.opaque_key, g.id`, clusterID)
	if err != nil {
		return catalog.ClusterInventory{}, fmt.Errorf("list GPU devices: %w", err)
	}
	for deviceRows.Next() {
		var item catalog.GPUDevice
		if err := deviceRows.Scan(&item.ID, &item.ClusterID, &item.NodeID, &item.OpaqueKey, &item.ResourceClass, &item.Model,
			&item.MemoryMiB, &item.AcceleratorMode, &item.HealthState, &item.Allocatable, &item.Generation,
			&item.LastSeenGeneration, &item.CreatedAt, &item.UpdatedAt); err != nil {
			deviceRows.Close()
			return catalog.ClusterInventory{}, fmt.Errorf("scan GPU device: %w", err)
		}
		item.Traits, err = repository.loadTraits(ctx, item.ID)
		if err != nil {
			deviceRows.Close()
			return catalog.ClusterInventory{}, err
		}
		result.GPUDevices = append(result.GPUDevices, item)
	}
	if err := deviceRows.Err(); err != nil {
		deviceRows.Close()
		return catalog.ClusterInventory{}, fmt.Errorf("iterate GPU devices: %w", err)
	}
	deviceRows.Close()

	inventoryRows, err := repository.database.QueryContext(ctx, `
SELECT ri.resource_provider_id::text, ri.resource_class, ri.total, ri.reserved, ri.allocated, ri.generation, ri.updated_at
FROM resource_inventories ri JOIN resource_providers rp ON rp.id = ri.resource_provider_id
WHERE rp.cluster_id = $1 ORDER BY rp.provider_type, ri.resource_provider_id, ri.resource_class`, clusterID)
	if err != nil {
		return catalog.ClusterInventory{}, fmt.Errorf("list inventories: %w", err)
	}
	defer inventoryRows.Close()
	for inventoryRows.Next() {
		var item catalog.Inventory
		if err := inventoryRows.Scan(&item.ResourceProviderID, &item.ResourceClass, &item.Total, &item.Reserved, &item.Allocated, &item.Generation, &item.UpdatedAt); err != nil {
			return catalog.ClusterInventory{}, fmt.Errorf("scan inventory: %w", err)
		}
		result.Inventories = append(result.Inventories, item)
	}
	if err := inventoryRows.Err(); err != nil {
		return catalog.ClusterInventory{}, fmt.Errorf("iterate inventories: %w", err)
	}
	return result, nil
}

func (repository *Repository) CreateAcceleratorProfile(ctx context.Context, params catalog.CreateAcceleratorProfileParams) (tenancy.Acceptance, error) {
	params.Name = strings.TrimSpace(params.Name)
	params.Slug = strings.TrimSpace(params.Slug)
	if err := catalog.ValidateAcceleratorProfile(params); err != nil {
		return tenancy.Acceptance{}, err
	}
	profileID, err := identity.NewUUID()
	if err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("generate accelerator profile ID: %w", err)
	}
	traits, err := json.Marshal(params.Traits)
	if err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("encode accelerator profile traits: %w", err)
	}
	return repository.acceptMutation(ctx, params.Mutation, acceptedMutationSpec{
		kind: "accelerator_profile.create", resourceType: "accelerator_profile", resourceID: profileID,
		eventType: "accelerator_profile.created", scopeType: "system", completeImmediately: true,
		eventFields: map[string]any{"slug": params.Slug, "resourceClass": params.ResourceClass},
		apply: func(ctx context.Context, tx *sql.Tx, now time.Time) error {
			_, err := tx.ExecContext(ctx, `
INSERT INTO accelerator_profiles (
  id, name, slug, accelerator_mode, resource_class, gpu_count, memory_mib, traits,
  enabled, generation, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, true, 1, $9, $9)`,
				profileID, params.Name, params.Slug, params.AcceleratorMode, params.ResourceClass,
				params.GPUCount, params.MemoryMiB, traits, now)
			return mapCatalogWriteError(err, "accelerator profile")
		},
	})
}

func (repository *Repository) GetAcceleratorProfile(ctx context.Context, profileID string) (catalog.AcceleratorProfile, error) {
	if !identity.IsUUID(profileID) {
		return catalog.AcceleratorProfile{}, catalog.ErrNotFound
	}
	var result catalog.AcceleratorProfile
	var memory sql.NullInt64
	var traits []byte
	err := repository.database.QueryRowContext(ctx, `
SELECT id::text, name, slug, accelerator_mode, resource_class, gpu_count, memory_mib, traits,
  enabled, generation, created_at, updated_at
FROM accelerator_profiles WHERE id = $1`, profileID).Scan(
		&result.ID, &result.Name, &result.Slug, &result.AcceleratorMode, &result.ResourceClass,
		&result.GPUCount, &memory, &traits, &result.Enabled, &result.Generation, &result.CreatedAt, &result.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return catalog.AcceleratorProfile{}, catalog.ErrNotFound
	}
	if err != nil {
		return catalog.AcceleratorProfile{}, fmt.Errorf("get accelerator profile: %w", err)
	}
	if memory.Valid {
		result.MemoryMiB = &memory.Int64
	}
	if err := json.Unmarshal(traits, &result.Traits); err != nil {
		return catalog.AcceleratorProfile{}, fmt.Errorf("decode accelerator profile traits: %w", err)
	}
	return result, nil
}

func (repository *Repository) CreateCapacityPool(ctx context.Context, params catalog.CreateCapacityPoolParams) (tenancy.Acceptance, error) {
	params.Name = strings.TrimSpace(params.Name)
	if !identity.IsUUID(params.ClusterID) || !identity.IsUUID(params.NodePoolID) || !identity.IsUUID(params.AcceleratorProfileID) {
		return tenancy.Acceptance{}, catalog.ErrNotFound
	}
	if err := catalog.ValidateCapacityPool(params); err != nil {
		return tenancy.Acceptance{}, err
	}
	poolID, err := identity.NewUUID()
	if err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("generate capacity pool ID: %w", err)
	}
	return repository.acceptMutation(ctx, params.Mutation, acceptedMutationSpec{
		kind: "capacity_pool.create", resourceType: "capacity_pool", resourceID: poolID,
		eventType: "capacity_pool.created", scopeType: "system", completeImmediately: true,
		eventFields: map[string]any{"clusterId": params.ClusterID, "nodePoolId": params.NodePoolID, "acceleratorProfileId": params.AcceleratorProfileID},
		apply: func(ctx context.Context, tx *sql.Tx, now time.Time) error {
			var total int64
			err := tx.QueryRowContext(ctx, `
SELECT COALESCE(ri.total / ap.gpu_count, 0)
FROM node_pools np
JOIN accelerator_profiles ap ON ap.id = $3 AND ap.enabled
LEFT JOIN resource_inventories ri ON ri.resource_provider_id = np.id AND ri.resource_class = ap.resource_class
WHERE np.id = $2 AND np.cluster_id = $1`, params.ClusterID, params.NodePoolID, params.AcceleratorProfileID).Scan(&total)
			if errors.Is(err, sql.ErrNoRows) {
				return catalog.ErrNotFound
			}
			if err != nil {
				return fmt.Errorf("calculate capacity pool inventory: %w", err)
			}
			_, err = tx.ExecContext(ctx, `
INSERT INTO capacity_pools (
  id, name, cluster_id, node_pool_id, accelerator_profile_id, scheduler_profile,
  total, reserved, allocated, enabled, generation, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, 0, 0, true, 1, $8, $8)`,
				poolID, params.Name, params.ClusterID, params.NodePoolID, params.AcceleratorProfileID, params.SchedulerProfile, total, now)
			return mapCatalogWriteError(err, "capacity pool")
		},
	})
}

func (repository *Repository) GetCapacityPool(ctx context.Context, poolID string) (catalog.CapacityPool, error) {
	if !identity.IsUUID(poolID) {
		return catalog.CapacityPool{}, catalog.ErrNotFound
	}
	var result catalog.CapacityPool
	err := repository.database.QueryRowContext(ctx, `
SELECT id::text, name, cluster_id::text, node_pool_id::text, accelerator_profile_id::text,
  scheduler_profile, total, reserved, allocated, enabled, generation, created_at, updated_at
FROM capacity_pools WHERE id = $1`, poolID).Scan(
		&result.ID, &result.Name, &result.ClusterID, &result.NodePoolID, &result.AcceleratorProfileID,
		&result.SchedulerProfile, &result.Total, &result.Reserved, &result.Allocated, &result.Enabled,
		&result.Generation, &result.CreatedAt, &result.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return catalog.CapacityPool{}, catalog.ErrNotFound
	}
	if err != nil {
		return catalog.CapacityPool{}, fmt.Errorf("get capacity pool: %w", err)
	}
	return result, nil
}

func upsertProvider(ctx context.Context, tx *sql.Tx, clusterID, parentID, providerType, name string, generation int64, now time.Time) (string, error) {
	var providerID string
	err := tx.QueryRowContext(ctx, `
SELECT id::text FROM resource_providers WHERE cluster_id = $1 AND provider_type = $2 AND name = $3`,
		clusterID, providerType, name).Scan(&providerID)
	if err == nil {
		_, err = tx.ExecContext(ctx, `
UPDATE resource_providers SET parent_id = $2, generation = $3, updated_at = $4 WHERE id = $1`,
			providerID, parentID, generation, now)
		if err != nil {
			return "", fmt.Errorf("update resource provider: %w", err)
		}
		return providerID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("find resource provider: %w", err)
	}
	providerID, err = identity.NewUUID()
	if err != nil {
		return "", fmt.Errorf("generate resource provider ID: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO resource_providers (id, cluster_id, parent_id, provider_type, name, generation, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $7)`, providerID, clusterID, parentID, providerType, name, generation, now)
	if err != nil {
		return "", mapCatalogWriteError(err, "resource provider")
	}
	return providerID, nil
}

func replaceTraits(ctx context.Context, tx *sql.Tx, providerID string, traits map[string]string, now time.Time) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM resource_traits WHERE resource_provider_id = $1", providerID); err != nil {
		return fmt.Errorf("delete resource traits: %w", err)
	}
	for key, value := range traits {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO resource_traits (resource_provider_id, trait_key, trait_value, updated_at)
VALUES ($1, $2, $3, $4)`, providerID, key, value, now); err != nil {
			return fmt.Errorf("insert resource trait: %w", err)
		}
	}
	return nil
}

func markStaleCatalogResources(ctx context.Context, tx *sql.Tx, clusterID string, generation int64, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `
UPDATE gpu_devices SET health_state = 'unreachable', allocatable = false, updated_at = $3
WHERE cluster_id = $1 AND last_seen_inventory_generation < $2`, clusterID, generation, now); err != nil {
		return fmt.Errorf("mark stale GPU devices: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE nodes SET health_state = 'unreachable', schedulable = false, updated_at = $3
WHERE cluster_id = $1 AND last_seen_inventory_generation < $2`, clusterID, generation, now); err != nil {
		return fmt.Errorf("mark stale nodes: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE node_pools SET management_state = 'disabled', updated_at = $3
WHERE cluster_id = $1 AND last_seen_inventory_generation < $2`, clusterID, generation, now); err != nil {
		return fmt.Errorf("mark stale node pools: %w", err)
	}
	return nil
}

func refreshCatalogInventories(ctx context.Context, tx *sql.Tx, clusterID string, generation int64, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `
INSERT INTO resource_inventories (resource_provider_id, resource_class, total, reserved, allocated, generation, updated_at)
SELECT g.id, g.resource_class,
  CASE WHEN g.allocatable AND g.health_state = 'healthy' AND n.schedulable AND n.health_state = 'healthy'
    AND n.management_state = 'enabled' AND np.management_state = 'enabled' THEN 1 ELSE 0 END,
  COALESCE(ri.reserved, 0), COALESCE(ri.allocated, 0), $2, $3
FROM gpu_devices g
JOIN nodes n ON n.id = g.node_id
JOIN node_pools np ON np.id = n.node_pool_id
LEFT JOIN resource_inventories ri ON ri.resource_provider_id = g.id AND ri.resource_class = g.resource_class
WHERE g.cluster_id = $1
ON CONFLICT (resource_provider_id, resource_class) DO UPDATE SET
  total = GREATEST(EXCLUDED.total, resource_inventories.reserved + resource_inventories.allocated),
  generation = EXCLUDED.generation, updated_at = EXCLUDED.updated_at`, clusterID, generation, now); err != nil {
		return fmt.Errorf("refresh GPU inventories: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO resource_inventories (resource_provider_id, resource_class, total, reserved, allocated, generation, updated_at)
SELECT n.id, $4,
  count(g.id) FILTER (WHERE g.allocatable AND g.health_state = 'healthy' AND n.schedulable
    AND n.health_state = 'healthy' AND n.management_state = 'enabled' AND np.management_state = 'enabled'),
  COALESCE(ri.reserved, 0), COALESCE(ri.allocated, 0), $2, $3
FROM nodes n
JOIN node_pools np ON np.id = n.node_pool_id
LEFT JOIN gpu_devices g ON g.node_id = n.id AND g.resource_class = $4
LEFT JOIN resource_inventories ri ON ri.resource_provider_id = n.id AND ri.resource_class = $4
WHERE n.cluster_id = $1
GROUP BY n.id, ri.reserved, ri.allocated
ON CONFLICT (resource_provider_id, resource_class) DO UPDATE SET
  total = GREATEST(EXCLUDED.total, resource_inventories.reserved + resource_inventories.allocated),
  generation = EXCLUDED.generation, updated_at = EXCLUDED.updated_at`, clusterID, generation, now, catalog.WholeGPUResourceClass); err != nil {
		return fmt.Errorf("refresh node inventories: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO resource_inventories (resource_provider_id, resource_class, total, reserved, allocated, generation, updated_at)
SELECT np.id, $4,
  count(g.id) FILTER (WHERE g.allocatable AND g.health_state = 'healthy' AND n.schedulable
    AND n.health_state = 'healthy' AND n.management_state = 'enabled' AND np.management_state = 'enabled'),
  COALESCE(ri.reserved, 0), COALESCE(ri.allocated, 0), $2, $3
FROM node_pools np
LEFT JOIN nodes n ON n.node_pool_id = np.id
LEFT JOIN gpu_devices g ON g.node_id = n.id AND g.resource_class = $4
LEFT JOIN resource_inventories ri ON ri.resource_provider_id = np.id AND ri.resource_class = $4
WHERE np.cluster_id = $1
GROUP BY np.id, ri.reserved, ri.allocated
ON CONFLICT (resource_provider_id, resource_class) DO UPDATE SET
  total = GREATEST(EXCLUDED.total, resource_inventories.reserved + resource_inventories.allocated),
  generation = EXCLUDED.generation, updated_at = EXCLUDED.updated_at`, clusterID, generation, now, catalog.WholeGPUResourceClass); err != nil {
		return fmt.Errorf("refresh node pool inventories: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO resource_inventories (resource_provider_id, resource_class, total, reserved, allocated, generation, updated_at)
SELECT $1, $4, COALESCE(sum(ri.total), 0), COALESCE(existing.reserved, 0), COALESCE(existing.allocated, 0), $2, $3
FROM node_pools np
LEFT JOIN resource_inventories ri ON ri.resource_provider_id = np.id AND ri.resource_class = $4
LEFT JOIN resource_inventories existing ON existing.resource_provider_id = $1 AND existing.resource_class = $4
WHERE np.cluster_id = $1
GROUP BY existing.reserved, existing.allocated
ON CONFLICT (resource_provider_id, resource_class) DO UPDATE SET
  total = GREATEST(EXCLUDED.total, resource_inventories.reserved + resource_inventories.allocated),
  generation = EXCLUDED.generation, updated_at = EXCLUDED.updated_at`, clusterID, generation, now, catalog.WholeGPUResourceClass); err != nil {
		return fmt.Errorf("refresh cluster inventory: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE capacity_pools cp SET total = GREATEST(ri.total / ap.gpu_count, cp.reserved + cp.allocated),
  generation = cp.generation + 1, updated_at = $2
FROM accelerator_profiles ap
JOIN resource_inventories ri ON ri.resource_class = ap.resource_class
WHERE cp.cluster_id = $1 AND cp.accelerator_profile_id = ap.id AND ri.resource_provider_id = cp.node_pool_id`, clusterID, now); err != nil {
		return fmt.Errorf("refresh capacity pools: %w", err)
	}
	return nil
}

func (repository *Repository) loadTraits(ctx context.Context, providerID string) (map[string]string, error) {
	rows, err := repository.database.QueryContext(ctx, `
SELECT trait_key, trait_value FROM resource_traits WHERE resource_provider_id = $1 ORDER BY trait_key`, providerID)
	if err != nil {
		return nil, fmt.Errorf("list resource traits: %w", err)
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan resource trait: %w", err)
		}
		result[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate resource traits: %w", err)
	}
	return result, nil
}

func opaqueProviderName(kind, key string) string {
	digest := sha256.Sum256([]byte(key))
	return kind + ":" + hex.EncodeToString(digest[:16])
}

const clusterSelect = `
SELECT id::text, managed_cluster_name, display_name, management_state, connection_state,
  connected, schedulable, inventory_fresh, execution_healthy, fenced, agent_epoch,
  report_sequence, fencing_enabled, inventory_generation, source_generation,
  last_heartbeat_at, last_inventory_at, conditions, generation, created_at, updated_at
FROM clusters`

func scanCluster(row scanner) (catalog.Cluster, error) {
	var result catalog.Cluster
	var agentEpoch, sourceGeneration sql.NullString
	var lastHeartbeat, lastInventory sql.NullTime
	var conditions []byte
	var reportSequence int64
	err := row.Scan(
		&result.ID, &result.ManagedClusterName, &result.DisplayName, &result.ManagementState, &result.ConnectionState,
		&result.Connected, &result.Schedulable, &result.InventoryFresh, &result.ExecutionHealthy, &result.Fenced,
		&agentEpoch, &reportSequence, &result.FencingEnabled, &result.InventoryGeneration, &sourceGeneration,
		&lastHeartbeat, &lastInventory, &conditions, &result.Generation, &result.CreatedAt, &result.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return catalog.Cluster{}, catalog.ErrNotFound
	}
	if err != nil {
		return catalog.Cluster{}, fmt.Errorf("scan cluster: %w", err)
	}
	result.ReportSequence = uint64(reportSequence)
	if agentEpoch.Valid {
		result.AgentEpoch = &agentEpoch.String
	}
	if sourceGeneration.Valid {
		result.SourceGeneration = &sourceGeneration.String
	}
	if lastHeartbeat.Valid {
		result.LastHeartbeatAt = &lastHeartbeat.Time
	}
	if lastInventory.Valid {
		result.LastInventoryAt = &lastInventory.Time
	}
	result.Conditions = append(json.RawMessage(nil), conditions...)
	return result, nil
}

func mapCatalogWriteError(err error, resource string) error {
	if err == nil {
		return nil
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%s already exists: %w", resource, catalog.ErrConflict)
		case "23503":
			return catalog.ErrNotFound
		case "23514", "22P02":
			return catalog.ErrInvalid
		}
	}
	return err
}

var _ catalog.Repository = (*Repository)(nil)
