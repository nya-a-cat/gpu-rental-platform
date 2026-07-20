package ocm

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/ports"
)

func TestClientAppliesAndWaitsForManifestWork(t *testing.T) {
	var patchCount atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(response, "unauthorized", http.StatusUnauthorized)
			return
		}
		if request.URL.Path != "/apis/work.open-cluster-management.io/v1/namespaces/cluster1/manifestworks/gpu-project-123456789abc" {
			http.NotFound(response, request)
			return
		}
		switch request.Method {
		case http.MethodPatch:
			patchCount.Add(1)
			if request.Header.Get("Content-Type") != "application/apply-patch+yaml" ||
				request.URL.Query().Get("fieldManager") != "gpu-cloud-control-plane" ||
				request.URL.Query().Get("force") != "true" {
				http.Error(response, "invalid apply request", http.StatusBadRequest)
				return
			}
			body, _ := io.ReadAll(request.Body)
			if !strings.Contains(string(body), "gpu-project-123456789abc") {
				http.Error(response, "missing work identity", http.StatusBadRequest)
				return
			}
			response.Header().Set("Content-Type", "application/json")
			response.WriteHeader(http.StatusCreated)
			_, _ = response.Write([]byte(`{"kind":"ManifestWork"}`))
		case http.MethodGet:
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"status":{"conditions":[{"type":"Applied","status":"True"},{"type":"Available","status":"True"}]}}`))
		default:
			http.Error(response, "unsupported method", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	manifest := []byte(`{
	  "apiVersion":"work.open-cluster-management.io/v1",
	  "kind":"ManifestWork",
	  "metadata":{"name":"gpu-project-123456789abc","namespace":"cluster1"}
	}`)
	request := ports.WorkRequest{
		OperationID: "operation-1",
		ClusterID:   "cluster1",
		WorkID:      "gpu-project-123456789abc",
		Manifest:    manifest,
	}
	for attempt := 0; attempt < 2; attempt++ {
		result, err := client.ApplyWork(context.Background(), request)
		if err != nil {
			t.Fatalf("ApplyWork() attempt %d error = %v", attempt, err)
		}
		if result.WorkID != request.WorkID || !result.Applied || !result.Available {
			t.Fatalf("ApplyWork() result = %#v", result)
		}
	}
	if patchCount.Load() != 2 {
		t.Fatalf("PATCH count = %d, want 2", patchCount.Load())
	}
}

func TestClientReturnsDegradedManifestWork(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPatch {
			_, _ = response.Write([]byte(`{"kind":"ManifestWork"}`))
			return
		}
		_, _ = response.Write([]byte(`{"status":{"conditions":[{"type":"Degraded","status":"True","reason":"ApplyFailed","message":"resource rejected"}]}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server)
	_, err := client.ApplyWork(context.Background(), ports.WorkRequest{
		ClusterID: "cluster1",
		WorkID:    "gpu-project-123456789abc",
		Manifest:  []byte(`{"apiVersion":"work.open-cluster-management.io/v1","kind":"ManifestWork","metadata":{"name":"gpu-project-123456789abc","namespace":"cluster1"}}`),
	})
	if err == nil || !strings.Contains(err.Error(), "ApplyFailed") {
		t.Fatalf("ApplyWork() error = %v, want degraded detail", err)
	}
}

func TestClientRejectsConfusedDeputyManifest(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	client := newTestClient(t, server)
	_, err := client.ApplyWork(context.Background(), ports.WorkRequest{
		ClusterID: "cluster1",
		WorkID:    "expected-work",
		Manifest:  []byte(`{"apiVersion":"work.open-cluster-management.io/v1","kind":"ManifestWork","metadata":{"name":"other-work","namespace":"cluster2"}}`),
	})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("ApplyWork() error = %v, want identity mismatch", err)
	}
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	tempDir := t.TempDir()
	certificate := server.Certificate()
	if certificate == nil {
		t.Fatal("test TLS certificate is nil")
	}
	caPath := filepath.Join(tempDir, "ca.crt")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}), 0o600); err != nil {
		t.Fatalf("write CA file: %v", err)
	}
	if _, err := x509.ParseCertificate(certificate.Raw); err != nil {
		t.Fatalf("parse test certificate: %v", err)
	}
	tokenPath := filepath.Join(tempDir, "token")
	if err := os.WriteFile(tokenPath, []byte("test-token"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	client, err := NewClient(Config{
		HubURL:       server.URL,
		TokenFile:    tokenPath,
		CAFile:       caPath,
		PollInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}
