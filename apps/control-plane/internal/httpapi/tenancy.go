package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/authn"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/ports"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

const maxJSONBodyBytes = 1 << 20

type TenancyStore interface {
	CreateTenant(context.Context, tenancy.CreateTenantParams) (tenancy.Acceptance, error)
	GetTenant(context.Context, string) (tenancy.Tenant, error)
	CreateProject(context.Context, tenancy.CreateProjectParams) (tenancy.Acceptance, error)
	GetProject(context.Context, string) (tenancy.Project, error)
	CreateRoleBinding(context.Context, tenancy.CreateRoleBindingParams) (tenancy.Acceptance, error)
	GetRoleBinding(context.Context, string) (tenancy.RoleBinding, error)
	SetQuota(context.Context, tenancy.SetQuotaParams) (tenancy.Acceptance, error)
	GetQuota(context.Context, string, string) (tenancy.Quota, error)
}

type createTenantRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type createProjectRequest struct {
	TenantID       string                 `json:"tenantId"`
	Name           string                 `json:"name"`
	Slug           string                 `json:"slug"`
	IsolationClass tenancy.IsolationClass `json:"isolationClass"`
}

type createRoleBindingRequest struct {
	ScopeType   tenancy.ScopeType   `json:"scopeType"`
	ScopeID     string              `json:"scopeId"`
	SubjectType tenancy.SubjectType `json:"subjectType"`
	SubjectID   string              `json:"subjectId"`
	Role        tenancy.Role        `json:"role"`
}

type setQuotaRequest struct {
	HardLimit int64 `json:"hardLimit"`
}

func registerTenancyRoutes(mux *http.ServeMux, dependencies Dependencies) {
	registerMethods(mux, "/api/v1/tenants", map[string]http.Handler{
		http.MethodPost: createTenantHandler(dependencies),
	})
	registerMethods(mux, "/api/v1/tenants/{tenantID}", map[string]http.Handler{
		http.MethodGet: getTenantHandler(dependencies),
	})
	registerMethods(mux, "/api/v1/projects", map[string]http.Handler{
		http.MethodPost: createProjectHandler(dependencies),
	})
	registerMethods(mux, "/api/v1/projects/{projectID}", map[string]http.Handler{
		http.MethodGet: getProjectHandler(dependencies),
	})
	registerMethods(mux, "/api/v1/role-bindings", map[string]http.Handler{
		http.MethodPost: createRoleBindingHandler(dependencies),
	})
	registerMethods(mux, "/api/v1/role-bindings/{bindingID}", map[string]http.Handler{
		http.MethodGet: getRoleBindingHandler(dependencies),
	})
	registerMethods(mux, "/api/v1/projects/{projectID}/quotas/{resourceClass}", map[string]http.Handler{
		http.MethodGet: getQuotaHandler(dependencies),
		http.MethodPut: setQuotaHandler(dependencies),
	})
}

func createTenantHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := authenticateRequest(response, request, dependencies.Authenticator)
		if !ok {
			return
		}
		if !authorizeRequest(response, request, dependencies.Authorization, principal, ports.AuthorizationRequest{
			Action:    "tenant.create",
			ScopeType: "system",
			Resource:  "tenant",
		}) {
			return
		}
		var input createTenantRequest
		if !decodeRequestJSON(response, request, &input) {
			return
		}
		mutation, ok := mutationContext(response, request, principal, input)
		if !ok {
			return
		}
		if dependencies.Tenancy == nil {
			writeTenancyUnavailable(response, request)
			return
		}
		accepted, err := dependencies.Tenancy.CreateTenant(request.Context(), tenancy.CreateTenantParams{
			Mutation: mutation,
			Name:     input.Name,
			Slug:     input.Slug,
		})
		if err != nil {
			writeTenancyError(response, request, err)
			return
		}
		writeAcceptance(response, http.StatusAccepted, "/api/v1/tenants/"+accepted.ResourceID, accepted)
	}
}

func getTenantHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := authenticateRequest(response, request, dependencies.Authenticator)
		if !ok {
			return
		}
		tenantID := request.PathValue("tenantID")
		if !authorizeRequest(response, request, dependencies.Authorization, principal, ports.AuthorizationRequest{
			Action:     "tenant.read",
			ScopeType:  string(tenancy.ScopeTenant),
			ScopeID:    tenantID,
			Resource:   "tenant",
			ResourceID: tenantID,
		}) {
			return
		}
		if dependencies.Tenancy == nil {
			writeTenancyUnavailable(response, request)
			return
		}
		result, err := dependencies.Tenancy.GetTenant(request.Context(), tenantID)
		if err != nil {
			writeTenancyError(response, request, err)
			return
		}
		writeJSON(response, http.StatusOK, result)
	}
}

func createProjectHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := authenticateRequest(response, request, dependencies.Authenticator)
		if !ok {
			return
		}
		var input createProjectRequest
		if !decodeRequestJSON(response, request, &input) {
			return
		}
		if !authorizeRequest(response, request, dependencies.Authorization, principal, ports.AuthorizationRequest{
			Action:    "project.create",
			ScopeType: string(tenancy.ScopeTenant),
			ScopeID:   input.TenantID,
			Resource:  "project",
		}) {
			return
		}
		mutation, ok := mutationContext(response, request, principal, input)
		if !ok {
			return
		}
		if dependencies.Tenancy == nil {
			writeTenancyUnavailable(response, request)
			return
		}
		accepted, err := dependencies.Tenancy.CreateProject(request.Context(), tenancy.CreateProjectParams{
			Mutation:       mutation,
			TenantID:       input.TenantID,
			Name:           input.Name,
			Slug:           input.Slug,
			IsolationClass: input.IsolationClass,
		})
		if err != nil {
			writeTenancyError(response, request, err)
			return
		}
		writeAcceptance(response, http.StatusAccepted, "/api/v1/projects/"+accepted.ResourceID, accepted)
	}
}

func getProjectHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := authenticateRequest(response, request, dependencies.Authenticator)
		if !ok {
			return
		}
		projectID := request.PathValue("projectID")
		if !authorizeRequest(response, request, dependencies.Authorization, principal, ports.AuthorizationRequest{
			Action:     "project.read",
			ScopeType:  string(tenancy.ScopeProject),
			ScopeID:    projectID,
			Resource:   "project",
			ResourceID: projectID,
		}) {
			return
		}
		if dependencies.Tenancy == nil {
			writeTenancyUnavailable(response, request)
			return
		}
		result, err := dependencies.Tenancy.GetProject(request.Context(), projectID)
		if err != nil {
			writeTenancyError(response, request, err)
			return
		}
		writeJSON(response, http.StatusOK, result)
	}
}

func createRoleBindingHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := authenticateRequest(response, request, dependencies.Authenticator)
		if !ok {
			return
		}
		var input createRoleBindingRequest
		if !decodeRequestJSON(response, request, &input) {
			return
		}
		if !authorizeRequest(response, request, dependencies.Authorization, principal, ports.AuthorizationRequest{
			Action:    "role_binding.create",
			ScopeType: string(input.ScopeType),
			ScopeID:   input.ScopeID,
			Resource:  "role_binding",
		}) {
			return
		}
		mutation, ok := mutationContext(response, request, principal, input)
		if !ok {
			return
		}
		if dependencies.Tenancy == nil {
			writeTenancyUnavailable(response, request)
			return
		}
		accepted, err := dependencies.Tenancy.CreateRoleBinding(request.Context(), tenancy.CreateRoleBindingParams{
			Mutation:    mutation,
			ScopeType:   input.ScopeType,
			ScopeID:     input.ScopeID,
			SubjectType: input.SubjectType,
			SubjectID:   input.SubjectID,
			Role:        input.Role,
		})
		if err != nil {
			writeTenancyError(response, request, err)
			return
		}
		writeAcceptance(response, http.StatusAccepted, "/api/v1/role-bindings/"+accepted.ResourceID, accepted)
	}
}

func getRoleBindingHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := authenticateRequest(response, request, dependencies.Authenticator)
		if !ok {
			return
		}
		if dependencies.Tenancy == nil {
			writeTenancyUnavailable(response, request)
			return
		}
		result, err := dependencies.Tenancy.GetRoleBinding(request.Context(), request.PathValue("bindingID"))
		if err != nil {
			writeTenancyError(response, request, err)
			return
		}
		if !authorizeRequest(response, request, dependencies.Authorization, principal, ports.AuthorizationRequest{
			Action:     "role_binding.read",
			ScopeType:  string(result.ScopeType),
			ScopeID:    result.ScopeID,
			Resource:   "role_binding",
			ResourceID: result.ID,
		}) {
			return
		}
		writeJSON(response, http.StatusOK, result)
	}
}

func setQuotaHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := authenticateRequest(response, request, dependencies.Authenticator)
		if !ok {
			return
		}
		projectID := request.PathValue("projectID")
		resourceClass := request.PathValue("resourceClass")
		if !authorizeRequest(response, request, dependencies.Authorization, principal, ports.AuthorizationRequest{
			Action:     "quota.set",
			ScopeType:  string(tenancy.ScopeProject),
			ScopeID:    projectID,
			Resource:   "quota",
			ResourceID: resourceClass,
		}) {
			return
		}
		var input setQuotaRequest
		if !decodeRequestJSON(response, request, &input) {
			return
		}
		mutation, ok := mutationContext(response, request, principal, input)
		if !ok {
			return
		}
		if dependencies.Tenancy == nil {
			writeTenancyUnavailable(response, request)
			return
		}
		accepted, err := dependencies.Tenancy.SetQuota(request.Context(), tenancy.SetQuotaParams{
			Mutation:      mutation,
			ProjectID:     projectID,
			ResourceClass: resourceClass,
			HardLimit:     input.HardLimit,
		})
		if err != nil {
			writeTenancyError(response, request, err)
			return
		}
		location := "/api/v1/projects/" + projectID + "/quotas/" + url.PathEscape(resourceClass)
		writeAcceptance(response, http.StatusAccepted, location, accepted)
	}
}

func getQuotaHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := authenticateRequest(response, request, dependencies.Authenticator)
		if !ok {
			return
		}
		projectID := request.PathValue("projectID")
		resourceClass := request.PathValue("resourceClass")
		if !authorizeRequest(response, request, dependencies.Authorization, principal, ports.AuthorizationRequest{
			Action:     "quota.read",
			ScopeType:  string(tenancy.ScopeProject),
			ScopeID:    projectID,
			Resource:   "quota",
			ResourceID: resourceClass,
		}) {
			return
		}
		if dependencies.Tenancy == nil {
			writeTenancyUnavailable(response, request)
			return
		}
		result, err := dependencies.Tenancy.GetQuota(request.Context(), projectID, resourceClass)
		if err != nil {
			writeTenancyError(response, request, err)
			return
		}
		writeJSON(response, http.StatusOK, result)
	}
}

func authenticateRequest(
	response http.ResponseWriter,
	request *http.Request,
	authenticator authn.Authenticator,
) (authn.Principal, bool) {
	if authenticator == nil {
		writeProblem(response, request, Problem{
			Title:  "Service Unavailable",
			Status: http.StatusServiceUnavailable,
			Detail: "The authenticated tenancy API is not configured.",
			Code:   "authentication_unavailable",
		})
		return authn.Principal{}, false
	}
	principal, err := authenticator.Authenticate(request)
	if errors.Is(err, authn.ErrUnauthenticated) {
		response.Header().Set("WWW-Authenticate", "Bearer")
		writeProblem(response, request, Problem{
			Title:  "Unauthorized",
			Status: http.StatusUnauthorized,
			Detail: "A valid bearer token is required.",
			Code:   "unauthenticated",
		})
		return authn.Principal{}, false
	}
	if err != nil {
		writeProblem(response, request, Problem{
			Title:  "Service Unavailable",
			Status: http.StatusServiceUnavailable,
			Detail: "Authentication could not be completed.",
			Code:   "authentication_unavailable",
		})
		return authn.Principal{}, false
	}
	return principal, true
}

func authorizeRequest(
	response http.ResponseWriter,
	request *http.Request,
	engine ports.AuthorizationEngine,
	principal authn.Principal,
	authorizationRequest ports.AuthorizationRequest,
) bool {
	if principal.SystemAdmin {
		return true
	}
	if engine == nil {
		writeProblem(response, request, Problem{
			Title:  "Service Unavailable",
			Status: http.StatusServiceUnavailable,
			Detail: "Authorization storage is unavailable.",
			Code:   "authorization_unavailable",
		})
		return false
	}
	authorizationRequest.SubjectID = principal.ID
	decision, err := engine.Authorize(request.Context(), authorizationRequest)
	if err != nil {
		writeProblem(response, request, Problem{
			Title:  "Service Unavailable",
			Status: http.StatusServiceUnavailable,
			Detail: "Authorization could not be completed.",
			Code:   "authorization_unavailable",
		})
		return false
	}
	if !decision.Allowed {
		writeProblem(response, request, Problem{
			Title:  "Forbidden",
			Status: http.StatusForbidden,
			Detail: "The authenticated principal is not allowed to perform this action.",
			Code:   "forbidden",
		})
		return false
	}
	return true
}

func mutationContext(
	response http.ResponseWriter,
	request *http.Request,
	principal authn.Principal,
	input any,
) (tenancy.MutationContext, bool) {
	idempotencyKey := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 255 || strings.ContainsAny(idempotencyKey, "\r\n") {
		writeProblem(response, request, Problem{
			Title:  "Invalid Idempotency-Key",
			Status: http.StatusBadRequest,
			Detail: "Idempotency-Key must contain between 8 and 255 characters.",
			Code:   "invalid_idempotency_key",
		})
		return tenancy.MutationContext{}, false
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		writeProblem(response, request, Problem{
			Title:  "Internal Server Error",
			Status: http.StatusInternalServerError,
			Detail: "The request fingerprint could not be created.",
			Code:   "request_hash_failed",
		})
		return tenancy.MutationContext{}, false
	}
	digest := sha256.Sum256([]byte(request.Method + "\n" + request.URL.Path + "\n" + principal.ID + "\n" + string(encoded)))
	return tenancy.MutationContext{
		PrincipalID:    principal.ID,
		RequestID:      RequestIDFromContext(request.Context()),
		IdempotencyKey: idempotencyKey,
		RequestHash:    hex.EncodeToString(digest[:]),
	}, true
}

func decodeRequestJSON(response http.ResponseWriter, request *http.Request, target any) bool {
	request.Body = http.MaxBytesReader(response, request.Body, maxJSONBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeProblem(response, request, Problem{
			Title:  "Invalid request body",
			Status: http.StatusBadRequest,
			Detail: "The request body must be a valid JSON object with supported fields.",
			Code:   "invalid_request_body",
		})
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeProblem(response, request, Problem{
			Title:  "Invalid request body",
			Status: http.StatusBadRequest,
			Detail: "The request body must contain a single JSON object.",
			Code:   "invalid_request_body",
		})
		return false
	}
	return true
}

func writeAcceptance(response http.ResponseWriter, status int, location string, acceptance tenancy.Acceptance) {
	response.Header().Set("Location", location)
	response.Header().Set("Operation-Location", "/api/v1/operations/"+acceptance.OperationID)
	writeJSON(response, status, acceptance)
}

func writeTenancyUnavailable(response http.ResponseWriter, request *http.Request) {
	writeProblem(response, request, Problem{
		Title:  "Service Unavailable",
		Status: http.StatusServiceUnavailable,
		Detail: "Tenancy storage is unavailable.",
		Code:   "tenancy_storage_unavailable",
	})
}

func writeTenancyError(response http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, tenancy.ErrInvalid):
		detail := strings.TrimSuffix(err.Error(), ": "+tenancy.ErrInvalid.Error())
		writeProblem(response, request, Problem{Title: "Invalid request", Status: http.StatusBadRequest, Detail: detail, Code: "invalid_request"})
	case errors.Is(err, tenancy.ErrNotFound):
		writeProblem(response, request, Problem{Title: "Resource not found", Status: http.StatusNotFound, Detail: "The requested tenancy resource does not exist.", Code: "tenancy_resource_not_found"})
	case errors.Is(err, tenancy.ErrIdempotencyConflict):
		writeProblem(response, request, Problem{Title: "Idempotency conflict", Status: http.StatusConflict, Detail: "The Idempotency-Key was already used with a different request.", Code: "idempotency_conflict"})
	case errors.Is(err, tenancy.ErrConflict):
		writeProblem(response, request, Problem{Title: "Resource conflict", Status: http.StatusConflict, Detail: "A resource with the supplied identity already exists.", Code: "tenancy_resource_conflict"})
	case errors.Is(err, tenancy.ErrQuotaExceeded):
		writeProblem(response, request, Problem{Title: "Quota exceeded", Status: http.StatusConflict, Detail: "The requested amount exceeds the available project quota.", Code: "quota_exceeded"})
	case errors.Is(err, tenancy.ErrQuotaBelowUsage):
		writeProblem(response, request, Problem{Title: "Quota conflict", Status: http.StatusConflict, Detail: "The quota limit cannot be lower than current reserved and allocated usage.", Code: "quota_below_usage"})
	default:
		writeProblem(response, request, Problem{Title: "Internal Server Error", Status: http.StatusInternalServerError, Detail: "The tenancy request could not be completed.", Code: "tenancy_request_failed"})
	}
}

func registerMethods(mux *http.ServeMux, pattern string, handlers map[string]http.Handler) {
	allowed := make([]string, 0, len(handlers))
	for method, handler := range handlers {
		allowed = append(allowed, method)
		mux.Handle(method+" "+pattern, handler)
	}
	sort.Strings(allowed)
	mux.HandleFunc(pattern, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Allow", strings.Join(allowed, ", "))
		writeProblem(response, request, Problem{
			Title:  "Method Not Allowed",
			Status: http.StatusMethodNotAllowed,
			Detail: fmt.Sprintf("This resource supports %s requests.", strings.Join(allowed, " and ")),
			Code:   "method_not_allowed",
		})
	})
}
