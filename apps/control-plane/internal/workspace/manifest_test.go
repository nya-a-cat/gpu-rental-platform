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
	if manifests := workload["manifests"].([]any); len(manifests) != 1 {
		t.Fatalf("stopped workspace produced %d persistent manifests", len(manifests))
	}
}
