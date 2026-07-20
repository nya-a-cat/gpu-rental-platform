package sharedisolation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/outbox"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/ports"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

type repositoryStub struct {
	snapshot  ProjectSnapshot
	started   []ReconcileState
	completed []ReconcileState
	failed    []ReconcileState
	terminal  bool
}

func (stub *repositoryStub) LoadProjectForIsolation(context.Context, string) (ProjectSnapshot, error) {
	return stub.snapshot, nil
}

func (stub *repositoryStub) StartSharedIsolation(_ context.Context, state ReconcileState) error {
	stub.started = append(stub.started, state)
	return nil
}

func (stub *repositoryStub) CompleteSharedIsolation(_ context.Context, state ReconcileState) error {
	stub.completed = append(stub.completed, state)
	return nil
}

func (stub *repositoryStub) FailSharedIsolation(_ context.Context, state ReconcileState, _ error, terminal bool) error {
	stub.failed = append(stub.failed, state)
	stub.terminal = terminal
	return nil
}

type fleetStub struct {
	requests    []ports.WorkRequest
	err         error
	unavailable bool
}

func (stub *fleetStub) Place(context.Context, ports.PlacementRequest) (ports.PlacementResult, error) {
	return ports.PlacementResult{}, errors.New("unused")
}

func (stub *fleetStub) ApplyWork(_ context.Context, request ports.WorkRequest) (ports.WorkResult, error) {
	stub.requests = append(stub.requests, request)
	ready := stub.err == nil && !stub.unavailable
	return ports.WorkResult{WorkID: request.WorkID, Applied: ready, Available: ready}, stub.err
}

func TestReconcilerAppliesAndCompletesProject(t *testing.T) {
	repository := &repositoryStub{snapshot: testSnapshot()}
	fleet := &fleetStub{}
	reconciler, err := NewReconciler(repository, fleet, "cluster1", "open-cluster-management-agent-addon", "gpu-platform-addon-agent")
	if err != nil {
		t.Fatalf("NewReconciler() error = %v", err)
	}
	if err := reconciler.Reconcile(context.Background(), testProjectEvent(t), false); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(repository.started) != 1 || len(repository.completed) != 1 || len(repository.failed) != 0 {
		t.Fatalf("repository calls start/complete/fail = %d/%d/%d", len(repository.started), len(repository.completed), len(repository.failed))
	}
	if len(fleet.requests) != 1 || fleet.requests[0].ClusterID != "cluster1" {
		t.Fatalf("fleet requests = %#v", fleet.requests)
	}
}

func TestReconcilerRejectsIncompleteWorkResult(t *testing.T) {
	repository := &repositoryStub{snapshot: testSnapshot()}
	fleet := &fleetStub{unavailable: true}
	reconciler, err := NewReconciler(repository, fleet, "cluster1", "open-cluster-management-agent-addon", "gpu-platform-addon-agent")
	if err != nil {
		t.Fatalf("NewReconciler() error = %v", err)
	}
	err = reconciler.Reconcile(context.Background(), testProjectEvent(t), false)
	if err == nil || len(repository.failed) != 1 || repository.terminal {
		t.Fatalf("Reconcile() error = %v, failures = %d, terminal = %v", err, len(repository.failed), repository.terminal)
	}
}

func TestReconcilerRecordsTerminalFailure(t *testing.T) {
	repository := &repositoryStub{snapshot: testSnapshot()}
	fleet := &fleetStub{err: errors.New("hub unavailable")}
	reconciler, err := NewReconciler(repository, fleet, "cluster1", "open-cluster-management-agent-addon", "gpu-platform-addon-agent")
	if err != nil {
		t.Fatalf("NewReconciler() error = %v", err)
	}
	err = reconciler.Reconcile(context.Background(), testProjectEvent(t), true)
	if err == nil || len(repository.failed) != 1 || !repository.terminal {
		t.Fatalf("Reconcile() error = %v, failures = %d, terminal = %v", err, len(repository.failed), repository.terminal)
	}
}

func TestReconcilerSkipsAlreadyObservedWork(t *testing.T) {
	snapshot := testSnapshot()
	clusterID := "cluster1"
	workName := WorkName(snapshot.Project.ID)
	snapshot.Project.TargetClusterID = &clusterID
	snapshot.Project.ManifestWorkName = &workName
	snapshot.Project.ObservedState = "active"
	snapshot.Project.ProvisioningState = "succeeded"
	snapshot.Project.ObservedGeneration = snapshot.Project.Generation
	snapshot.Project.AppliedGPUQuota = snapshot.GPUQuota
	repository := &repositoryStub{snapshot: snapshot}
	fleet := &fleetStub{}
	reconciler, err := NewReconciler(repository, fleet, clusterID, "open-cluster-management-agent-addon", "gpu-platform-addon-agent")
	if err != nil {
		t.Fatalf("NewReconciler() error = %v", err)
	}
	if err := reconciler.Reconcile(context.Background(), testProjectEvent(t), false); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(fleet.requests) != 0 || len(repository.started) != 0 || len(repository.completed) != 1 {
		t.Fatalf("ready reconciliation calls fleet/start/complete = %d/%d/%d", len(fleet.requests), len(repository.started), len(repository.completed))
	}
}

func TestEventIdentityUsesProjectIDForGPUQuotaUpdates(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"resourceId":    "11111111-2222-4333-8444-555555555555/gpu.nvidia.full",
		"projectId":     "11111111-2222-4333-8444-555555555555",
		"operationId":   "99999999-8888-4777-8666-555555555555",
		"resourceClass": GPUQuotaResourceClass,
	})
	if err != nil {
		t.Fatalf("marshal quota event: %v", err)
	}
	projectID, operationID, err := eventIdentity(outbox.Event{EventType: "project.gpu-quota.updated", Payload: payload})
	if err != nil {
		t.Fatalf("eventIdentity() error = %v", err)
	}
	if projectID != "11111111-2222-4333-8444-555555555555" || operationID != "99999999-8888-4777-8666-555555555555" {
		t.Fatalf("event identity = %q/%q", projectID, operationID)
	}
}

func testSnapshot() ProjectSnapshot {
	return ProjectSnapshot{
		Project: tenancy.Project{
			ID:                "11111111-2222-4333-8444-555555555555",
			TenantID:          "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
			IsolationClass:    tenancy.IsolationShared,
			NamespaceName:     "gpu-p-111111112222",
			DesiredState:      "active",
			ObservedState:     "pending",
			ProvisioningState: "pending",
			Generation:        1,
		},
		GPUQuota: 2,
	}
}

func testProjectEvent(t *testing.T) outbox.Event {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"resourceId":  "11111111-2222-4333-8444-555555555555",
		"operationId": "99999999-8888-4777-8666-555555555555",
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return outbox.Event{EventType: "project.created", Payload: payload}
}
