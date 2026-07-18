package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/operation"
)

const testOperationID = "cae14860-f87c-44b2-9a4b-53512502e959"

type readinessStub struct {
	err error
}

func (stub readinessStub) PingContext(context.Context) error {
	return stub.err
}

type operationReaderStub struct {
	operation operation.Operation
	err       error
}

func (stub operationReaderStub) GetByID(context.Context, string) (operation.Operation, error) {
	return stub.operation, stub.err
}

func testHandler(readiness ReadinessChecker, operations operation.Reader) http.Handler {
	return NewHandler(Dependencies{
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		Readiness:        readiness,
		Operations:       operations,
		ReadinessTimeout: time.Second,
		Info: SystemInfo{
			Product:      "gpu-container-cloud",
			APIVersion:   "v1",
			Version:      "test",
			Commit:       "test-commit",
			Stage:        "phase-0-foundation",
			Architecture: "modular-monolith",
			Persistence:  "postgresql",
			Capabilities: []string{"operations", "outbox"},
		},
	})
}

func TestLivenessAndRequestID(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	response := httptest.NewRecorder()
	testHandler(readinessStub{}, operationReaderStub{}).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("X-Request-ID is empty")
	}
}

func TestReadinessFailureUsesProblemJSON(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	request.Header.Set("X-Request-ID", "test-request")
	response := httptest.NewRecorder()
	testHandler(readinessStub{err: errors.New("database down")}, operationReaderStub{}).ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", contentType)
	}
	var problem Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Type != "urn:gpu-container-cloud:problem:database_unavailable" || problem.Code != "database_unavailable" || problem.RequestID != "test-request" {
		t.Fatalf("problem = %#v", problem)
	}
}

func TestReadinessSuccessIncludesPostgreSQLCheck(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	response := httptest.NewRecorder()
	testHandler(readinessStub{}, operationReaderStub{}).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var status struct {
		Status string `json:"status"`
		Checks struct {
			PostgreSQL struct {
				Status    string `json:"status"`
				LatencyMS int64  `json:"latencyMs"`
			} `json:"postgresql"`
		} `json:"checks"`
	}
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		t.Fatalf("decode readiness status: %v", err)
	}
	if status.Status != "ready" || status.Checks.PostgreSQL.Status != "ready" || status.Checks.PostgreSQL.LatencyMS < 0 {
		t.Fatalf("readiness status = %#v", status)
	}
}

func TestGetOperation(t *testing.T) {
	expected := operation.Operation{
		ID:        testOperationID,
		Kind:      "instance.create",
		Status:    operation.StatusQueued,
		Target:    operation.ResourceRef{Type: "instance", ID: "instance-1"},
		Steps:     []operation.Step{},
		RequestID: "request-1",
		CreatedAt: time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC),
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations/"+testOperationID, nil)
	response := httptest.NewRecorder()
	testHandler(readinessStub{}, operationReaderStub{operation: expected}).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	var actual operation.Operation
	if err := json.NewDecoder(response.Body).Decode(&actual); err != nil {
		t.Fatalf("decode operation: %v", err)
	}
	if actual.ID != expected.ID || actual.Kind != expected.Kind {
		t.Fatalf("operation = %#v", actual)
	}
}

func TestGetOperationRejectsMalformedID(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations/not-a-uuid", nil)
	response := httptest.NewRecorder()
	testHandler(readinessStub{}, operationReaderStub{}).ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func TestUnknownRouteUsesProblemJSON(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
	response := httptest.NewRecorder()
	testHandler(readinessStub{}, operationReaderStub{}).ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", contentType)
	}
	var problem Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Type != "urn:gpu-container-cloud:problem:route_not_found" || problem.Code != "route_not_found" {
		t.Fatalf("problem = %#v", problem)
	}
}

func TestKnownRouteRejectsUnsupportedMethodWithProblemJSON(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/health/live", nil)
	response := httptest.NewRecorder()
	testHandler(readinessStub{}, operationReaderStub{}).ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", response.Code)
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", allow)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", contentType)
	}
	var problem Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Type != "urn:gpu-container-cloud:problem:method_not_allowed" || problem.Code != "method_not_allowed" {
		t.Fatalf("problem = %#v", problem)
	}
}

func TestMetrics(t *testing.T) {
	handler := testHandler(readinessStub{}, operationReaderStub{})
	firstRequest := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	handler.ServeHTTP(httptest.NewRecorder(), firstRequest)

	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if body := response.Body.String(); !contains(body, "gpu_control_plane_build_info") || !contains(body, "gpu_control_plane_http_requests_total 2") {
		t.Fatalf("metrics body = %s", body)
	}
}

func contains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
