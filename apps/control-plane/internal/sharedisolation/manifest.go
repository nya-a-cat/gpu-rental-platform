package sharedisolation

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/ports"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

const (
	GPUQuotaResourceClass = "gpu.nvidia.full"
	gpuQuotaResourceName  = "requests.nvidia.com/gpu"
)

type ManifestInput struct {
	Project               tenancy.Project
	OperationID           string
	ClusterID             string
	GPUQuota              int64
	AddonInstallNamespace string
	AddonServiceAccount   string
}

func BuildWork(input ManifestInput) (ports.WorkRequest, error) {
	if input.Project.ID == "" || input.Project.TenantID == "" || input.Project.NamespaceName == "" {
		return ports.WorkRequest{}, errors.New("project identity and namespace are required")
	}
	if input.Project.IsolationClass != tenancy.IsolationShared {
		return ports.WorkRequest{}, errors.New("shared isolation renderer requires a shared project")
	}
	if input.GPUQuota < 0 {
		return ports.WorkRequest{}, errors.New("GPU quota must be non-negative")
	}
	if strings.TrimSpace(input.ClusterID) == "" ||
		strings.TrimSpace(input.AddonInstallNamespace) == "" ||
		strings.TrimSpace(input.AddonServiceAccount) == "" {
		return ports.WorkRequest{}, errors.New("cluster and Add-on identities are required")
	}

	workName := WorkName(input.Project.ID)
	labels := map[string]any{
		"app.kubernetes.io/managed-by":        "gpu-cloud-control-plane",
		"gpu.platform.nyaacat.dev/project-id": input.Project.ID,
		"gpu.platform.nyaacat.dev/tenant-id":  input.Project.TenantID,
		"gpu.platform.nyaacat.dev/isolation":  string(input.Project.IsolationClass),
	}
	namespacedMetadata := func(name string) map[string]any {
		return map[string]any{
			"name":      name,
			"namespace": input.Project.NamespaceName,
			"labels":    labels,
		}
	}

	manifests := []any{
		map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]any{
				"name": input.Project.NamespaceName,
				"labels": mergeMaps(labels, map[string]any{
					"pod-security.kubernetes.io/enforce":         "restricted",
					"pod-security.kubernetes.io/enforce-version": "v1.34",
					"pod-security.kubernetes.io/audit":           "restricted",
					"pod-security.kubernetes.io/audit-version":   "v1.34",
					"pod-security.kubernetes.io/warn":            "restricted",
					"pod-security.kubernetes.io/warn-version":    "v1.34",
				}),
			},
		},
		map[string]any{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "Role",
			"metadata":   namespacedMetadata("gpu-platform-project-observer"),
			"rules": []any{
				map[string]any{
					"apiGroups": []any{""},
					"resources": []any{"events", "persistentvolumeclaims", "pods", "pods/log", "services"},
					"verbs":     []any{"get", "list", "watch"},
				},
				map[string]any{
					"apiGroups": []any{"apps"},
					"resources": []any{"deployments", "replicasets", "statefulsets"},
					"verbs":     []any{"get", "list", "watch"},
				},
				map[string]any{
					"apiGroups": []any{"batch"},
					"resources": []any{"jobs"},
					"verbs":     []any{"get", "list", "watch"},
				},
			},
		},
		map[string]any{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "RoleBinding",
			"metadata":   namespacedMetadata("gpu-platform-project-observer"),
			"roleRef": map[string]any{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     "gpu-platform-project-observer",
			},
			"subjects": []any{
				map[string]any{
					"kind":      "ServiceAccount",
					"name":      input.AddonServiceAccount,
					"namespace": input.AddonInstallNamespace,
				},
			},
		},
		map[string]any{
			"apiVersion": "v1",
			"kind":       "ResourceQuota",
			"metadata":   namespacedMetadata("gpu-project-quota"),
			"spec": map[string]any{
				"hard": map[string]any{
					gpuQuotaResourceName: strconv.FormatInt(input.GPUQuota, 10),
				},
			},
		},
		map[string]any{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "NetworkPolicy",
			"metadata":   namespacedMetadata("default-deny"),
			"spec": map[string]any{
				"podSelector": map[string]any{},
				"policyTypes": []any{"Ingress", "Egress"},
			},
		},
		map[string]any{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "NetworkPolicy",
			"metadata":   namespacedMetadata("allow-project-internal"),
			"spec": map[string]any{
				"podSelector": map[string]any{},
				"policyTypes": []any{"Ingress"},
				"ingress": []any{
					map[string]any{
						"from": []any{map[string]any{"podSelector": map[string]any{}}},
					},
				},
			},
		},
		map[string]any{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "NetworkPolicy",
			"metadata":   namespacedMetadata("allow-dns"),
			"spec": map[string]any{
				"podSelector": map[string]any{},
				"policyTypes": []any{"Egress"},
				"egress": []any{
					map[string]any{
						"to": []any{
							map[string]any{
								"namespaceSelector": map[string]any{
									"matchLabels": map[string]any{"kubernetes.io/metadata.name": "kube-system"},
								},
								"podSelector": map[string]any{
									"matchLabels": map[string]any{"k8s-app": "kube-dns"},
								},
							},
						},
						"ports": []any{
							map[string]any{"port": 53, "protocol": "UDP"},
							map[string]any{"port": 53, "protocol": "TCP"},
						},
					},
				},
			},
		},
	}

	work := map[string]any{
		"apiVersion": "work.open-cluster-management.io/v1",
		"kind":       "ManifestWork",
		"metadata": map[string]any{
			"name":      workName,
			"namespace": input.ClusterID,
			"labels":    labels,
			"annotations": map[string]any{
				"gpu.platform.nyaacat.dev/operation-id":       input.OperationID,
				"gpu.platform.nyaacat.dev/project-generation": strconv.FormatInt(input.Project.Generation, 10),
			},
		},
		"spec": map[string]any{
			"workload": map[string]any{"manifests": manifests},
		},
	}
	encoded, err := json.Marshal(work)
	if err != nil {
		return ports.WorkRequest{}, fmt.Errorf("encode shared-isolation ManifestWork: %w", err)
	}
	return ports.WorkRequest{
		OperationID: input.OperationID,
		ClusterID:   input.ClusterID,
		WorkID:      workName,
		Manifest:    encoded,
	}, nil
}

func WorkName(projectID string) string {
	compact := strings.ReplaceAll(projectID, "-", "")
	if len(compact) > 12 {
		compact = compact[:12]
	}
	return "gpu-project-" + compact
}

func mergeMaps(first, second map[string]any) map[string]any {
	result := make(map[string]any, len(first)+len(second))
	for key, value := range first {
		result[key] = value
	}
	for key, value := range second {
		result[key] = value
	}
	return result
}
