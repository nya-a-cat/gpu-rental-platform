package addonmanager

import (
	"bytes"
	"strings"
	"testing"
	"text/template"
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
