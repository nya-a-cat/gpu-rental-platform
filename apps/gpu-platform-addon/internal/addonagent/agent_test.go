package addonagent

import (
	"context"
	"testing"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/gpu-platform-addon/internal/inventory"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestReporterOwnsInventoryConfigMapWhenAddonUIDIsAvailable(t *testing.T) {
	managedClient := fake.NewSimpleClientset(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
			"nvidia.com/gpu": resource.MustParse("1"),
		}},
	})
	hubClient := fake.NewSimpleClientset()
	reporter := &reporter{
		managedClient: managedClient,
		hubClient:     hubClient,
		clusterName:   "cluster-a",
		addonName:     "gpu-platform-addon",
		addonUID:      "uid-current",
	}
	observedAt := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)

	if err := reporter.report(context.Background(), observedAt); err != nil {
		t.Fatalf("report inventory: %v", err)
	}
	configMap, err := hubClient.CoreV1().ConfigMaps("cluster-a").Get(
		context.Background(), inventory.ConfigMapName, metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("get inventory ConfigMap: %v", err)
	}
	if len(configMap.OwnerReferences) != 1 {
		t.Fatalf("expected one owner reference, got %#v", configMap.OwnerReferences)
	}
	owner := configMap.OwnerReferences[0]
	if owner.APIVersion != "addon.open-cluster-management.io/v1beta1" || owner.Kind != "ManagedClusterAddOn" || owner.Name != "gpu-platform-addon" || string(owner.UID) != "uid-current" {
		t.Fatalf("unexpected owner reference %#v", owner)
	}
	if owner.Controller == nil || !*owner.Controller {
		t.Fatalf("expected controller owner reference, got %#v", owner)
	}

	reporter.addonUID = "uid-recreated"
	if err := reporter.report(context.Background(), observedAt.Add(time.Minute)); err != nil {
		t.Fatalf("update inventory: %v", err)
	}
	configMap, err = hubClient.CoreV1().ConfigMaps("cluster-a").Get(
		context.Background(), inventory.ConfigMapName, metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("get updated inventory ConfigMap: %v", err)
	}
	if len(configMap.OwnerReferences) != 1 || string(configMap.OwnerReferences[0].UID) != "uid-recreated" {
		t.Fatalf("expected recreated add-on ownership, got %#v", configMap.OwnerReferences)
	}
}

func TestReporterKeepsNMinusOneManagerCompatibilityWithoutAddonUID(t *testing.T) {
	managedClient := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}})
	hubClient := fake.NewSimpleClientset()
	reporter := &reporter{
		managedClient: managedClient,
		hubClient:     hubClient,
		clusterName:   "cluster-a",
		addonName:     "gpu-platform-addon",
	}

	if err := reporter.report(context.Background(), time.Now()); err != nil {
		t.Fatalf("report inventory without add-on UID: %v", err)
	}
	configMap, err := hubClient.CoreV1().ConfigMaps("cluster-a").Get(
		context.Background(), inventory.ConfigMapName, metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("get inventory ConfigMap: %v", err)
	}
	if len(configMap.OwnerReferences) != 0 {
		t.Fatalf("expected no owner reference for N-1 manager compatibility, got %#v", configMap.OwnerReferences)
	}
}

func TestReporterPreservesInventoryOwnershipWithoutAddonUID(t *testing.T) {
	controller := true
	managedClient := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}})
	hubClient := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      inventory.ConfigMapName,
			Namespace: "cluster-a",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "addon.open-cluster-management.io/v1beta1",
				Kind:       "ManagedClusterAddOn",
				Name:       "gpu-platform-addon",
				UID:        "uid-current",
				Controller: &controller,
			}},
		},
	})
	reporter := &reporter{
		managedClient: managedClient,
		hubClient:     hubClient,
		clusterName:   "cluster-a",
		addonName:     "gpu-platform-addon",
	}

	if err := reporter.report(context.Background(), time.Now()); err != nil {
		t.Fatalf("update owned inventory without add-on UID: %v", err)
	}
	configMap, err := hubClient.CoreV1().ConfigMaps("cluster-a").Get(
		context.Background(), inventory.ConfigMapName, metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("get updated inventory ConfigMap: %v", err)
	}
	if len(configMap.OwnerReferences) != 1 || string(configMap.OwnerReferences[0].UID) != "uid-current" {
		t.Fatalf("expected existing ownership to remain, got %#v", configMap.OwnerReferences)
	}
}
