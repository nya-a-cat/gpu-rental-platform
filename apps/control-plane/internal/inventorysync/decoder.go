package inventorysync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/catalog"
)

const (
	SchemaVersion          = "gpu.platform.nyaacat.dev/v1alpha1"
	maxInventoryReportSize = 1 << 20
)

type Report struct {
	ClusterName      string
	AgentEpoch       string
	Sequence         uint64
	FencingToken     string
	FencingEnabled   bool
	SourceGeneration string
	ObservedAt       time.Time
	ExecutionHealthy bool
	Fenced           bool
	Detailed         bool
	NodePools        []catalog.NodePoolSnapshot
}

type wireReport struct {
	SchemaVersion        string          `json:"schemaVersion"`
	ClusterName          string          `json:"clusterName"`
	AgentEpoch           string          `json:"agentEpoch"`
	Sequence             uint64          `json:"sequence"`
	FencingToken         string          `json:"fencingToken"`
	FencingEnabled       bool            `json:"fencingEnabled"`
	Generation           string          `json:"generation"`
	ObservedAt           time.Time       `json:"observedAt"`
	ExecutionHealthy     bool            `json:"executionHealthy"`
	Fenced               bool            `json:"fenced"`
	NodeCount            int             `json:"nodeCount"`
	SchedulableNodeCount int             `json:"schedulableNodeCount"`
	Resources            []wireResource  `json:"resources"`
	NodePools            json.RawMessage `json:"nodePools"`
}

type wireResource struct {
	Name                   string `json:"name"`
	Allocatable            int64  `json:"allocatable"`
	SchedulableAllocatable int64  `json:"schedulableAllocatable"`
}

type wireNodePool struct {
	Name            string                  `json:"name"`
	ManagementState catalog.ManagementState `json:"managementState"`
	Nodes           []wireNode              `json:"nodes"`
}

type wireNode struct {
	OpaqueKey       string                  `json:"opaqueKey"`
	ManagementState catalog.ManagementState `json:"managementState"`
	HealthState     catalog.HealthState     `json:"healthState"`
	Schedulable     bool                    `json:"schedulable"`
	Traits          map[string]string       `json:"traits"`
	GPUDevices      []wireGPUDevice         `json:"gpuDevices"`
}

type wireGPUDevice struct {
	OpaqueKey       string                  `json:"opaqueKey"`
	ResourceClass   string                  `json:"resourceClass"`
	Model           string                  `json:"model"`
	MemoryMiB       int64                   `json:"memoryMiB"`
	AcceleratorMode catalog.AcceleratorMode `json:"acceleratorMode"`
	HealthState     catalog.HealthState     `json:"healthState"`
	Allocatable     bool                    `json:"allocatable"`
	Traits          map[string]string       `json:"traits"`
}

func Decode(payload []byte, expectedClusterName string) (Report, error) {
	if len(payload) == 0 {
		return Report{}, errors.New("inventory report is empty")
	}
	if len(payload) > maxInventoryReportSize {
		return Report{}, errors.New("inventory report exceeds the size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var wire wireReport
	if err := decoder.Decode(&wire); err != nil {
		return Report{}, fmt.Errorf("decode inventory report: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Report{}, err
	}
	if wire.SchemaVersion != SchemaVersion {
		return Report{}, fmt.Errorf("unsupported inventory schema version %q", wire.SchemaVersion)
	}
	if strings.TrimSpace(expectedClusterName) == "" || wire.ClusterName != expectedClusterName {
		return Report{}, errors.New("inventory report cluster identity does not match the registered cluster")
	}

	detailed := len(wire.NodePools) > 0
	var wirePools []wireNodePool
	if detailed {
		if bytes.Equal(bytes.TrimSpace(wire.NodePools), []byte("null")) {
			return Report{}, errors.New("detailed inventory nodePools must be an array")
		}
		poolDecoder := json.NewDecoder(bytes.NewReader(wire.NodePools))
		poolDecoder.DisallowUnknownFields()
		if err := poolDecoder.Decode(&wirePools); err != nil {
			return Report{}, fmt.Errorf("decode detailed inventory node pools: %w", err)
		}
		if err := requireJSONEOF(poolDecoder); err != nil {
			return Report{}, fmt.Errorf("decode detailed inventory node pools: %w", err)
		}
	}
	report := Report{
		ClusterName: wire.ClusterName, AgentEpoch: wire.AgentEpoch, Sequence: wire.Sequence,
		FencingToken: wire.FencingToken, FencingEnabled: wire.FencingEnabled,
		SourceGeneration: wire.Generation, ObservedAt: wire.ObservedAt.UTC(),
		ExecutionHealthy: wire.ExecutionHealthy, Fenced: wire.Fenced,
		Detailed: detailed, NodePools: []catalog.NodePoolSnapshot{},
	}
	heartbeat := catalog.ObserveClusterHeartbeatParams{
		AgentEpoch: report.AgentEpoch, ReportSequence: report.Sequence,
		FencingToken: report.FencingToken, FencingEnabled: report.FencingEnabled,
		ExecutionHealthy: report.ExecutionHealthy, Fenced: report.Fenced, ObservedAt: report.ObservedAt,
	}
	if err := catalog.ValidateHeartbeat(heartbeat); err != nil {
		return Report{}, fmt.Errorf("validate inventory report metadata: %w", err)
	}
	if !detailed {
		return report, nil
	}
	for _, pool := range wirePools {
		mappedPool := catalog.NodePoolSnapshot{Name: pool.Name, ManagementState: pool.ManagementState, Nodes: []catalog.NodeSnapshot{}}
		for _, node := range pool.Nodes {
			mappedNode := catalog.NodeSnapshot{
				OpaqueKey: node.OpaqueKey, ManagementState: node.ManagementState, HealthState: node.HealthState,
				Schedulable: node.Schedulable, Traits: node.Traits, GPUDevices: []catalog.GPUDeviceSnapshot{},
			}
			for _, device := range node.GPUDevices {
				mappedNode.GPUDevices = append(mappedNode.GPUDevices, catalog.GPUDeviceSnapshot{
					OpaqueKey: device.OpaqueKey, ResourceClass: device.ResourceClass, Model: device.Model,
					MemoryMiB: device.MemoryMiB, AcceleratorMode: device.AcceleratorMode,
					HealthState: device.HealthState, Allocatable: device.Allocatable, Traits: device.Traits,
				})
			}
			mappedPool.Nodes = append(mappedPool.Nodes, mappedNode)
		}
		report.NodePools = append(report.NodePools, mappedPool)
	}
	if err := catalog.ValidateInventory(catalog.ReplaceInventoryParams{
		ExpectedGeneration: 0, SourceGeneration: report.SourceGeneration,
		AgentEpoch: report.AgentEpoch, ReportSequence: report.Sequence,
		FencingToken: report.FencingToken, FencingEnabled: report.FencingEnabled,
		ExecutionHealthy: report.ExecutionHealthy, Fenced: report.Fenced,
		ObservedAt: report.ObservedAt, NodePools: report.NodePools,
	}); err != nil {
		return Report{}, fmt.Errorf("validate detailed inventory report: %w", err)
	}
	return report, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode trailing inventory report data: %w", err)
	}
	return errors.New("inventory report contains multiple JSON values")
}
