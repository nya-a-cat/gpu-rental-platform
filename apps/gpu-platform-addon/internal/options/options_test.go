package options

import (
	"strings"
	"testing"
	"time"
)

func TestParseManagerValidatesRequiredImage(t *testing.T) {
	_, err := ParseManager(nil)
	if err == nil || !strings.Contains(err.Error(), "--agent-image is required") {
		t.Fatalf("expected required agent image error, got %v", err)
	}
}

func TestParseManagerRejectsInvalidNamespaceAndInterval(t *testing.T) {
	_, err := ParseManager([]string{
		"--agent-image=gpu-platform-addon:test",
		"--agent-install-namespace=INVALID_NAMESPACE",
	})
	if err == nil || !strings.Contains(err.Error(), "--agent-install-namespace") {
		t.Fatalf("expected invalid namespace error, got %v", err)
	}

	_, err = ParseManager([]string{
		"--agent-image=gpu-platform-addon:test",
		"--report-interval=0s",
	})
	if err == nil || !strings.Contains(err.Error(), "--report-interval") {
		t.Fatalf("expected invalid interval error, got %v", err)
	}
}

func TestParseAgentAcceptsValidArguments(t *testing.T) {
	t.Setenv("GPU_PLATFORM_ADDON_UID", "addon-uid-123")

	opts, err := ParseAgent([]string{
		"--hub-kubeconfig=/var/run/hub/kubeconfig",
		"--cluster-name=cluster-a",
		"--report-interval=45s",
	})
	if err != nil {
		t.Fatalf("parse agent options: %v", err)
	}
	if opts.ClusterName != "cluster-a" {
		t.Fatalf("unexpected cluster name %q", opts.ClusterName)
	}
	if opts.ReportInterval != 45*time.Second {
		t.Fatalf("unexpected report interval %s", opts.ReportInterval)
	}
	if opts.AddonUID != "addon-uid-123" {
		t.Fatalf("unexpected add-on UID %q", opts.AddonUID)
	}
}

func TestParseAgentValidatesRegistrationArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "hub kubeconfig",
			args: []string{"--cluster-name=cluster-a"},
			want: "--hub-kubeconfig is required",
		},
		{
			name: "cluster name",
			args: []string{"--hub-kubeconfig=/hub/kubeconfig"},
			want: "--cluster-name",
		},
		{
			name: "positional argument",
			args: []string{"--hub-kubeconfig=/hub/kubeconfig", "--cluster-name=cluster-a", "extra"},
			want: "unexpected positional arguments",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseAgent(test.args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}
