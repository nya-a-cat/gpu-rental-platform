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
	if input.Workspace.DesiredState == DesiredRunning {
		manifests = append(manifests, map[string]any{
			"apiVersion": "apps/v1", "kind": "StatefulSet",
			"metadata": map[string]any{"name": name, "namespace": input.Workspace.NamespaceName, "labels": labels},
			"spec": map[string]any{
				"serviceName": name, "replicas": 1,
				"selector": map[string]any{"matchLabels": labels},
				"template": map[string]any{
					"metadata": map[string]any{"labels": labels},
					"spec": map[string]any{"containers": []any{map[string]any{
						"name": "workspace", "image": "nvidia/cuda:12.6.3-base-ubuntu24.04", "command": []any{"sleep", "infinity"}, "resources": resources,
					}}},
				},
			},
		})
		manifests = append(manifests, map[string]any{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]any{"name": name, "namespace": input.Workspace.NamespaceName, "labels": labels},
			"spec":     map[string]any{"clusterIP": "None", "selector": labels},
		})
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
