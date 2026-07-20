package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/authn"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/ports"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

const (
	testTenantID  = "20b29b1f-2952-4f64-bdfa-13c05528f9a7"
	testProjectID = "6dcb930b-590c-4ce1-8abf-c062b5a5402d"
	testBindingID = "79238920-d0f6-47b6-b634-7b29342a86b1"
)

type authenticatorStub struct {
	principal authn.Principal
	err       error
}

func (stub authenticatorStub) Authenticate(*http.Request) (authn.Principal, error) {
	return stub.principal, stub.err
}

type authorizationStub struct {
	decision ports.AuthorizationDecision
	err      error
	request  ports.AuthorizationRequest
}

func (stub *authorizationStub) Authorize(
	_ context.Context,
	request ports.AuthorizationRequest,
) (ports.AuthorizationDecision, error) {
	stub.request = request
	return stub.decision, stub.err
}

type tenancyStoreStub struct {
	acceptance       tenancy.Acceptance
	err              error
	createTenant     tenancy.CreateTenantParams
	createTenantCall int
	quota            tenancy.Quota
}

func (stub *tenancyStoreStub) CreateTenant(
	_ context.Context,
	params tenancy.CreateTenantParams,
) (tenancy.Acceptance, error) {
	stub.createTenant = params
	stub.createTenantCall++
	return stub.acceptance, stub.err
}

func (stub *tenancyStoreStub) GetTenant(context.Context, string) (tenancy.Tenant, error) {
	return tenancy.Tenant{ID: testTenantID, Name: "Tenant", Slug: "tenant-one", Status: "active"}, stub.err
}

func (stub *tenancyStoreStub) CreateProject(context.Context, tenancy.CreateProjectParams) (tenancy.Acceptance, error) {
	return stub.acceptance, stub.err
}

func (stub *tenancyStoreStub) GetProject(context.Context, string) (tenancy.Project, error) {
	return tenancy.Project{ID: testProjectID, TenantID: testTenantID, IsolationClass: tenancy.IsolationShared}, stub.err
}

func (stub *tenancyStoreStub) CreateRoleBinding(context.Context, tenancy.CreateRoleBindingParams) (tenancy.Acceptance, error) {
	return stub.acceptance, stub.err
}

func (stub *tenancyStoreStub) GetRoleBinding(context.Context, string) (tenancy.RoleBinding, error) {
	return tenancy.RoleBinding{ID: testBindingID, ScopeType: tenancy.ScopeProject, ScopeID: testProjectID}, stub.err
}

func (stub *tenancyStoreStub) SetQuota(context.Context, tenancy.SetQuotaParams) (tenancy.Acceptance, error) {
	return stub.acceptance, stub.err
}

func (stub *tenancyStoreStub) GetQuota(context.Context, string, string) (tenancy.Quota, error) {
	return stub.quota, stub.err
}

func tenancyHandler(
	store TenancyStore,
	authenticator authn.Authenticator,
	authorization ports.AuthorizationEngine,
) http.Handler {
	return NewHandler(Dependencies{
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Readiness:     readinessStub{},
		Operations:    operationReaderStub{},
		Tenancy:       store,
		Authenticator: authenticator,
		Authorization: authorization,
		Info:          SystemInfo{Product: "gpu-container-cloud", APIVersion: "v1"},
	})
}

func TestCreateTenantRequiresAuthentication(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", strings.NewReader(`{"name":"Tenant","slug":"tenant-one"}`))
	request.Header.Set("Idempotency-Key", "tenant-key")
	response := httptest.NewRecorder()
	tenancyHandler(&tenancyStoreStub{}, authenticatorStub{err: authn.ErrUnauthenticated}, nil).ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("WWW-Authenticate = %q", response.Header().Get("WWW-Authenticate"))
	}
}

func TestCreateTenantRequiresIdempotencyKey(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", strings.NewReader(`{"name":"Tenant","slug":"tenant-one"}`))
	response := httptest.NewRecorder()
	tenancyHandler(&tenancyStoreStub{}, authenticatorStub{principal: authn.Principal{ID: "admin", SystemAdmin: true}}, nil).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body.String())
	}
	var problem Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Code != "invalid_idempotency_key" {
		t.Fatalf("problem = %#v", problem)
	}
}

func TestCreateTenantReturnsOperationAcceptance(t *testing.T) {
	store := &tenancyStoreStub{acceptance: tenancy.Acceptance{
		ResourceID:  testTenantID,
		OperationID: testOperationID,
	}}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", strings.NewReader(`{"name":"Tenant","slug":"tenant-one"}`))
	request.Header.Set("Idempotency-Key", "tenant-key")
	request.Header.Set("X-Request-ID", "tenant-request")
	response := httptest.NewRecorder()
	tenancyHandler(store, authenticatorStub{principal: authn.Principal{ID: "admin", SystemAdmin: true}}, nil).ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Location") != "/api/v1/tenants/"+testTenantID || response.Header().Get("Operation-Location") != "/api/v1/operations/"+testOperationID {
		t.Fatalf("acceptance headers = %#v", response.Header())
	}
	if store.createTenantCall != 1 || store.createTenant.Mutation.PrincipalID != "admin" || store.createTenant.Mutation.RequestID != "tenant-request" || store.createTenant.Mutation.IdempotencyKey != "tenant-key" || len(store.createTenant.Mutation.RequestHash) != 64 {
		t.Fatalf("create tenant params = %#v", store.createTenant)
	}
	var accepted map[string]any
	if err := json.NewDecoder(response.Body).Decode(&accepted); err != nil {
		t.Fatalf("decode acceptance: %v", err)
	}
	if len(accepted) != 2 || accepted["resourceId"] != testTenantID || accepted["operationId"] != testOperationID {
		t.Fatalf("acceptance body = %#v", accepted)
	}
}

func TestCreateTenantRejectsUnauthorizedPrincipal(t *testing.T) {
	store := &tenancyStoreStub{}
	authorization := &authorizationStub{decision: ports.AuthorizationDecision{Allowed: false}}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", strings.NewReader(`{"name":"Tenant","slug":"tenant-one"}`))
	request.Header.Set("Idempotency-Key", "tenant-key")
	response := httptest.NewRecorder()
	tenancyHandler(store, authenticatorStub{principal: authn.Principal{ID: "user-1"}}, authorization).ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", response.Code, response.Body.String())
	}
	if store.createTenantCall != 0 || authorization.request.Action != "tenant.create" || authorization.request.SubjectID != "user-1" {
		t.Fatalf("authorization request = %#v, calls = %d", authorization.request, store.createTenantCall)
	}
}

func TestCreateTenantMapsIdempotencyConflict(t *testing.T) {
	store := &tenancyStoreStub{err: tenancy.ErrIdempotencyConflict}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", strings.NewReader(`{"name":"Tenant","slug":"tenant-one"}`))
	request.Header.Set("Idempotency-Key", "tenant-key")
	response := httptest.NewRecorder()
	tenancyHandler(store, authenticatorStub{principal: authn.Principal{ID: "admin", SystemAdmin: true}}, nil).ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", response.Code, response.Body.String())
	}
	var problem Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Code != "idempotency_conflict" {
		t.Fatalf("problem = %#v", problem)
	}
}

func TestQuotaRouteAdvertisesSupportedMethods(t *testing.T) {
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+testProjectID+"/quotas/gpu.nvidia.full", nil)
	response := httptest.NewRecorder()
	tenancyHandler(&tenancyStoreStub{}, authenticatorStub{}, nil).ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", response.Code)
	}
	if response.Header().Get("Allow") != "GET, PUT" {
		t.Fatalf("Allow = %q, want GET, PUT", response.Header().Get("Allow"))
	}
}

func TestTenancyInvalidBodyUsesProblemJSON(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", strings.NewReader(`{"name":"Tenant","slug":"tenant-one","unknown":true}`))
	request.Header.Set("Idempotency-Key", "tenant-key")
	response := httptest.NewRecorder()
	tenancyHandler(&tenancyStoreStub{}, authenticatorStub{principal: authn.Principal{ID: "admin", SystemAdmin: true}}, nil).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || response.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("status/content-type = %d/%q", response.Code, response.Header().Get("Content-Type"))
	}
}

func TestTenancyAuthenticationUnavailable(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/"+testTenantID, nil)
	response := httptest.NewRecorder()
	tenancyHandler(&tenancyStoreStub{}, nil, nil).ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
}

func TestTenancyStoreFailureIsInternalError(t *testing.T) {
	store := &tenancyStoreStub{err: errors.New("database failed")}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/"+testTenantID, nil)
	response := httptest.NewRecorder()
	tenancyHandler(store, authenticatorStub{principal: authn.Principal{ID: "admin", SystemAdmin: true}}, nil).ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
}
