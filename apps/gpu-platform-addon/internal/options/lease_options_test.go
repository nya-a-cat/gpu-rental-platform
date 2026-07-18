package options

import (
	"strings"
	"testing"
)

func TestParseAgentCarriesLeaseNamespace(t *testing.T) {
	opts, err := ParseAgent([]string{
		"--hub-kubeconfig=/var/run/hub/kubeconfig",
		"--cluster-name=cluster-a",
		"--addon-namespace=gpu-addon-system",
	})
	if err != nil {
		t.Fatalf("parse agent options: %v", err)
	}
	if opts.AddonInstallNamespace != "gpu-addon-system" {
		t.Fatalf("unexpected add-on namespace %q", opts.AddonInstallNamespace)
	}
}

func TestParseAgentRejectsInvalidLeaseNamespace(t *testing.T) {
	_, err := ParseAgent([]string{
		"--hub-kubeconfig=/var/run/hub/kubeconfig",
		"--cluster-name=cluster-a",
		"--addon-namespace=INVALID_NAMESPACE",
	})
	if err == nil || !strings.Contains(err.Error(), "--addon-namespace") {
		t.Fatalf("expected invalid add-on namespace error, got %v", err)
	}
}
