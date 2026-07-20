package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/authn"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/catalog"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

type catalogStoreStub struct {
	createClusterParams catalog.CreateClusterParams
	replaceParams       catalog.ReplaceInventoryParams
	replaceErr          error
}

func (stub *catalogStoreStub) GetResourceClass(context.Context, string) (catalog.ResourceClass, error) {
	return catalog.ResourceClass{}, nil
}
func (stub *catalogStoreStub) CreateCluster(_ context.Context, params catalog.CreateClusterParams) (tenancy.Acceptance, error) {
	stub.createClusterParams = params
	return tenancy.Acceptance{ResourceID: "11111111-1111-4111-8111-111111111111", OperationID: "22222222-2222-4222-8222-222222222222"}, nil
}
func (stub *catalogStoreStub) GetCluster(context.Context, string) (catalog.Cluster, error) {
	return catalog.Cluster{}, nil
}
func (stub *catalogStoreStub) ReplaceClusterInventory(_ context.Context, params catalog.ReplaceInventoryParams) (tenancy.Acceptance, error) {
	stub.replaceParams = params
	if stub.replaceErr != nil {
		return tenancy.Acceptance{}, stub.replaceErr
	}
	return tenancy.Acceptance{ResourceID: params.ClusterID, OperationID: "22222222-2222-4222-8222-222222222222"}, nil
}
func (stub *catalogStoreStub) GetClusterInventory(context.Context, string) (catalog.ClusterInventory, error) {
	return catalog.ClusterInventory{}, nil
}
func (stub *catalogStoreStub) CreateAcceleratorProfile(context.Context, catalog.CreateAcceleratorProfileParams) (tenancy.Acceptance, error) {
	return tenancy.Acceptance{}, nil
}
func (stub *catalogStoreStub) GetAcceleratorProfile(context.Context, string) (catalog.AcceleratorProfile, error) {
	return catalog.AcceleratorProfile{}, nil
}
func (stub *catalogStoreStub) CreateCapacityPool(context.Context, catalog.CreateCapacityPoolParams) (tenancy.Acceptance, error) {
	return tenancy.Acceptance{}, nil
}
func (stub *catalogStoreStub) GetCapacityPool(context.Context, string) (catalog.CapacityPool, error) {
	return catalog.CapacityPool{}, nil
}

func catalogTestHandler(store CatalogStore, principal authn.Principal) http.Handler {
	return NewHandler(Dependencies{
		Logger: slog.Default(), Catalog: store,
		Authenticator:    authenticatorStub{principal: principal},
		ReadinessTimeout: time.Second,
	})
}

func TestCreateClusterRequiresSystemAdmin(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", strings.NewReader(`{"managedClusterName":"cluster-a","displayName":"Cluster A"}`))
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	catalogTestHandler(&catalogStoreStub{}, authn.Principal{ID: "operator"}).ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
}

func TestCreateClusterReturnsAcceptedOperation(t *testing.T) {
	store := &catalogStoreStub{}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", strings.NewReader(`{"managedClusterName":"cluster-a","displayName":"Cluster A"}`))
	request.Header.Set("Authorization", "Bearer token")
	request.Header.Set("Idempotency-Key", "cluster-create-0001")
	response := httptest.NewRecorder()
	catalogTestHandler(store, authn.Principal{ID: "admin", SystemAdmin: true}).ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if store.createClusterParams.ManagedClusterName != "cluster-a" || store.createClusterParams.Mutation.IdempotencyKey != "cluster-create-0001" {
		t.Fatalf("create params = %#v", store.createClusterParams)
	}
	if got := response.Header().Get("Operation-Location"); got != "/api/v1/operations/22222222-2222-4222-8222-222222222222" {
		t.Fatalf("Operation-Location = %q", got)
	}
}

func TestReplaceInventoryMapsGenerationConflict(t *testing.T) {
	store := &catalogStoreStub{replaceErr: catalog.ErrGenerationConflict}
	clusterID := "11111111-1111-4111-8111-111111111111"
	body := `{"expectedGeneration":1,"sourceGeneration":"` + strings.Repeat("a", 64) + `","agentEpoch":"epoch-0001","reportSequence":2,"fencingToken":"fence","fencingEnabled":true,"executionHealthy":true,"fenced":false,"observedAt":"2026-07-20T00:00:00Z","nodePools":[]}`
	request := httptest.NewRequest(http.MethodPut, "/api/v1/clusters/"+clusterID+"/inventory", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer token")
	request.Header.Set("Idempotency-Key", "inventory-replace-0001")
	response := httptest.NewRecorder()
	catalogTestHandler(store, authn.Principal{ID: "admin", SystemAdmin: true}).ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !errors.Is(store.replaceErr, catalog.ErrGenerationConflict) || store.replaceParams.ClusterID != clusterID {
		t.Fatalf("replace params = %#v", store.replaceParams)
	}
}
