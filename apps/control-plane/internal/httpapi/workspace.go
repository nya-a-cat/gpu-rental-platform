package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/authn"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/ports"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/workspace"
)

type createWorkspaceRequest struct {
	ProjectID            string `json:"projectId"`
	ClusterID            string `json:"clusterId"`
	AcceleratorProfileID string `json:"acceleratorProfileId"`
	Name                 string `json:"name"`
	StorageGiB           int    `json:"storageGiB,omitempty"`
}

type setWorkspaceDesiredStateRequest struct {
	DesiredState workspace.DesiredState `json:"desiredState"`
}

type createAccessTokenRequest struct {
	AccessType string `json:"accessType"`
}

type inspectAccessTokenRequest struct {
	Token string `json:"token"`
}

func registerWorkspaceRoutes(mux *http.ServeMux, dependencies Dependencies) {
	registerMethods(mux, "/api/v1/instances", map[string]http.Handler{http.MethodPost: createWorkspaceHandler(dependencies)})
	registerMethods(mux, "/api/v1/instances/{instanceID}", map[string]http.Handler{http.MethodGet: getWorkspaceHandler(dependencies), http.MethodPatch: setWorkspaceDesiredStateHandler(dependencies)})
	registerMethods(mux, "/api/v1/instances/{instanceID}/access-tokens", map[string]http.Handler{http.MethodPost: createAccessTokenHandler(dependencies)})
	registerMethods(mux, "/api/v1/instances/{instanceID}/access-tokens/{tokenID}", map[string]http.Handler{http.MethodDelete: revokeAccessTokenHandler(dependencies)})
	registerMethods(mux, "/api/v1/access-tokens/introspect", map[string]http.Handler{http.MethodPost: inspectAccessTokenHandler(dependencies)})
}

func createAccessTokenHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if dependencies.Workspace == nil {
			writeProblem(response, request, Problem{Title: "Service Unavailable", Status: http.StatusServiceUnavailable, Detail: "GPU Workspace storage is unavailable.", Code: "workspace_storage_unavailable"})
			return
		}
		var input createAccessTokenRequest
		if !decodeRequestJSON(response, request, &input) {
			return
		}
		workspaceID := request.PathValue("instanceID")
		current, err := dependencies.Workspace.GetWorkspace(request.Context(), workspaceID)
		if err != nil {
			writeWorkspaceError(response, request, err)
			return
		}
		principal, ok := workspacePrincipal(response, request, dependencies, "instance.access", current.ProjectID, workspaceID)
		if !ok {
			return
		}
		mutation, ok := mutationContext(response, request, principal, input)
		if !ok {
			return
		}
		issued, err := dependencies.Workspace.CreateAccessToken(request.Context(), workspace.CreateAccessTokenParams{Mutation: mutation, WorkspaceID: workspaceID, AccessType: workspace.AccessType(input.AccessType)})
		if err != nil {
			writeWorkspaceError(response, request, err)
			return
		}
		response.Header().Set("Location", "/api/v1/instances/"+workspaceID+"/access-tokens/"+issued.ID)
		writeJSON(response, http.StatusCreated, issued)
	}
}

func revokeAccessTokenHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if dependencies.Workspace == nil {
			writeProblem(response, request, Problem{Title: "Service Unavailable", Status: http.StatusServiceUnavailable, Detail: "GPU Workspace storage is unavailable.", Code: "workspace_storage_unavailable"})
			return
		}
		workspaceID := request.PathValue("instanceID")
		current, err := dependencies.Workspace.GetWorkspace(request.Context(), workspaceID)
		if err != nil {
			writeWorkspaceError(response, request, err)
			return
		}
		principal, ok := workspacePrincipal(response, request, dependencies, "instance.access", current.ProjectID, workspaceID)
		if !ok {
			return
		}
		input := map[string]string{"workspaceId": workspaceID, "tokenId": request.PathValue("tokenID")}
		mutation, ok := mutationContext(response, request, principal, input)
		if !ok {
			return
		}
		accepted, err := dependencies.Workspace.RevokeAccessToken(request.Context(), workspace.RevokeAccessTokenParams{Mutation: mutation, WorkspaceID: workspaceID, TokenID: request.PathValue("tokenID")})
		if err != nil {
			writeWorkspaceError(response, request, err)
			return
		}
		writeAcceptance(response, http.StatusAccepted, "/api/v1/instances/"+workspaceID+"/access-tokens/"+request.PathValue("tokenID"), accepted)
	}
}

func inspectAccessTokenHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if dependencies.Workspace == nil {
			writeProblem(response, request, Problem{Title: "Service Unavailable", Status: http.StatusServiceUnavailable, Detail: "GPU Workspace storage is unavailable.", Code: "workspace_storage_unavailable"})
			return
		}
		if _, ok := authenticateRequest(response, request, dependencies.Authenticator); !ok {
			return
		}
		var input inspectAccessTokenRequest
		if !decodeRequestJSON(response, request, &input) {
			return
		}
		result, err := dependencies.Workspace.InspectAccessToken(request.Context(), input.Token)
		if errors.Is(err, workspace.ErrNotFound) {
			writeProblem(response, request, Problem{Title: "Unauthorized", Status: http.StatusUnauthorized, Detail: "The access token is invalid, expired or revoked.", Code: "access_token_invalid"})
			return
		}
		if err != nil {
			writeWorkspaceError(response, request, err)
			return
		}
		writeJSON(response, http.StatusOK, result)
	}
}

func workspacePrincipal(response http.ResponseWriter, request *http.Request, dependencies Dependencies, action, scopeID, resourceID string) (authn.Principal, bool) {
	principal, ok := authenticateRequest(response, request, dependencies.Authenticator)
	if !ok {
		return authn.Principal{}, false
	}
	if !authorizeRequest(response, request, dependencies.Authorization, principal, ports.AuthorizationRequest{Action: action, ScopeType: string(tenancy.ScopeProject), ScopeID: scopeID, Resource: "instance", ResourceID: resourceID}) {
		return authn.Principal{}, false
	}
	if dependencies.Workspace == nil {
		writeProblem(response, request, Problem{Title: "Service Unavailable", Status: http.StatusServiceUnavailable, Detail: "GPU Workspace storage is unavailable.", Code: "workspace_storage_unavailable"})
		return authn.Principal{}, false
	}
	return principal, true
}

func createWorkspaceHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		var input createWorkspaceRequest
		if !decodeRequestJSON(response, request, &input) {
			return
		}
		principal, ok := workspacePrincipal(response, request, dependencies, "instance.create", input.ProjectID, "")
		if !ok {
			return
		}
		mutation, ok := mutationContext(response, request, principal, input)
		if !ok {
			return
		}
		accepted, err := dependencies.Workspace.CreateWorkspace(request.Context(), workspace.CreateParams{Mutation: mutation, ProjectID: input.ProjectID, ClusterID: input.ClusterID, AcceleratorProfileID: input.AcceleratorProfileID, Name: input.Name, StorageGiB: input.StorageGiB})
		if err != nil {
			writeWorkspaceError(response, request, err)
			return
		}
		writeAcceptance(response, http.StatusAccepted, "/api/v1/instances/"+accepted.ResourceID, accepted)
	}
}

func getWorkspaceHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if dependencies.Workspace == nil {
			writeProblem(response, request, Problem{Title: "Service Unavailable", Status: http.StatusServiceUnavailable, Detail: "GPU Workspace storage is unavailable.", Code: "workspace_storage_unavailable"})
			return
		}
		result, err := dependencies.Workspace.GetWorkspace(request.Context(), request.PathValue("instanceID"))
		if errors.Is(err, workspace.ErrNotFound) {
			writeWorkspaceError(response, request, err)
			return
		}
		if err != nil {
			writeWorkspaceError(response, request, err)
			return
		}
		if _, ok := workspacePrincipal(response, request, dependencies, "instance.read", result.ProjectID, result.ID); !ok {
			return
		}
		writeJSON(response, http.StatusOK, result)
	}
}

func setWorkspaceDesiredStateHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if dependencies.Workspace == nil {
			writeProblem(response, request, Problem{Title: "Service Unavailable", Status: http.StatusServiceUnavailable, Detail: "GPU Workspace storage is unavailable.", Code: "workspace_storage_unavailable"})
			return
		}
		id := request.PathValue("instanceID")
		current, err := dependencies.Workspace.GetWorkspace(request.Context(), id)
		if err != nil {
			writeWorkspaceError(response, request, err)
			return
		}
		principal, ok := workspacePrincipal(response, request, dependencies, "instance.update", current.ProjectID, id)
		if !ok {
			return
		}
		var input setWorkspaceDesiredStateRequest
		if !decodeRequestJSON(response, request, &input) {
			return
		}
		mutation, ok := mutationContext(response, request, principal, input)
		if !ok {
			return
		}
		accepted, err := dependencies.Workspace.SetWorkspaceDesiredState(request.Context(), workspace.SetDesiredStateParams{Mutation: mutation, WorkspaceID: id, DesiredState: input.DesiredState})
		if err != nil {
			writeWorkspaceError(response, request, err)
			return
		}
		writeAcceptance(response, http.StatusAccepted, "/api/v1/instances/"+id, accepted)
	}
}

func writeWorkspaceError(response http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, workspace.ErrInvalid):
		writeProblem(response, request, Problem{Title: "Invalid request", Status: http.StatusBadRequest, Detail: strings.TrimSuffix(err.Error(), ": "+workspace.ErrInvalid.Error()), Code: "invalid_workspace_request"})
	case errors.Is(err, workspace.ErrNotFound):
		writeProblem(response, request, Problem{Title: "Resource not found", Status: http.StatusNotFound, Detail: "The requested GPU Workspace does not exist.", Code: "workspace_not_found"})
	case errors.Is(err, workspace.ErrConflict):
		writeProblem(response, request, Problem{Title: "Resource conflict", Status: http.StatusConflict, Detail: "A GPU Workspace with the supplied identity already exists.", Code: "workspace_conflict"})
	case errors.Is(err, workspace.ErrQuotaExceeded):
		writeProblem(response, request, Problem{Title: "Quota exceeded", Status: http.StatusConflict, Detail: "The requested GPU capacity exceeds the project quota.", Code: "quota_exceeded"})
	case errors.Is(err, tenancy.ErrInvalid):
		writeProblem(response, request, Problem{Title: "Invalid request", Status: http.StatusBadRequest, Detail: "The access token request is invalid.", Code: "invalid_workspace_request"})
	case errors.Is(err, tenancy.ErrIdempotencyConflict):
		writeProblem(response, request, Problem{Title: "Idempotency conflict", Status: http.StatusConflict, Detail: "The Idempotency-Key was already used with a different request.", Code: "idempotency_conflict"})
	default:
		writeProblem(response, request, Problem{Title: "Internal Server Error", Status: http.StatusInternalServerError, Detail: "The GPU Workspace request could not be completed.", Code: "workspace_request_failed"})
	}
}
