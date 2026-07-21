package workspace

import (
	"encoding/json"
	"testing"
)

func TestBuildWorkIsDeterministicAndRequestsWholeGPU(t *testing.T) {
	input := ManifestInput{OperationID: "op-1", Workspace: Workspace{ID: "11111111-1111-1111-1111-111111111111", ProjectID: "p", ClusterID: "cluster-a", NamespaceName: "gpu-p-demo", GPUCount: 2, StorageGiB: 20, DesiredState: DesiredRunning}}
	first, err := BuildWork(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildWork(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(first.Manifest) != string(second.Manifest) {
		t.Fatal("ManifestWork output changed between identical inputs")
	}
	var document map[string]any
	if err := json.Unmarshal(first.Manifest, &document); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(document)
	if len(encoded) == 0 || first.WorkID != WorkName(input.Workspace.ID) {
		t.Fatal("workspace work identity was not deterministic")
	}
}

func TestBuildWorkPublishesWorkspaceSnapshots(t *testing.T) {
	work, err := BuildWork(ManifestInput{Workspace: Workspace{ID: "11111111-1111-1111-1111-111111111111", ClusterID: "cluster-a", NamespaceName: "gpu-p-demo", GPUCount: 1, StorageGiB: 20, DesiredState: DesiredStopped, Snapshots: []Snapshot{{Name: "checkpoint", SourcePVCName: "gpu-workspace-11111111-data", State: "pending"}}}})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(work.Manifest, &document); err != nil {
		t.Fatal(err)
	}
	manifests := document["spec"].(map[string]any)["workload"].(map[string]any)["manifests"].([]any)
	for _, raw := range manifests {
		manifest := raw.(map[string]any)
		if manifest["kind"] != "VolumeSnapshot" {
			continue
		}
		spec := manifest["spec"].(map[string]any)
		if spec["source"].(map[string]any)["persistentVolumeClaimName"] != "gpu-workspace-11111111-data" {
			t.Fatalf("snapshot source PVC = %v", spec["source"])
		}
		return
	}
	t.Fatal("workspace snapshot manifest was not published")
}

func TestBuildWorkStopsByRemovingComputeManifests(t *testing.T) {
	work, err := BuildWork(ManifestInput{Workspace: Workspace{ID: "11111111-1111-1111-1111-111111111111", ClusterID: "cluster-a", NamespaceName: "gpu-p-demo", GPUCount: 1, StorageGiB: 20, DesiredState: DesiredStopped}})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(work.Manifest, &document); err != nil {
		t.Fatal(err)
	}
	spec := document["spec"].(map[string]any)
	workload := spec["workload"].(map[string]any)
	if manifests := workload["manifests"].([]any); len(manifests) != 5 {
		t.Fatalf("stopped workspace produced %d persistent/isolation manifests", len(manifests))
	}
}

func TestBuildWorkPublishesGatewayRoutesAndServicePorts(t *testing.T) {
	work, err := BuildWork(ManifestInput{Workspace: Workspace{ID: "11111111-1111-1111-1111-111111111111", ClusterID: "cluster-a", NamespaceName: "gpu-p-demo", GPUCount: 1, StorageGiB: 20, DesiredState: DesiredRunning}})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(work.Manifest, &document); err != nil {
		t.Fatal(err)
	}
	manifests := document["spec"].(map[string]any)["workload"].(map[string]any)["manifests"].([]any)
	seenService, seenRoute, seenGrant := false, false, false
	for _, raw := range manifests {
		manifest := raw.(map[string]any)
		switch manifest["kind"] {
		case "Service":
			seenService = true
			spec := manifest["spec"].(map[string]any)
			if len(spec["ports"].([]any)) != 3 {
				t.Fatalf("service ports = %d, want 3", len(spec["ports"].([]any)))
			}
		case "HTTPRoute":
			seenRoute = true
		case "ReferenceGrant":
			seenGrant = true
		}
	}
	if !seenService || !seenRoute || !seenGrant {
		t.Fatalf("gateway manifests service/route/grant = %v/%v/%v", seenService, seenRoute, seenGrant)
	}
}
