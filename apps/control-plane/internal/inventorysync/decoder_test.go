package inventorysync

import (
	"strings"
	"testing"
)

func TestDecodeDetailedInventory(t *testing.T) {
	payload := `{
		"schemaVersion":"gpu.platform.nyaacat.dev/v1alpha1",
		"clusterName":"cluster-a",
		"agentEpoch":"0123456789abcdef0123456789abcdef",
		"sequence":7,
		"fencingToken":"addon-uid",
		"fencingEnabled":true,
		"generation":"` + strings.Repeat("a", 64) + `",
		"observedAt":"2026-07-20T12:00:00Z",
		"executionHealthy":true,
		"fenced":false,
		"nodeCount":1,
		"schedulableNodeCount":1,
		"resources":[{"name":"nvidia.com/gpu","allocatable":1,"schedulableAllocatable":1}],
		"nodePools":[{"name":"gpu-a100","managementState":"enabled","nodes":[{
			"opaqueKey":"node-0123456789abcdef",
			"managementState":"enabled",
			"healthState":"healthy",
			"schedulable":true,
			"traits":{"topology.kubernetes.io/zone":"zone-a"},
			"gpuDevices":[{
				"opaqueKey":"gpu-0123456789abcdef",
				"resourceClass":"gpu.nvidia.full",
				"model":"NVIDIA A100",
				"memoryMiB":40960,
				"acceleratorMode":"whole",
				"healthState":"healthy",
				"allocatable":true,
				"traits":{"gpu.nvidia.com/model":"NVIDIA A100"}
			}]
		}]}]
	}`
	report, err := Decode([]byte(payload), "cluster-a")
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if !report.Detailed || report.Sequence != 7 || len(report.NodePools) != 1 || len(report.NodePools[0].Nodes) != 1 || len(report.NodePools[0].Nodes[0].GPUDevices) != 1 {
		t.Fatalf("Decode() report = %#v", report)
	}
	device := report.NodePools[0].Nodes[0].GPUDevices[0]
	if device.Model != "NVIDIA A100" || device.MemoryMiB != 40960 || !device.Allocatable {
		t.Fatalf("Decode() device = %#v", device)
	}
}

func TestDecodeLegacyAggregateAsHeartbeat(t *testing.T) {
	payload := `{
		"schemaVersion":"gpu.platform.nyaacat.dev/v1alpha1",
		"clusterName":"cluster-a",
		"agentEpoch":"0123456789abcdef0123456789abcdef",
		"sequence":8,
		"fencingToken":"addon-uid",
		"fencingEnabled":true,
		"generation":"legacy-generation",
		"observedAt":"2026-07-20T12:00:15Z",
		"executionHealthy":true,
		"fenced":false,
		"nodeCount":1,
		"schedulableNodeCount":1,
		"resources":[{"name":"nvidia.com/gpu","allocatable":1,"schedulableAllocatable":1}]
	}`
	report, err := Decode([]byte(payload), "cluster-a")
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if report.Detailed || report.Sequence != 8 || len(report.NodePools) != 0 {
		t.Fatalf("Decode() legacy report = %#v", report)
	}
}

func TestDecodeRejectsIdentityAndUnknownFields(t *testing.T) {
	base := `{"schemaVersion":"gpu.platform.nyaacat.dev/v1alpha1","clusterName":"cluster-a","agentEpoch":"01234567","sequence":1,"generation":"legacy","observedAt":"2026-07-20T12:00:00Z","executionHealthy":true,"unexpected":true}`
	if _, err := Decode([]byte(base), "cluster-a"); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Decode() unknown field error = %v", err)
	}
	identity := strings.Replace(base, `,"unexpected":true`, "", 1)
	if _, err := Decode([]byte(identity), "cluster-b"); err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("Decode() identity error = %v", err)
	}
}
func TestDecodeRejectsNullDetailedInventory(t *testing.T) {
	payload := `{"schemaVersion":"gpu.platform.nyaacat.dev/v1alpha1","clusterName":"cluster-a","agentEpoch":"01234567","sequence":1,"generation":"legacy","observedAt":"2026-07-20T12:00:00Z","executionHealthy":true,"nodePools":null}`
	if _, err := Decode([]byte(payload), "cluster-a"); err == nil || !strings.Contains(err.Error(), "must be an array") {
		t.Fatalf("Decode() null nodePools error = %v", err)
	}
}
