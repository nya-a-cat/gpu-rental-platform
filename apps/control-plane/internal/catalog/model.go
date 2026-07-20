package catalog

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrInvalid            = errors.New("invalid catalog request")
	ErrNotFound           = errors.New("catalog resource not found")
	ErrConflict           = errors.New("catalog resource conflict")
	ErrGenerationConflict = errors.New("inventory generation conflict")
	ErrStaleReport        = errors.New("stale inventory report")
)

const WholeGPUResourceClass = "gpu.nvidia.full"

type ManagementState string

const (
	ManagementEnabled     ManagementState = "enabled"
	ManagementDisabled    ManagementState = "disabled"
	ManagementDraining    ManagementState = "draining"
	ManagementMaintenance ManagementState = "maintenance"
	ManagementQuarantined ManagementState = "quarantined"
)

type HealthState string

const (
	HealthHealthy     HealthState = "healthy"
	HealthDegraded    HealthState = "degraded"
	HealthUnreachable HealthState = "unreachable"
	HealthFailed      HealthState = "failed"
	HealthUnknown     HealthState = "unknown"
)

type AcceleratorMode string

const (
	AcceleratorWhole       AcceleratorMode = "whole"
	AcceleratorMIG         AcceleratorMode = "mig"
	AcceleratorHAMi        AcceleratorMode = "hami"
	AcceleratorTimeSlicing AcceleratorMode = "time-slicing"
)

type SchedulerProfile string

const (
	SchedulerNone    SchedulerProfile = "none"
	SchedulerVolcano SchedulerProfile = "hpc-volcano"
	SchedulerKueue   SchedulerProfile = "standard-kueue"
)

type ResourceClass struct {
	Name        string    `json:"name"`
	Unit        string    `json:"unit"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Cluster struct {
	ID                  string          `json:"id"`
	ManagedClusterName  string          `json:"managedClusterName"`
	DisplayName         string          `json:"displayName"`
	ManagementState     ManagementState `json:"managementState"`
	ConnectionState     string          `json:"connectionState"`
	Connected           bool            `json:"connected"`
	Schedulable         bool            `json:"schedulable"`
	InventoryFresh      bool            `json:"inventoryFresh"`
	ExecutionHealthy    bool            `json:"executionHealthy"`
	Fenced              bool            `json:"fenced"`
	AgentEpoch          *string         `json:"agentEpoch,omitempty"`
	ReportSequence      uint64          `json:"reportSequence"`
	FencingEnabled      bool            `json:"fencingEnabled"`
	InventoryGeneration int64           `json:"inventoryGeneration"`
	SourceGeneration    *string         `json:"sourceGeneration,omitempty"`
	LastHeartbeatAt     *time.Time      `json:"lastHeartbeatAt,omitempty"`
	LastInventoryAt     *time.Time      `json:"lastInventoryAt,omitempty"`
	Conditions          json.RawMessage `json:"conditions"`
	Generation          int64           `json:"generation"`
	CreatedAt           time.Time       `json:"createdAt"`
	UpdatedAt           time.Time       `json:"updatedAt"`
}

type NodePool struct {
	ID                 string          `json:"id"`
	ClusterID          string          `json:"clusterId"`
	Name               string          `json:"name"`
	ManagementState    ManagementState `json:"managementState"`
	Generation         int64           `json:"generation"`
	LastSeenGeneration int64           `json:"lastSeenInventoryGeneration"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}

type Node struct {
	ID                 string            `json:"id"`
	ClusterID          string            `json:"clusterId"`
	NodePoolID         string            `json:"nodePoolId"`
	OpaqueKey          string            `json:"opaqueKey"`
	ManagementState    ManagementState   `json:"managementState"`
	HealthState        HealthState       `json:"healthState"`
	Schedulable        bool              `json:"schedulable"`
	Traits             map[string]string `json:"traits"`
	Generation         int64             `json:"generation"`
	LastSeenGeneration int64             `json:"lastSeenInventoryGeneration"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
}

type GPUDevice struct {
	ID                 string            `json:"id"`
	ClusterID          string            `json:"clusterId"`
	NodeID             string            `json:"nodeId"`
	OpaqueKey          string            `json:"opaqueKey"`
	ResourceClass      string            `json:"resourceClass"`
	Model              string            `json:"model"`
	MemoryMiB          int64             `json:"memoryMiB"`
	AcceleratorMode    AcceleratorMode   `json:"acceleratorMode"`
	HealthState        HealthState       `json:"healthState"`
	Allocatable        bool              `json:"allocatable"`
	Traits             map[string]string `json:"traits"`
	Generation         int64             `json:"generation"`
	LastSeenGeneration int64             `json:"lastSeenInventoryGeneration"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
}

type Inventory struct {
	ResourceProviderID string    `json:"resourceProviderId"`
	ResourceClass      string    `json:"resourceClass"`
	Total              int64     `json:"total"`
	Reserved           int64     `json:"reserved"`
	Allocated          int64     `json:"allocated"`
	Generation         int64     `json:"generation"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type ClusterInventory struct {
	Cluster     Cluster     `json:"cluster"`
	NodePools   []NodePool  `json:"nodePools"`
	Nodes       []Node      `json:"nodes"`
	GPUDevices  []GPUDevice `json:"gpuDevices"`
	Inventories []Inventory `json:"inventories"`
}

type AcceleratorProfile struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Slug            string            `json:"slug"`
	AcceleratorMode AcceleratorMode   `json:"acceleratorMode"`
	ResourceClass   string            `json:"resourceClass"`
	GPUCount        int               `json:"gpuCount"`
	MemoryMiB       *int64            `json:"memoryMiB,omitempty"`
	Traits          map[string]string `json:"traits"`
	Enabled         bool              `json:"enabled"`
	Generation      int64             `json:"generation"`
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`
}

type CapacityPool struct {
	ID                   string           `json:"id"`
	Name                 string           `json:"name"`
	ClusterID            string           `json:"clusterId"`
	NodePoolID           string           `json:"nodePoolId"`
	AcceleratorProfileID string           `json:"acceleratorProfileId"`
	SchedulerProfile     SchedulerProfile `json:"schedulerProfile"`
	Total                int64            `json:"total"`
	Reserved             int64            `json:"reserved"`
	Allocated            int64            `json:"allocated"`
	Enabled              bool             `json:"enabled"`
	Generation           int64            `json:"generation"`
	CreatedAt            time.Time        `json:"createdAt"`
	UpdatedAt            time.Time        `json:"updatedAt"`
}

type NodePoolSnapshot struct {
	Name            string          `json:"name"`
	ManagementState ManagementState `json:"managementState"`
	Nodes           []NodeSnapshot  `json:"nodes"`
}

type NodeSnapshot struct {
	OpaqueKey       string              `json:"opaqueKey"`
	ManagementState ManagementState     `json:"managementState"`
	HealthState     HealthState         `json:"healthState"`
	Schedulable     bool                `json:"schedulable"`
	Traits          map[string]string   `json:"traits"`
	GPUDevices      []GPUDeviceSnapshot `json:"gpuDevices"`
}

type GPUDeviceSnapshot struct {
	OpaqueKey       string            `json:"opaqueKey"`
	ResourceClass   string            `json:"resourceClass"`
	Model           string            `json:"model"`
	MemoryMiB       int64             `json:"memoryMiB"`
	AcceleratorMode AcceleratorMode   `json:"acceleratorMode"`
	HealthState     HealthState       `json:"healthState"`
	Allocatable     bool              `json:"allocatable"`
	Traits          map[string]string `json:"traits"`
}
