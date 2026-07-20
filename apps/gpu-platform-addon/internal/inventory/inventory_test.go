package inventory

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func readyCondition(status corev1.ConditionStatus) []corev1.NodeCondition {
	return []corev1.NodeCondition{{Type: corev1.NodeReady, Status: status}}
}

func TestBuildDetailedWholeGPUInventory(t *testing.T) {
	observedAt := time.Date(2026, 7, 19, 2, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "worker-a", Labels: map[string]string{
				labelNodePool: "gpu-a100", labelGPUProduct: "NVIDIA-A100-SXM4-40GB", labelGPUMemory: "40960",
				labelCUDAMajor: "12", labelCUDAMinor: "8", labelComputeMajor: "8", labelComputeMinor: "0",
				labelTopologyRegion: "cn-east", labelTopologyZone: "cn-east-1a", labelInstanceType: "gpu.a100.2",
			}},
			Status: corev1.NodeStatus{Conditions: readyCondition(corev1.ConditionTrue), Allocatable: corev1.ResourceList{
				corev1.ResourceName("nvidia.com/gpu"):        resource.MustParse("2"),
				corev1.ResourceName("nvidia.com/mig-1g.5gb"): resource.MustParse("1"),
				corev1.ResourceCPU:                           resource.MustParse("32"),
			}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "worker-b", Labels: map[string]string{
				labelNodePool: "gpu-a100", labelGPUProduct: "NVIDIA-A100-SXM4-40GB", labelGPUMemory: "40960",
			}},
			Spec: corev1.NodeSpec{Unschedulable: true},
			Status: corev1.NodeStatus{Conditions: readyCondition(corev1.ConditionTrue), Allocatable: corev1.ResourceList{
				corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
			}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "worker-c"},
			Status:     corev1.NodeStatus{Conditions: readyCondition(corev1.ConditionUnknown)},
		},
	}

	snapshot := Build("cluster-a", "addon-uid-secret", nodes, observedAt)
	if snapshot.SchemaVersion != SchemaVersion || snapshot.ObservedAt.Location() != time.UTC || !snapshot.ExecutionHealthy || snapshot.Fenced {
		t.Fatalf("unexpected snapshot metadata %#v", snapshot)
	}
	if snapshot.NodeCount != 3 || snapshot.SchedulableNodeCount != 1 {
		t.Fatalf("unexpected node totals: %#v", snapshot)
	}
	wantResources := []Resource{
		{Name: "nvidia.com/gpu", Allocatable: 3, SchedulableAllocatable: 2},
		{Name: "nvidia.com/mig-1g.5gb", Allocatable: 1, SchedulableAllocatable: 1},
	}
	if len(snapshot.Resources) != len(wantResources) {
		t.Fatalf("resources = %#v", snapshot.Resources)
	}
	for index := range wantResources {
		if snapshot.Resources[index] != wantResources[index] {
			t.Fatalf("resource %d = %#v, want %#v", index, snapshot.Resources[index], wantResources[index])
		}
	}
	if len(snapshot.NodePools) != 2 || snapshot.NodePools[0].Name != DefaultNodePool || snapshot.NodePools[1].Name != "gpu-a100" {
		t.Fatalf("node pools = %#v", snapshot.NodePools)
	}

	deviceCount := 0
	allocatableCount := 0
	for _, pool := range snapshot.NodePools {
		for _, node := range pool.Nodes {
			if node.OpaqueKey == "" || strings.Contains(node.OpaqueKey, "worker") {
				t.Fatalf("node identity is not opaque: %#v", node)
			}
			for _, device := range node.GPUDevices {
				deviceCount++
				if device.Allocatable {
					allocatableCount++
				}
				if device.ResourceClass != WholeGPUResourceClass || device.AcceleratorMode != "whole" || device.MemoryMiB != 40960 {
					t.Fatalf("unexpected device %#v", device)
				}
				if device.Traits["gpu.nvidia.com/model"] == "" || strings.Contains(device.OpaqueKey, "worker") {
					t.Fatalf("device traits or identity invalid: %#v", device)
				}
			}
		}
	}
	if deviceCount != 3 || allocatableCount != 2 {
		t.Fatalf("device counts = %d/%d, want 3/2", deviceCount, allocatableCount)
	}

	encoded, err := Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	for _, forbidden := range []string{"worker-a", "worker-b", "worker-c"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("serialized inventory exposes node name %q: %s", forbidden, encoded)
		}
	}
}

func TestDetailedIdentityAndGenerationAreStableWithinAddonEpoch(t *testing.T) {
	nodes := []corev1.Node{{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-a", Labels: map[string]string{labelGPUProduct: "NVIDIA L40S", labelGPUMemory: "46080"}},
		Status:     corev1.NodeStatus{Conditions: readyCondition(corev1.ConditionTrue), Allocatable: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("1")}},
	}}
	first := Build("cluster-a", "addon-uid", nodes, time.Unix(100, 0))
	second := Build("cluster-a", "addon-uid", nodes, time.Unix(200, 0))
	rotated := Build("cluster-a", "recreated-addon-uid", nodes, time.Unix(200, 0))
	firstNode := first.NodePools[0].Nodes[0]
	secondNode := second.NodePools[0].Nodes[0]
	rotatedNode := rotated.NodePools[0].Nodes[0]
	if first.Generation != second.Generation || firstNode.OpaqueKey != secondNode.OpaqueKey || firstNode.GPUDevices[0].OpaqueKey != secondNode.GPUDevices[0].OpaqueKey {
		t.Fatalf("stable inventory changed across observations: %#v %#v", first, second)
	}
	if firstNode.OpaqueKey == rotatedNode.OpaqueKey || first.Generation == rotated.Generation {
		t.Fatalf("recreated add-on retained opaque identities: %#v %#v", firstNode, rotatedNode)
	}
}

func TestBuildOmitsUnidentifiableLogicalGPUDevices(t *testing.T) {
	nodes := []corev1.Node{{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-a"},
		Status:     corev1.NodeStatus{Conditions: readyCondition(corev1.ConditionTrue), Allocatable: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("2")}},
	}}
	snapshot := Build("cluster-a", "addon-uid", nodes, time.Now())
	if got := len(snapshot.NodePools[0].Nodes[0].GPUDevices); got != 0 {
		t.Fatalf("devices without model and memory labels = %d, want 0", got)
	}
}

func TestMarshalInventoryIsStableAndContainsNoNodeIdentity(t *testing.T) {
	snapshot := Snapshot{
		SchemaVersion: SchemaVersion, ClusterName: "cluster-a", AgentEpoch: "0123456789abcdef0123456789abcdef",
		Sequence: 7, FencingToken: "uid-current", FencingEnabled: true, Generation: "generation-1",
		ObservedAt: time.Date(2026, 7, 18, 18, 0, 0, 0, time.UTC), ExecutionHealthy: true,
		NodeCount: 1, SchedulableNodeCount: 1,
		Resources: []Resource{{Name: "nvidia.com/gpu", Allocatable: 2, SchedulableAllocatable: 2}}, NodePools: []NodePool{},
	}
	data, err := Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal inventory: %v", err)
	}
	want := `{"schemaVersion":"gpu.platform.nyaacat.dev/v1alpha1","clusterName":"cluster-a","agentEpoch":"0123456789abcdef0123456789abcdef","sequence":7,"fencingToken":"uid-current","fencingEnabled":true,"generation":"generation-1","observedAt":"2026-07-18T18:00:00Z","executionHealthy":true,"fenced":false,"nodeCount":1,"schedulableNodeCount":1,"resources":[{"name":"nvidia.com/gpu","allocatable":2,"schedulableAllocatable":2}],"nodePools":[]}`
	if string(data) != want {
		t.Fatalf("unexpected serialized inventory:\nwant %s\n got %s", want, data)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode inventory: %v", err)
	}
	for _, forbidden := range []string{"nodeNames", "deviceIDs", "gpuIDs"} {
		if _, exists := decoded[forbidden]; exists {
			t.Fatalf("serialized inventory exposes forbidden identity field %q", forbidden)
		}
	}
}
