package inventory

import (
	"encoding/json"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestAggregateAcceleratorInventory(t *testing.T) {
	observedAt := time.Date(2026, 7, 19, 2, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	nodes := []corev1.Node{
		{
			Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
				corev1.ResourceName("nvidia.com/gpu"):        resource.MustParse("4"),
				corev1.ResourceName("nvidia.com/mig-1g.5gb"): resource.MustParse("2"),
				corev1.ResourceCPU:                           resource.MustParse("32"),
			}},
		},
		{
			Spec: corev1.NodeSpec{Unschedulable: true},
			Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
				corev1.ResourceName("nvidia.com/gpu"):        resource.MustParse("8"),
				corev1.ResourceName("nvidia.com/mig-1g.5gb"): resource.MustParse("1"),
				corev1.ResourceName("example.com/fpga"):      resource.MustParse("3"),
			}},
		},
	}

	snapshot := Aggregate("cluster-a", nodes, observedAt)
	if snapshot.SchemaVersion != SchemaVersion {
		t.Fatalf("unexpected schema version %q", snapshot.SchemaVersion)
	}
	if snapshot.ObservedAt.Location() != time.UTC {
		t.Fatalf("observed time was not normalized to UTC: %s", snapshot.ObservedAt.Location())
	}
	if snapshot.NodeCount != 2 || snapshot.SchedulableNodeCount != 1 {
		t.Fatalf("unexpected node totals: %#v", snapshot)
	}
	if len(snapshot.Resources) != 2 {
		t.Fatalf("expected only NVIDIA GPU and MIG resources, got %#v", snapshot.Resources)
	}

	want := []Resource{
		{Name: "nvidia.com/gpu", Allocatable: 12, SchedulableAllocatable: 4},
		{Name: "nvidia.com/mig-1g.5gb", Allocatable: 3, SchedulableAllocatable: 2},
	}
	for i := range want {
		if snapshot.Resources[i] != want[i] {
			t.Fatalf("resource %d: want %#v, got %#v", i, want[i], snapshot.Resources[i])
		}
	}
}

func TestMarshalInventoryIsStableAndContainsNoNodeIdentity(t *testing.T) {
	snapshot := Snapshot{
		SchemaVersion:        SchemaVersion,
		ClusterName:          "cluster-a",
		AgentEpoch:           "0123456789abcdef0123456789abcdef",
		Sequence:             7,
		FencingToken:         "uid-current",
		FencingEnabled:       true,
		Generation:           "generation-1",
		ObservedAt:           time.Date(2026, 7, 18, 18, 0, 0, 0, time.UTC),
		NodeCount:            1,
		SchedulableNodeCount: 1,
		Resources: []Resource{
			{Name: "nvidia.com/gpu", Allocatable: 2, SchedulableAllocatable: 2},
		},
	}

	data, err := Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal inventory: %v", err)
	}
	want := `{"schemaVersion":"gpu.platform.nyaacat.dev/v1alpha1","clusterName":"cluster-a","agentEpoch":"0123456789abcdef0123456789abcdef","sequence":7,"fencingToken":"uid-current","fencingEnabled":true,"generation":"generation-1","observedAt":"2026-07-18T18:00:00Z","nodeCount":1,"schedulableNodeCount":1,"resources":[{"name":"nvidia.com/gpu","allocatable":2,"schedulableAllocatable":2}]}`
	if string(data) != want {
		t.Fatalf("unexpected serialized inventory:\nwant %s\n got %s", want, data)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode inventory: %v", err)
	}
	for _, forbidden := range []string{"nodes", "nodeNames", "deviceIDs", "gpuIDs"} {
		if _, exists := decoded[forbidden]; exists {
			t.Fatalf("serialized inventory exposes forbidden identity field %q", forbidden)
		}
	}
}
