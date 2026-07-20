package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/catalog"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/operation"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/outbox"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

func catalogMutation(key, hash string) tenancy.MutationContext {
	return tenancy.MutationContext{PrincipalID: "break-glass-admin", RequestID: "request-" + key, IdempotencyKey: key, RequestHash: hash}
}

func TestCatalogInventoryGenerationAndCapacityLifecycle(t *testing.T) {
	database := openTenancyIntegrationDatabase(t)
	repository := NewRepository(database)
	fixedNow := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	repository.now = func() time.Time { return fixedNow }
	ctx := context.Background()

	created, err := repository.CreateCluster(ctx, catalog.CreateClusterParams{
		Mutation:           catalogMutation("cluster-create-key", strings.Repeat("a", 64)),
		ManagedClusterName: "cluster-a", DisplayName: "Cluster A",
	})
	if err != nil {
		t.Fatalf("CreateCluster() error = %v", err)
	}
	clusters, err := repository.ListClusters(ctx)
	if err != nil || len(clusters) != 1 || clusters[0].ID != created.ResourceID {
		t.Fatalf("ListClusters() = %#v, error = %v", clusters, err)
	}
	op, err := repository.GetByID(ctx, created.OperationID)
	if err != nil || op.Status != operation.StatusSucceeded || op.Progress != 100 {
		t.Fatalf("cluster operation = %#v, error = %v", op, err)
	}
	queued, err := repository.Claim(ctx, outbox.ClaimParams{WorkerID: "test", EventType: "operation.queued", Limit: 10, LeaseDuration: time.Minute})
	if err != nil || len(queued) != 0 {
		t.Fatalf("queued immediate operations = %d, error = %v", len(queued), err)
	}

	first, err := repository.ReplaceClusterInventory(ctx, catalog.ReplaceInventoryParams{
		Mutation: catalogMutation("inventory-replace-key-1", strings.Repeat("b", 64)), ClusterID: created.ResourceID,
		ExpectedGeneration: 0, SourceGeneration: strings.Repeat("c", 64), AgentEpoch: "epoch-0001", ReportSequence: 1,
		FencingToken: "fence-1", FencingEnabled: true, ExecutionHealthy: true, ObservedAt: fixedNow,
		NodePools: []catalog.NodePoolSnapshot{{Name: "pool-a", ManagementState: catalog.ManagementEnabled, Nodes: []catalog.NodeSnapshot{
			{OpaqueKey: "node-a", ManagementState: catalog.ManagementEnabled, HealthState: catalog.HealthHealthy, Schedulable: true, GPUDevices: []catalog.GPUDeviceSnapshot{
				{OpaqueKey: "gpu-a", ResourceClass: catalog.WholeGPUResourceClass, Model: "NVIDIA A100", MemoryMiB: 40960, AcceleratorMode: catalog.AcceleratorWhole, HealthState: catalog.HealthHealthy, Allocatable: true, Traits: map[string]string{"gpu.nvidia.com/product": "A100"}},
				{OpaqueKey: "gpu-b", ResourceClass: catalog.WholeGPUResourceClass, Model: "NVIDIA A100", MemoryMiB: 40960, AcceleratorMode: catalog.AcceleratorWhole, HealthState: catalog.HealthHealthy, Allocatable: true},
			}},
			{OpaqueKey: "node-b", ManagementState: catalog.ManagementEnabled, HealthState: catalog.HealthHealthy, Schedulable: true, GPUDevices: []catalog.GPUDeviceSnapshot{
				{OpaqueKey: "gpu-c", ResourceClass: catalog.WholeGPUResourceClass, Model: "NVIDIA L40S", MemoryMiB: 46080, AcceleratorMode: catalog.AcceleratorWhole, HealthState: catalog.HealthHealthy, Allocatable: true},
			}},
		}}},
	})
	if err != nil {
		t.Fatalf("ReplaceClusterInventory() error = %v", err)
	}
	if first.ResourceID != created.ResourceID {
		t.Fatalf("inventory acceptance = %#v", first)
	}
	inventory, err := repository.GetClusterInventory(ctx, created.ResourceID)
	if err != nil {
		t.Fatalf("GetClusterInventory() error = %v", err)
	}
	if inventory.Cluster.InventoryGeneration != 1 || !inventory.Cluster.Connected || !inventory.Cluster.Schedulable {
		t.Fatalf("cluster inventory state = %#v", inventory.Cluster)
	}
	if len(inventory.NodePools) != 1 || len(inventory.Nodes) != 2 || len(inventory.GPUDevices) != 3 {
		t.Fatalf("inventory counts = %d/%d/%d", len(inventory.NodePools), len(inventory.Nodes), len(inventory.GPUDevices))
	}
	var clusterTotal int64 = -1
	for _, item := range inventory.Inventories {
		if item.ResourceProviderID == created.ResourceID && item.ResourceClass == catalog.WholeGPUResourceClass {
			clusterTotal = item.Total
		}
	}
	if clusterTotal != 3 {
		t.Fatalf("cluster GPU total = %d, want 3", clusterTotal)
	}

	fixedNow = fixedNow.Add(time.Second)
	_, err = repository.ReplaceClusterInventory(ctx, catalog.ReplaceInventoryParams{
		Mutation: catalogMutation("inventory-heartbeat-key", strings.Repeat("4", 64)), ClusterID: created.ResourceID,
		ExpectedGeneration: 1, SourceGeneration: strings.Repeat("c", 64), AgentEpoch: "epoch-0001", ReportSequence: 2,
		FencingToken: "fence-1", FencingEnabled: true, ExecutionHealthy: true, ObservedAt: fixedNow,
	})
	if err != nil {
		t.Fatalf("unchanged ReplaceClusterInventory() error = %v", err)
	}
	inventory, err = repository.GetClusterInventory(ctx, created.ResourceID)
	if err != nil {
		t.Fatalf("unchanged GetClusterInventory() error = %v", err)
	}
	if inventory.Cluster.InventoryGeneration != 1 || inventory.Cluster.ReportSequence != 2 || inventory.Cluster.LastInventoryAt == nil || !inventory.Cluster.LastInventoryAt.Equal(fixedNow) {
		t.Fatalf("unchanged inventory advanced resource generation or missed heartbeat: %#v", inventory.Cluster)
	}

	lastDetailedAt := fixedNow
	if _, err := database.ExecContext(ctx, "UPDATE clusters SET management_state = 'disabled' WHERE id = $1", created.ResourceID); err != nil {
		t.Fatalf("disable cluster for heartbeat test: %v", err)
	}
	fixedNow = fixedNow.Add(time.Second)
	err = repository.ObserveClusterHeartbeat(ctx, catalog.ObserveClusterHeartbeatParams{
		ClusterID: created.ResourceID, AgentEpoch: "epoch-0001", ReportSequence: 3,
		FencingToken: "fence-1", FencingEnabled: true, ExecutionHealthy: true, ObservedAt: fixedNow,
	})
	if err != nil {
		t.Fatalf("ObserveClusterHeartbeat() error = %v", err)
	}
	inventory, err = repository.GetClusterInventory(ctx, created.ResourceID)
	if err != nil {
		t.Fatalf("heartbeat GetClusterInventory() error = %v", err)
	}
	if inventory.Cluster.InventoryGeneration != 1 || inventory.Cluster.SourceGeneration == nil || *inventory.Cluster.SourceGeneration != strings.Repeat("c", 64) ||
		inventory.Cluster.LastInventoryAt == nil || !inventory.Cluster.LastInventoryAt.Equal(lastDetailedAt) ||
		inventory.Cluster.LastHeartbeatAt == nil || !inventory.Cluster.LastHeartbeatAt.Equal(fixedNow) || inventory.Cluster.Schedulable {
		t.Fatalf("heartbeat changed inventory facts or ignored manual disable: %#v", inventory.Cluster)
	}
	if _, err := database.ExecContext(ctx, "UPDATE clusters SET management_state = 'enabled' WHERE id = $1", created.ResourceID); err != nil {
		t.Fatalf("enable cluster after heartbeat test: %v", err)
	}

	stale := catalog.ReplaceInventoryParams{
		Mutation: catalogMutation("inventory-stale-key", strings.Repeat("d", 64)), ClusterID: created.ResourceID,
		ExpectedGeneration: 1, SourceGeneration: strings.Repeat("e", 64), AgentEpoch: "epoch-0001", ReportSequence: 3,
		FencingToken: "fence-1", FencingEnabled: true, ExecutionHealthy: true, ObservedAt: fixedNow,
	}
	if _, err := repository.ReplaceClusterInventory(ctx, stale); !errors.Is(err, catalog.ErrStaleReport) {
		t.Fatalf("stale inventory error = %v", err)
	}

	fixedNow = fixedNow.Add(time.Second)
	_, err = repository.ReplaceClusterInventory(ctx, catalog.ReplaceInventoryParams{
		Mutation: catalogMutation("inventory-replace-key-2", strings.Repeat("f", 64)), ClusterID: created.ResourceID,
		ExpectedGeneration: 1, SourceGeneration: strings.Repeat("1", 64), AgentEpoch: "epoch-0001", ReportSequence: 4,
		FencingToken: "fence-1", FencingEnabled: true, ExecutionHealthy: true, ObservedAt: fixedNow,
		NodePools: []catalog.NodePoolSnapshot{{Name: "pool-a", ManagementState: catalog.ManagementEnabled, Nodes: []catalog.NodeSnapshot{{
			OpaqueKey: "node-a", ManagementState: catalog.ManagementEnabled, HealthState: catalog.HealthHealthy, Schedulable: true,
			GPUDevices: []catalog.GPUDeviceSnapshot{{OpaqueKey: "gpu-a", ResourceClass: catalog.WholeGPUResourceClass, Model: "NVIDIA A100", MemoryMiB: 40960, AcceleratorMode: catalog.AcceleratorWhole, HealthState: catalog.HealthHealthy, Allocatable: true}},
		}}}},
	})
	if err != nil {
		t.Fatalf("second ReplaceClusterInventory() error = %v", err)
	}
	inventory, err = repository.GetClusterInventory(ctx, created.ResourceID)
	if err != nil {
		t.Fatalf("second GetClusterInventory() error = %v", err)
	}
	clusterTotal = -1
	unreachable := 0
	for _, item := range inventory.Inventories {
		if item.ResourceProviderID == created.ResourceID && item.ResourceClass == catalog.WholeGPUResourceClass {
			clusterTotal = item.Total
		}
	}
	for _, item := range inventory.Nodes {
		if item.HealthState == catalog.HealthUnreachable {
			unreachable++
		}
	}
	if clusterTotal != 1 || unreachable != 1 {
		t.Fatalf("second inventory total/unreachable = %d/%d", clusterTotal, unreachable)
	}

	profile, err := repository.CreateAcceleratorProfile(ctx, catalog.CreateAcceleratorProfileParams{
		Mutation: catalogMutation("profile-create-key", strings.Repeat("2", 64)), Name: "One A100", Slug: "one-a100",
		AcceleratorMode: catalog.AcceleratorWhole, ResourceClass: catalog.WholeGPUResourceClass, GPUCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateAcceleratorProfile() error = %v", err)
	}
	capacity, err := repository.CreateCapacityPool(ctx, catalog.CreateCapacityPoolParams{
		Mutation: catalogMutation("capacity-create-key", strings.Repeat("3", 64)), Name: "Pool A whole GPUs",
		ClusterID: created.ResourceID, NodePoolID: inventory.NodePools[0].ID, AcceleratorProfileID: profile.ResourceID, SchedulerProfile: catalog.SchedulerNone,
	})
	if err != nil {
		t.Fatalf("CreateCapacityPool() error = %v", err)
	}
	capacityPool, err := repository.GetCapacityPool(ctx, capacity.ResourceID)
	if err != nil || capacityPool.Total != 1 {
		t.Fatalf("capacity pool = %#v, error = %v", capacityPool, err)
	}
}
