package addonmanager

import (
	"bytes"
	"strings"
	"testing"
	"text/template"

	workv1 "open-cluster-management.io/api/work/v1"
)

func TestAgentDeploymentTemplateIncludesManagedClusterAddOnUID(t *testing.T) {
	source, err := manifestFS.ReadFile("manifests/templates/30-deployment.yaml")
	if err != nil {
		t.Fatalf("read agent Deployment template: %v", err)
	}
	tmpl, err := template.New("agent-deployment").Parse(string(source))
	if err != nil {
		t.Fatalf("parse agent Deployment template: %v", err)
	}

	values := struct {
		AgentName             string
		AddonInstallNamespace string
		Image                 string
		AddonUID              string
		ClusterName           string
		AddonName             string
		ReportInterval        string
		HubKubeConfigSecret   string
	}{
		AgentName:             "gpu-platform-addon-agent",
		AddonInstallNamespace: "open-cluster-management-agent-addon",
		Image:                 "gpu-platform-addon:current",
		AddonUID:              "managed-cluster-addon-uid",
		ClusterName:           "cluster-a",
		AddonName:             "gpu-platform-addon",
		ReportInterval:        "15s",
		HubKubeConfigSecret:   "gpu-platform-addon-hub-kubeconfig",
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, values); err != nil {
		t.Fatalf("render agent Deployment template: %v", err)
	}
	if !strings.Contains(rendered.String(), "value: \"managed-cluster-addon-uid\"") {
		t.Fatalf("rendered Deployment is missing the add-on UID environment value:\n%s", rendered.String())
	}
}

func TestAgentLeaseTemplateIsManagedWithTheWorkload(t *testing.T) {
	source, err := manifestFS.ReadFile("manifests/templates/24-lease.yaml")
	if err != nil {
		t.Fatalf("read agent Lease template: %v", err)
	}
	tmpl, err := template.New("agent-lease").Parse(string(source))
	if err != nil {
		t.Fatalf("parse agent Lease template: %v", err)
	}

	values := struct {
		AddonName             string
		AddonInstallNamespace string
	}{
		AddonName:             "gpu-platform-addon",
		AddonInstallNamespace: "open-cluster-management-agent-addon",
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, values); err != nil {
		t.Fatalf("render agent Lease template: %v", err)
	}
	for _, expected := range []string{
		"kind: Lease",
		"name: gpu-platform-addon",
		"namespace: open-cluster-management-agent-addon",
	} {
		if !strings.Contains(rendered.String(), expected) {
			t.Fatalf("rendered Lease is missing %q:\n%s", expected, rendered.String())
		}
	}
}

func TestAgentLeaseUsesServerSideApply(t *testing.T) {
	config := leaseManifestConfig(
		"gpu-platform-addon",
		"open-cluster-management-agent-addon",
	)

	if config.ResourceIdentifier.Group != "coordination.k8s.io" ||
		config.ResourceIdentifier.Resource != "leases" ||
		config.ResourceIdentifier.Name != "gpu-platform-addon" ||
		config.ResourceIdentifier.Namespace != "open-cluster-management-agent-addon" {
		t.Fatalf("unexpected Lease resource identifier: %#v", config.ResourceIdentifier)
	}
	if config.UpdateStrategy == nil {
		t.Fatal("Lease update strategy is missing")
	}
	if config.UpdateStrategy.Type != workv1.UpdateStrategyTypeServerSideApply {
		t.Fatalf(
			"unexpected Lease update strategy %q",
			config.UpdateStrategy.Type,
		)
	}
}
