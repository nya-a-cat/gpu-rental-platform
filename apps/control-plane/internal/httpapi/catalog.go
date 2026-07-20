package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/authn"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/catalog"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

type CatalogStore interface {
	GetResourceClass(context.Context, string) (catalog.ResourceClass, error)
	CreateCluster(context.Context, catalog.CreateClusterParams) (tenancy.Acceptance, error)
	GetCluster(context.Context, string) (catalog.Cluster, error)
	ReplaceClusterInventory(context.Context, catalog.ReplaceInventoryParams) (tenancy.Acceptance, error)
	GetClusterInventory(context.Context, string) (catalog.ClusterInventory, error)
	CreateAcceleratorProfile(context.Context, catalog.CreateAcceleratorProfileParams) (tenancy.Acceptance, error)
	GetAcceleratorProfile(context.Context, string) (catalog.AcceleratorProfile, error)
	CreateCapacityPool(context.Context, catalog.CreateCapacityPoolParams) (tenancy.Acceptance, error)
	GetCapacityPool(context.Context, string) (catalog.CapacityPool, error)
}

type createClusterRequest struct {
	ManagedClusterName string `json:"managedClusterName"`
	DisplayName        string `json:"displayName"`
}

type replaceInventoryRequest struct {
	ExpectedGeneration int64                      `json:"expectedGeneration"`
	SourceGeneration   string                     `json:"sourceGeneration"`
	AgentEpoch         string                     `json:"agentEpoch"`
	ReportSequence     uint64                     `json:"reportSequence"`
	FencingToken       string                     `json:"fencingToken"`
	FencingEnabled     bool                       `json:"fencingEnabled"`
	ExecutionHealthy   bool                       `json:"executionHealthy"`
	Fenced             bool                       `json:"fenced"`
	ObservedAt         time.Time                  `json:"observedAt"`
	NodePools          []catalog.NodePoolSnapshot `json:"nodePools"`
}

type createAcceleratorProfileRequest struct {
	Name            string                  `json:"name"`
	Slug            string                  `json:"slug"`
	AcceleratorMode catalog.AcceleratorMode `json:"acceleratorMode"`
	ResourceClass   string                  `json:"resourceClass"`
	GPUCount        int                     `json:"gpuCount"`
	MemoryMiB       *int64                  `json:"memoryMiB"`
	Traits          map[string]string       `json:"traits"`
}

type createCapacityPoolRequest struct {
	Name                 string                   `json:"name"`
	ClusterID            string                   `json:"clusterId"`
	NodePoolID           string                   `json:"nodePoolId"`
	AcceleratorProfileID string                   `json:"acceleratorProfileId"`
	SchedulerProfile     catalog.SchedulerProfile `json:"schedulerProfile"`
}

func registerCatalogRoutes(mux *http.ServeMux, dependencies Dependencies) {
	registerMethods(mux, "/api/v1/resource-classes/{resourceClass}", map[string]http.Handler{http.MethodGet: getResourceClassHandler(dependencies)})
	registerMethods(mux, "/api/v1/clusters", map[string]http.Handler{http.MethodPost: createClusterHandler(dependencies)})
	registerMethods(mux, "/api/v1/clusters/{clusterID}", map[string]http.Handler{http.MethodGet: getClusterHandler(dependencies)})
	registerMethods(mux, "/api/v1/clusters/{clusterID}/inventory", map[string]http.Handler{
		http.MethodGet: getClusterInventoryHandler(dependencies), http.MethodPut: replaceClusterInventoryHandler(dependencies),
	})
	registerMethods(mux, "/api/v1/accelerator-profiles", map[string]http.Handler{http.MethodPost: createAcceleratorProfileHandler(dependencies)})
	registerMethods(mux, "/api/v1/accelerator-profiles/{profileID}", map[string]http.Handler{http.MethodGet: getAcceleratorProfileHandler(dependencies)})
	registerMethods(mux, "/api/v1/capacity-pools", map[string]http.Handler{http.MethodPost: createCapacityPoolHandler(dependencies)})
	registerMethods(mux, "/api/v1/capacity-pools/{poolID}", map[string]http.Handler{http.MethodGet: getCapacityPoolHandler(dependencies)})
}

func catalogPrincipal(response http.ResponseWriter, request *http.Request, dependencies Dependencies) (authn.Principal, bool) {
	principal, ok := authenticateRequest(response, request, dependencies.Authenticator)
	if !ok {
		return authn.Principal{}, false
	}
	if !principal.SystemAdmin {
		writeProblem(response, request, Problem{Title: "Forbidden", Status: http.StatusForbidden, Detail: "System administrator access is required for vendor resource catalog operations.", Code: "forbidden"})
		return authn.Principal{}, false
	}
	if dependencies.Catalog == nil {
		writeProblem(response, request, Problem{Title: "Service Unavailable", Status: http.StatusServiceUnavailable, Detail: "Resource catalog storage is unavailable.", Code: "catalog_storage_unavailable"})
		return authn.Principal{}, false
	}
	return principal, true
}

func createClusterHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := catalogPrincipal(response, request, dependencies)
		if !ok {
			return
		}
		var input createClusterRequest
		if !decodeRequestJSON(response, request, &input) {
			return
		}
		mutation, ok := mutationContext(response, request, principal, input)
		if !ok {
			return
		}
		accepted, err := dependencies.Catalog.CreateCluster(request.Context(), catalog.CreateClusterParams{Mutation: mutation, ManagedClusterName: input.ManagedClusterName, DisplayName: input.DisplayName})
		if err != nil {
			writeCatalogError(response, request, err)
			return
		}
		writeAcceptance(response, http.StatusAccepted, "/api/v1/clusters/"+accepted.ResourceID, accepted)
	}
}

func getClusterHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if _, ok := catalogPrincipal(response, request, dependencies); !ok {
			return
		}
		result, err := dependencies.Catalog.GetCluster(request.Context(), request.PathValue("clusterID"))
		if err != nil {
			writeCatalogError(response, request, err)
			return
		}
		writeJSON(response, http.StatusOK, result)
	}
}

func replaceClusterInventoryHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := catalogPrincipal(response, request, dependencies)
		if !ok {
			return
		}
		var input replaceInventoryRequest
		if !decodeRequestJSON(response, request, &input) {
			return
		}
		mutation, ok := mutationContext(response, request, principal, input)
		if !ok {
			return
		}
		clusterID := request.PathValue("clusterID")
		accepted, err := dependencies.Catalog.ReplaceClusterInventory(request.Context(), catalog.ReplaceInventoryParams{
			Mutation: mutation, ClusterID: clusterID, ExpectedGeneration: input.ExpectedGeneration,
			SourceGeneration: input.SourceGeneration, AgentEpoch: input.AgentEpoch, ReportSequence: input.ReportSequence,
			FencingToken: input.FencingToken, FencingEnabled: input.FencingEnabled, ExecutionHealthy: input.ExecutionHealthy,
			Fenced: input.Fenced, ObservedAt: input.ObservedAt, NodePools: input.NodePools,
		})
		if err != nil {
			writeCatalogError(response, request, err)
			return
		}
		writeAcceptance(response, http.StatusAccepted, "/api/v1/clusters/"+clusterID+"/inventory", accepted)
	}
}

func getClusterInventoryHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if _, ok := catalogPrincipal(response, request, dependencies); !ok {
			return
		}
		result, err := dependencies.Catalog.GetClusterInventory(request.Context(), request.PathValue("clusterID"))
		if err != nil {
			writeCatalogError(response, request, err)
			return
		}
		writeJSON(response, http.StatusOK, result)
	}
}

func getResourceClassHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if _, ok := catalogPrincipal(response, request, dependencies); !ok {
			return
		}
		result, err := dependencies.Catalog.GetResourceClass(request.Context(), request.PathValue("resourceClass"))
		if err != nil {
			writeCatalogError(response, request, err)
			return
		}
		writeJSON(response, http.StatusOK, result)
	}
}

func createAcceleratorProfileHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := catalogPrincipal(response, request, dependencies)
		if !ok {
			return
		}
		var input createAcceleratorProfileRequest
		if !decodeRequestJSON(response, request, &input) {
			return
		}
		mutation, ok := mutationContext(response, request, principal, input)
		if !ok {
			return
		}
		accepted, err := dependencies.Catalog.CreateAcceleratorProfile(request.Context(), catalog.CreateAcceleratorProfileParams{
			Mutation: mutation, Name: input.Name, Slug: input.Slug, AcceleratorMode: input.AcceleratorMode,
			ResourceClass: input.ResourceClass, GPUCount: input.GPUCount, MemoryMiB: input.MemoryMiB, Traits: input.Traits,
		})
		if err != nil {
			writeCatalogError(response, request, err)
			return
		}
		writeAcceptance(response, http.StatusAccepted, "/api/v1/accelerator-profiles/"+accepted.ResourceID, accepted)
	}
}

func getAcceleratorProfileHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if _, ok := catalogPrincipal(response, request, dependencies); !ok {
			return
		}
		result, err := dependencies.Catalog.GetAcceleratorProfile(request.Context(), request.PathValue("profileID"))
		if err != nil {
			writeCatalogError(response, request, err)
			return
		}
		writeJSON(response, http.StatusOK, result)
	}
}

func createCapacityPoolHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := catalogPrincipal(response, request, dependencies)
		if !ok {
			return
		}
		var input createCapacityPoolRequest
		if !decodeRequestJSON(response, request, &input) {
			return
		}
		mutation, ok := mutationContext(response, request, principal, input)
		if !ok {
			return
		}
		accepted, err := dependencies.Catalog.CreateCapacityPool(request.Context(), catalog.CreateCapacityPoolParams{
			Mutation: mutation, Name: input.Name, ClusterID: input.ClusterID, NodePoolID: input.NodePoolID,
			AcceleratorProfileID: input.AcceleratorProfileID, SchedulerProfile: input.SchedulerProfile,
		})
		if err != nil {
			writeCatalogError(response, request, err)
			return
		}
		writeAcceptance(response, http.StatusAccepted, "/api/v1/capacity-pools/"+accepted.ResourceID, accepted)
	}
}

func getCapacityPoolHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if _, ok := catalogPrincipal(response, request, dependencies); !ok {
			return
		}
		result, err := dependencies.Catalog.GetCapacityPool(request.Context(), request.PathValue("poolID"))
		if err != nil {
			writeCatalogError(response, request, err)
			return
		}
		writeJSON(response, http.StatusOK, result)
	}
}

func writeCatalogError(response http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, catalog.ErrInvalid):
		detail := strings.TrimSuffix(err.Error(), ": "+catalog.ErrInvalid.Error())
		writeProblem(response, request, Problem{Title: "Invalid request", Status: http.StatusBadRequest, Detail: detail, Code: "invalid_catalog_request"})
	case errors.Is(err, catalog.ErrNotFound):
		writeProblem(response, request, Problem{Title: "Resource not found", Status: http.StatusNotFound, Detail: "The requested catalog resource does not exist.", Code: "catalog_resource_not_found"})
	case errors.Is(err, catalog.ErrGenerationConflict):
		writeProblem(response, request, Problem{Title: "Inventory generation conflict", Status: http.StatusConflict, Detail: "The inventory snapshot was based on an outdated generation.", Code: "inventory_generation_conflict"})
	case errors.Is(err, catalog.ErrStaleReport):
		writeProblem(response, request, Problem{Title: "Stale inventory report", Status: http.StatusConflict, Detail: "The inventory report sequence or observation time is stale.", Code: "stale_inventory_report"})
	case errors.Is(err, tenancy.ErrIdempotencyConflict):
		writeProblem(response, request, Problem{Title: "Idempotency conflict", Status: http.StatusConflict, Detail: "The Idempotency-Key was already used with a different request.", Code: "idempotency_conflict"})
	case errors.Is(err, catalog.ErrConflict):
		writeProblem(response, request, Problem{Title: "Resource conflict", Status: http.StatusConflict, Detail: "A catalog resource with the supplied identity already exists.", Code: "catalog_resource_conflict"})
	default:
		writeProblem(response, request, Problem{Title: "Internal Server Error", Status: http.StatusInternalServerError, Detail: "The resource catalog request could not be completed.", Code: "catalog_request_failed"})
	}
}
