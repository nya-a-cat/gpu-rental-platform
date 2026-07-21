package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/authn"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/placement"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

type PlacementStore interface {
	CreatePlacement(context.Context, placement.CreateParams) (tenancy.Acceptance, error)
	GetPlacement(context.Context, string) (placement.Decision, error)
}

type createPlacementRequest struct {
	ProjectID            string            `json:"projectId"`
	AcceleratorProfileID string            `json:"acceleratorProfileId"`
	Quantity             int               `json:"quantity"`
	Traits               map[string]string `json:"traits"`
}

func registerPlacementRoutes(mux *http.ServeMux, dependencies Dependencies) {
	registerMethods(mux, "/api/v1/placements", map[string]http.Handler{http.MethodPost: createPlacementHandler(dependencies)})
	registerMethods(mux, "/api/v1/placements/{placementID}", map[string]http.Handler{http.MethodGet: getPlacementHandler(dependencies)})
}

func placementPrincipal(response http.ResponseWriter, request *http.Request, dependencies Dependencies) (authn.Principal, bool) {
	principal, ok := authenticateRequest(response, request, dependencies.Authenticator)
	if !ok {
		return authn.Principal{}, false
	}
	if !principal.SystemAdmin {
		writeProblem(response, request, Problem{Title: "Forbidden", Status: http.StatusForbidden, Detail: "System administrator access is required for placement operations.", Code: "forbidden"})
		return authn.Principal{}, false
	}
	if dependencies.Placement == nil {
		writeProblem(response, request, Problem{Title: "Service Unavailable", Status: http.StatusServiceUnavailable, Detail: "Placement storage is unavailable.", Code: "placement_storage_unavailable"})
		return authn.Principal{}, false
	}
	return principal, true
}

func createPlacementHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := placementPrincipal(response, request, dependencies)
		if !ok {
			return
		}
		var input createPlacementRequest
		if !decodeRequestJSON(response, request, &input) {
			return
		}
		mutation, ok := mutationContext(response, request, principal, input)
		if !ok {
			return
		}
		accepted, err := dependencies.Placement.CreatePlacement(request.Context(), placement.CreateParams{Mutation: mutation, ProjectID: input.ProjectID, AcceleratorProfileID: input.AcceleratorProfileID, Quantity: input.Quantity, Traits: input.Traits})
		if err != nil {
			writePlacementError(response, request, err)
			return
		}
		writeAcceptance(response, http.StatusAccepted, "/api/v1/placements/"+accepted.ResourceID, accepted)
	}
}

func getPlacementHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if _, ok := placementPrincipal(response, request, dependencies); !ok {
			return
		}
		result, err := dependencies.Placement.GetPlacement(request.Context(), request.PathValue("placementID"))
		if err != nil {
			writePlacementError(response, request, err)
			return
		}
		writeJSON(response, http.StatusOK, result)
	}
}

func writePlacementError(response http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, placement.ErrInvalid):
		writeProblem(response, request, Problem{Title: "Invalid request", Status: http.StatusBadRequest, Detail: "The placement request is invalid.", Code: "invalid_placement_request"})
	case errors.Is(err, placement.ErrNotFound):
		writeProblem(response, request, Problem{Title: "Resource not found", Status: http.StatusNotFound, Detail: "The placement decision does not exist.", Code: "placement_not_found"})
	case errors.Is(err, placement.ErrCapacity):
		writeProblem(response, request, Problem{Title: "Capacity unavailable", Status: http.StatusConflict, Detail: "No enabled capacity pool can satisfy the placement request.", Code: "capacity_unavailable"})
	default:
		writeProblem(response, request, Problem{Title: "Internal Server Error", Status: http.StatusInternalServerError, Detail: "The placement request could not be completed.", Code: "placement_request_failed"})
	}
}
