package inventory

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestGenerationIsStableForEquivalentInventory(t *testing.T) {
	nodes := []corev1.Node{
		{
			Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
				corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("2"),
			}},
		},
	}

	first := Aggregate("cluster-a", nodes, time.Unix(100, 0))
	second := Aggregate("cluster-a", nodes, time.Unix(200, 0))
	if first.Generation == "" {
		t.Fatal("generation must not be empty")
	}
	if first.Generation != second.Generation {
		t.Fatalf("equivalent inventory produced different generations: %q and %q", first.Generation, second.Generation)
	}
}

func TestGenerationChangesWhenCapacityChanges(t *testing.T) {
	baseNodes := []corev1.Node{
		{
			Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
				corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("2"),
			}},
		},
	}
	changedNodes := baseNodes[0].DeepCopy()
	changedNodes.Status.Allocatable[corev1.ResourceName("nvidia.com/gpu")] = resource.MustParse("3")

	base := Aggregate("cluster-a", baseNodes, time.Unix(100, 0))
	changed := Aggregate("cluster-a", []corev1.Node{*changedNodes}, time.Unix(100, 0))
	if base.Generation == changed.Generation {
		t.Fatalf("capacity change retained generation %q", base.Generation)
	}
}
