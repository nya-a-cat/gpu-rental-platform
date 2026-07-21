package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/ports"
)

type ManifestInput struct {
	Workspace   Workspace
	OperationID string
}

func WorkName(id string) string {
	compact := strings.ReplaceAll(id, "-", "")
	if len(compact) > 12 {
		compact = compact[:12]
	}
	return "gpu-workspace-" + compact
}

func BuildWork(input ManifestInput) (ports.WorkRequest, error) {
	if input.Workspace.ID == "" || input.Workspace.ClusterID == "" || input.Workspace.NamespaceName == "" {
		return ports.WorkRequest{}, errors.New("workspace identity and placement are required")
	}
	if input.Workspace.GPUCount <= 0 {
		return ports.WorkRequest{}, errors.New("workspace GPU count must be positive")
	}
	if input.Workspace.StorageGiB <= 0 {
		return ports.WorkRequest{}, errors.New("workspace storage capacity must be positive")
	}
	name := WorkName(input.Workspace.ID)
	labels := map[string]any{
		"app.kubernetes.io/managed-by":          "gpu-cloud-control-plane",
		"gpu.platform.nyaacat.dev/workspace-id": input.Workspace.ID,
	}
	resources := map[string]any{
		"requests": map[string]any{"nvidia.com/gpu": strconv.Itoa(input.Workspace.GPUCount)},
		"limits":   map[string]any{"nvidia.com/gpu": strconv.Itoa(input.Workspace.GPUCount)},
	}
	manifests := []any{}
	if input.Workspace.DesiredState != DesiredTerminated {
		manifests = append(manifests, map[string]any{
			"apiVersion": "v1", "kind": "PersistentVolumeClaim",
			"metadata": map[string]any{"name": name + "-data", "namespace": input.Workspace.NamespaceName, "labels": labels},
			"spec": map[string]any{
				"accessModes": []any{"ReadWriteOnce"},
				"resources":   map[string]any{"requests": map[string]any{"storage": fmt.Sprintf("%dGi", input.Workspace.StorageGiB)}},
			},
		})
		manifests = append(manifests,
			map[string]any{
				"apiVersion": "gateway.networking.k8s.io/v1beta1", "kind": "ReferenceGrant",
				"metadata": map[string]any{"name": name + "-gateway", "namespace": "gateway-system", "labels": labels},
				"spec": map[string]any{
					"from": []any{map[string]any{"group": "gateway.networking.k8s.io", "kind": "HTTPRoute", "namespace": input.Workspace.NamespaceName}},
					"to":   []any{map[string]any{"group": "gateway.networking.k8s.io", "kind": "Gateway"}},
				},
			},
			map[string]any{
				"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy",
				"metadata": map[string]any{"name": name + "-default-deny", "namespace": input.Workspace.NamespaceName, "labels": labels},
				"spec":     map[string]any{"podSelector": map[string]any{"matchLabels": labels}, "policyTypes": []any{"Ingress", "Egress"}},
			},
			map[string]any{
				"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy",
				"metadata": map[string]any{"name": name + "-allow-internal", "namespace": input.Workspace.NamespaceName, "labels": labels},
				"spec": map[string]any{
					"podSelector": map[string]any{"matchLabels": labels}, "policyTypes": []any{"Ingress"},
					"ingress": []any{map[string]any{"from": []any{map[string]any{"podSelector": map[string]any{"matchLabels": labels}}}}},
				},
			},
			map[string]any{
				"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy",
				"metadata": map[string]any{"name": name + "-allow-dns", "namespace": input.Workspace.NamespaceName, "labels": labels},
				"spec": map[string]any{
					"podSelector": map[string]any{"matchLabels": labels}, "policyTypes": []any{"Egress"},
					"egress": []any{map[string]any{
						"to":    []any{map[string]any{"namespaceSelector": map[string]any{"matchLabels": map[string]any{"kubernetes.io/metadata.name": "kube-system"}}, "podSelector": map[string]any{"matchLabels": map[string]any{"k8s-app": "kube-dns"}}}},
						"ports": []any{map[string]any{"port": 53, "protocol": "UDP"}, map[string]any{"port": 53, "protocol": "TCP"}},
					}},
				},
			},
		)
		for _, snapshot := range input.Workspace.Snapshots {
			if snapshot.State == "failed" {
				continue
			}
			manifests = append(manifests, map[string]any{
				"apiVersion": "snapshot.storage.k8s.io/v1", "kind": "VolumeSnapshot",
				"metadata": map[string]any{"name": snapshot.Name, "namespace": input.Workspace.NamespaceName, "labels": labels},
				"spec":     map[string]any{"source": map[string]any{"persistentVolumeClaimName": snapshot.SourcePVCName}},
			})
		}
	}
	if input.Workspace.DesiredState == DesiredRunning {
		manifests = append(manifests, map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "StatefulSet",
			"metadata":   map[string]any{"name": name, "namespace": input.Workspace.NamespaceName, "labels": labels},
			"spec": map[string]any{
				"serviceName": name,
				"replicas":    1,
				"selector":    map[string]any{"matchLabels": labels},
				"template": map[string]any{
					"metadata": map[string]any{"labels": labels},
					"spec": map[string]any{
						"containers": []any{map[string]any{
							"name": "workspace", "image": "nvidia/cuda:12.6.3-base-ubuntu24.04", "command": []any{"sleep", "infinity"}, "resources": resources,
							"volumeMounts": []any{map[string]any{"name": "workspace-data", "mountPath": "/workspace"}},
						}},
						"volumes": []any{map[string]any{"name": "workspace-data", "persistentVolumeClaim": map[string]any{"claimName": name + "-data"}}},
					},
				},
			},
		})
		manifests = append(manifests, map[string]any{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]any{"name": name, "namespace": input.Workspace.NamespaceName, "labels": labels},
			"spec": map[string]any{
				"clusterIP": "None", "selector": labels,
				"ports": []any{
					map[string]any{"name": "ssh", "port": 22, "protocol": "TCP", "targetPort": 22},
					map[string]any{"name": "web-terminal", "port": 7681, "protocol": "TCP", "targetPort": 7681},
					map[string]any{"name": "jupyter", "port": 8888, "protocol": "TCP", "targetPort": 8888},
				},
			},
		})
		manifests = append(manifests,
			map[string]any{
				"apiVersion": "gateway.networking.k8s.io/v1", "kind": "HTTPRoute",
				"metadata": map[string]any{"name": name + "-jupyter", "namespace": input.Workspace.NamespaceName, "labels": labels},
				"spec": map[string]any{
					"parentRefs": []any{map[string]any{"name": "gpu-platform-gateway", "namespace": "gateway-system"}},
					"rules":      []any{map[string]any{"matches": []any{map[string]any{"path": map[string]any{"type": "PathPrefix", "value": "/jupyter"}}}, "backendRefs": []any{map[string]any{"name": name, "port": 8888}}}},
				},
			},
			map[string]any{
				"apiVersion": "gateway.networking.k8s.io/v1", "kind": "HTTPRoute",
				"metadata": map[string]any{"name": name + "-terminal", "namespace": input.Workspace.NamespaceName, "labels": labels},
				"spec": map[string]any{
					"parentRefs": []any{map[string]any{"name": "gpu-platform-gateway", "namespace": "gateway-system"}},
					"rules":      []any{map[string]any{"matches": []any{map[string]any{"path": map[string]any{"type": "PathPrefix", "value": "/terminal"}}}, "backendRefs": []any{map[string]any{"name": name, "port": 7681}}}},
				},
			},
		)
	}
	work := map[string]any{
		"apiVersion": "work.open-cluster-management.io/v1", "kind": "ManifestWork",
		"metadata": map[string]any{
			"name": name, "namespace": input.Workspace.ClusterID, "labels": labels,
			"annotations": map[string]any{"gpu.platform.nyaacat.dev/operation-id": input.OperationID},
		},
		"spec": map[string]any{"workload": map[string]any{"manifests": manifests}},
	}
	encoded, err := json.Marshal(work)
	if err != nil {
		return ports.WorkRequest{}, fmt.Errorf("encode workspace ManifestWork: %w", err)
	}
	return ports.WorkRequest{OperationID: input.OperationID, ClusterID: input.Workspace.ClusterID, WorkID: name, Manifest: encoded}, nil
}
