package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/outbox"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/ports"
)

type reconcilerRepositoryStub struct {
	snapshot                   Workspace
	started, completed, failed int
	terminal                   bool
}

func (stub *reconcilerRepositoryStub) LoadWorkspace(context.Context, string) (Workspace, error) {
	return stub.snapshot, nil
}
func (stub *reconcilerRepositoryStub) StartWorkspace(context.Context, ReconcileState) error {
	stub.started++
	return nil
}
func (stub *reconcilerRepositoryStub) CompleteWorkspace(context.Context, ReconcileState) error {
	stub.completed++
	return nil
}
func (stub *reconcilerRepositoryStub) FailWorkspace(_ context.Context, _ ReconcileState, _ error, terminal bool) error {
	stub.failed++
	stub.terminal = terminal
	return nil
}

type reconcilerFleetStub struct {
	err      error
	requests []ports.WorkRequest
}

func (stub *reconcilerFleetStub) Place(context.Context, ports.PlacementRequest) (ports.PlacementResult, error) {
	return ports.PlacementResult{}, errors.New("unused")
}
func (stub *reconcilerFleetStub) ApplyWork(_ context.Context, request ports.WorkRequest) (ports.WorkResult, error) {
	stub.requests = append(stub.requests, request)
	if stub.err != nil {
		return ports.WorkResult{}, stub.err
	}
	return ports.WorkResult{WorkID: request.WorkID, Applied: true, Available: true}, nil
}

func TestReconcilerAppliesAndCompletesWorkspace(t *testing.T) {
	repository := &reconcilerRepositoryStub{snapshot: testWorkspace()}
	fleet := &reconcilerFleetStub{}
	reconciler, err := NewReconciler(repository, fleet)
	if err != nil {
		t.Fatal(err)
	}
	if err := reconciler.Reconcile(context.Background(), testWorkspaceEvent(t), false); err != nil {
		t.Fatal(err)
	}
	if repository.started != 1 || repository.completed != 1 || repository.failed != 0 || len(fleet.requests) != 1 {
		t.Fatalf("calls start/complete/fail/work = %d/%d/%d/%d", repository.started, repository.completed, repository.failed, len(fleet.requests))
	}
}

func TestReconcilerRecordsTerminalFailure(t *testing.T) {
	repository := &reconcilerRepositoryStub{snapshot: testWorkspace()}
	reconciler, err := NewReconciler(repository, &reconcilerFleetStub{err: errors.New("hub unavailable")})
	if err != nil {
		t.Fatal(err)
	}
	if err := reconciler.Reconcile(context.Background(), testWorkspaceEvent(t), true); err == nil || repository.failed != 1 || !repository.terminal {
		t.Fatalf("error=%v failed=%d terminal=%v", err, repository.failed, repository.terminal)
	}
}

func TestReconcilerSkipsObservedGeneration(t *testing.T) {
	snapshot := testWorkspace()
	snapshot.ObservedGeneration = snapshot.Generation
	snapshot.ProvisioningState = "succeeded"
	snapshot.ObservedState = "running"
	repository := &reconcilerRepositoryStub{snapshot: snapshot}
	fleet := &reconcilerFleetStub{}
	reconciler, err := NewReconciler(repository, fleet)
	if err != nil {
		t.Fatal(err)
	}
	if err := reconciler.Reconcile(context.Background(), testWorkspaceEvent(t), false); err != nil {
		t.Fatal(err)
	}
	if repository.completed != 1 || repository.started != 0 || len(fleet.requests) != 0 {
		t.Fatalf("calls start/complete/work = %d/%d/%d", repository.started, repository.completed, len(fleet.requests))
	}
}

func TestWorkspaceQuotaDeltaFollowsComputeLifecycle(t *testing.T) {
	if got := workspaceQuotaDelta(DesiredStopped, DesiredRunning, 2); got != 2 {
		t.Fatalf("stopped to running delta = %d", got)
	}
	if got := workspaceQuotaDelta(DesiredRunning, DesiredStopped, 2); got != -2 {
		t.Fatalf("running to stopped delta = %d", got)
	}
	if got := workspaceQuotaDelta(DesiredRunning, DesiredTerminated, 2); got != -2 {
		t.Fatalf("running to terminated delta = %d", got)
	}
	if got := workspaceQuotaDelta(DesiredStopped, DesiredTerminated, 2); got != 0 {
		t.Fatalf("stopped to terminated delta = %d", got)
	}
}

func testWorkspace() Workspace {
	return Workspace{ID: "11111111-2222-4333-8444-555555555555", ProjectID: "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee", ClusterID: "cluster-a", AcceleratorProfileID: "99999999-8888-4777-8666-555555555555", Name: "demo", GPUCount: 1, StorageGiB: 20, NamespaceName: "gpu-p-demo", DesiredState: DesiredRunning, ObservedState: "pending", ProvisioningState: "pending", Generation: 1}
}

func testWorkspaceEvent(t *testing.T) outbox.Event {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"resourceId": testWorkspace().ID, "operationId": "99999999-8888-4777-8666-555555555555"})
	if err != nil {
		t.Fatal(err)
	}
	return outbox.Event{EventType: "workspace.created", Payload: payload}
}
