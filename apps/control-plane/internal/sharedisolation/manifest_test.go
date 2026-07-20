package sharedisolation

import (
	"encoding/json"
	"testing"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

func TestBuildWorkRendersSharedIsolationBoundary(t *testing.T) {
	project := tenancy.Project{
		ID:             "11111111-2222-4333-8444-555555555555",
		TenantID:       "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
		IsolationClass: tenancy.IsolationShared,
		NamespaceName:  "gpu-p-111111112222",
		Generation:     3,
	}
	work, err := BuildWork(ManifestInput{
		Project:               project,
		OperationID:           "operation-1",
		ClusterID:             "cluster1",
		GPUQuota:              2,
		AddonInstallNamespace: "open-cluster-management-agent-addon",
		AddonServiceAccount:   "gpu-platform-addon-agent",
	})
	if err != nil {
		t.Fatalf("BuildWork() error = %v", err)
	}
	if work.WorkID != "gpu-project-111111112222" || work.ClusterID != "cluster1" {
		t.Fatalf("work identity = %#v", work)
	}

	var document map[string]any
	if err := json.Unmarshal(work.Manifest, &document); err != nil {
		t.Fatalf("decode work: %v", err)
	}
	metadata := document["metadata"].(map[string]any)
	if metadata["name"] != work.WorkID || metadata["namespace"] != "cluster1" {
		t.Fatalf("ManifestWork metadata = %#v", metadata)
	}
	manifests := document["spec"].(map[string]any)["workload"].(map[string]any)["manifests"].([]any)
	if len(manifests) != 7 {
		t.Fatalf("manifest count = %d, want 7", len(manifests))
	}

	namespace := findManifest(t, manifests, "Namespace", project.NamespaceName)
	namespaceLabels := namespace["metadata"].(map[string]any)["labels"].(map[string]any)
	for _, key := range []string{
		"pod-security.kubernetes.io/enforce",
		"pod-security.kubernetes.io/audit",
		"pod-security.kubernetes.io/warn",
	} {
		if namespaceLabels[key] != "restricted" {
			t.Fatalf("Namespace label %s = %#v", key, namespaceLabels[key])
		}
	}
	if namespaceLabels["pod-security.kubernetes.io/enforce-version"] != "v1.34" {
		t.Fatalf("Namespace enforce version = %#v", namespaceLabels["pod-security.kubernetes.io/enforce-version"])
	}

	roleBinding := findManifest(t, manifests, "RoleBinding", "gpu-platform-project-observer")
	subject := roleBinding["subjects"].([]any)[0].(map[string]any)
	if subject["name"] != "gpu-platform-addon-agent" ||
		subject["namespace"] != "open-cluster-management-agent-addon" {
		t.Fatalf("RoleBinding subject = %#v", subject)
	}

	quota := findManifest(t, manifests, "ResourceQuota", "gpu-project-quota")
	hard := quota["spec"].(map[string]any)["hard"].(map[string]any)
	if hard[gpuQuotaResourceName] != "2" {
		t.Fatalf("GPU quota = %#v", hard[gpuQuotaResourceName])
	}

	defaultDeny := findManifest(t, manifests, "NetworkPolicy", "default-deny")
	policyTypes := defaultDeny["spec"].(map[string]any)["policyTypes"].([]any)
	if len(policyTypes) != 2 {
		t.Fatalf("default deny policy types = %#v", policyTypes)
	}
	findManifest(t, manifests, "NetworkPolicy", "allow-project-internal")
	dns := findManifest(t, manifests, "NetworkPolicy", "allow-dns")
	ports := dns["spec"].(map[string]any)["egress"].([]any)[0].(map[string]any)["ports"].([]any)
	if len(ports) != 2 {
		t.Fatalf("DNS ports = %#v", ports)
	}
}

func TestBuildWorkRejectsUnsafeInputs(t *testing.T) {
	_, err := BuildWork(ManifestInput{
		Project: tenancy.Project{
			ID:             "project",
			TenantID:       "tenant",
			IsolationClass: tenancy.IsolationDedicatedNodePool,
			NamespaceName:  "namespace",
		},
		ClusterID:             "cluster1",
		AddonInstallNamespace: "addon",
		AddonServiceAccount:   "agent",
	})
	if err == nil {
		t.Fatal("BuildWork() error = nil for dedicated project")
	}
}

func findManifest(t *testing.T, manifests []any, kind, name string) map[string]any {
	t.Helper()
	for _, item := range manifests {
		manifest := item.(map[string]any)
		metadata := manifest["metadata"].(map[string]any)
		if manifest["kind"] == kind && metadata["name"] == name {
			return manifest
		}
	}
	t.Fatalf("manifest %s/%s not found", kind, name)
	return nil
}
