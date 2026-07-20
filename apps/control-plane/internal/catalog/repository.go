package catalog

import (
	"context"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

type CreateClusterParams struct {
	Mutation           tenancy.MutationContext
	ManagedClusterName string
	DisplayName        string
}

type ReplaceInventoryParams struct {
	Mutation           tenancy.MutationContext
	ClusterID          string
	ExpectedGeneration int64
	SourceGeneration   string
	AgentEpoch         string
	ReportSequence     uint64
	FencingToken       string
	FencingEnabled     bool
	ExecutionHealthy   bool
	Fenced             bool
	ObservedAt         time.Time
	NodePools          []NodePoolSnapshot
}

type CreateAcceleratorProfileParams struct {
	Mutation        tenancy.MutationContext
	Name            string
	Slug            string
	AcceleratorMode AcceleratorMode
	ResourceClass   string
	GPUCount        int
	MemoryMiB       *int64
	Traits          map[string]string
}

type CreateCapacityPoolParams struct {
	Mutation             tenancy.MutationContext
	Name                 string
	ClusterID            string
	NodePoolID           string
	AcceleratorProfileID string
	SchedulerProfile     SchedulerProfile
}

type Repository interface {
	GetResourceClass(context.Context, string) (ResourceClass, error)
	CreateCluster(context.Context, CreateClusterParams) (tenancy.Acceptance, error)
	GetCluster(context.Context, string) (Cluster, error)
	ReplaceClusterInventory(context.Context, ReplaceInventoryParams) (tenancy.Acceptance, error)
	GetClusterInventory(context.Context, string) (ClusterInventory, error)
	CreateAcceleratorProfile(context.Context, CreateAcceleratorProfileParams) (tenancy.Acceptance, error)
	GetAcceleratorProfile(context.Context, string) (AcceleratorProfile, error)
	CreateCapacityPool(context.Context, CreateCapacityPoolParams) (tenancy.Acceptance, error)
	GetCapacityPool(context.Context, string) (CapacityPool, error)
}
