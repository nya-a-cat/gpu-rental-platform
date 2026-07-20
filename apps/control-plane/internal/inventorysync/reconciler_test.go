package inventorysync

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/catalog"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

type readerStub struct {
	payload []byte
	err     error
	cluster string
}

func (stub *readerStub) ReadInventory(_ context.Context, cluster string) ([]byte, error) {
	stub.cluster = cluster
	return stub.payload, stub.err
}

type repositoryStub struct {
	clusters       []catalog.Cluster
	current        catalog.Cluster
	listErr        error
	getErr         error
	heartbeatCalls []catalog.ObserveClusterHeartbeatParams
	heartbeatErr   error
	replaceCalls   []catalog.ReplaceInventoryParams
	replaceErrors  []error
}

func (stub *repositoryStub) ListClusters(context.Context) ([]catalog.Cluster, error) {
	return stub.clusters, stub.listErr
}

func (stub *repositoryStub) GetCluster(context.Context, string) (catalog.Cluster, error) {
	return stub.current, stub.getErr
}

func (stub *repositoryStub) ObserveClusterHeartbeat(_ context.Context, params catalog.ObserveClusterHeartbeatParams) error {
	stub.heartbeatCalls = append(stub.heartbeatCalls, params)
	return stub.heartbeatErr
}

func (stub *repositoryStub) ReplaceClusterInventory(_ context.Context, params catalog.ReplaceInventoryParams) (tenancy.Acceptance, error) {
	stub.replaceCalls = append(stub.replaceCalls, params)
	index := len(stub.replaceCalls) - 1
	if index < len(stub.replaceErrors) {
		return tenancy.Acceptance{}, stub.replaceErrors[index]
	}
	return tenancy.Acceptance{ResourceID: params.ClusterID}, nil
}

func TestReconcilerReplacesDetailedInventoryAndRetriesGenerationConflict(t *testing.T) {
	cluster := testCluster(4)
	reader := &readerStub{payload: detailedPayload(12)}
	repository := &repositoryStub{current: testCluster(5), replaceErrors: []error{catalog.ErrGenerationConflict, nil}}
	reconciler, err := NewReconciler(repository, reader)
	if err != nil {
		t.Fatalf("NewReconciler() error = %v", err)
	}
	if err := reconciler.Sync(context.Background(), cluster); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if reader.cluster != cluster.ManagedClusterName || len(repository.replaceCalls) != 2 {
		t.Fatalf("reader cluster/replacements = %q/%d", reader.cluster, len(repository.replaceCalls))
	}
	if repository.replaceCalls[0].ExpectedGeneration != 4 || repository.replaceCalls[1].ExpectedGeneration != 5 {
		t.Fatalf("expected generations = %d/%d", repository.replaceCalls[0].ExpectedGeneration, repository.replaceCalls[1].ExpectedGeneration)
	}
	mutation := repository.replaceCalls[0].Mutation
	if mutation.PrincipalID != "system:inventory-sync" || len(mutation.RequestHash) != 64 || mutation.IdempotencyKey == "" || mutation.RequestID == "" {
		t.Fatalf("inventory mutation = %#v", mutation)
	}
}

func TestReconcilerObservesLegacyHeartbeatWithoutReplacingInventory(t *testing.T) {
	cluster := testCluster(3)
	reader := &readerStub{payload: legacyPayload(13)}
	repository := &repositoryStub{}
	reconciler, _ := NewReconciler(repository, reader)
	if err := reconciler.Sync(context.Background(), cluster); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if len(repository.heartbeatCalls) != 1 || len(repository.replaceCalls) != 0 || repository.heartbeatCalls[0].ClusterID != cluster.ID {
		t.Fatalf("heartbeat/replacement calls = %#v/%d", repository.heartbeatCalls, len(repository.replaceCalls))
	}
}

func TestReconcilerTreatsStaleReportsAsConverged(t *testing.T) {
	reader := &readerStub{payload: detailedPayload(14)}
	repository := &repositoryStub{replaceErrors: []error{catalog.ErrStaleReport}}
	reconciler, _ := NewReconciler(repository, reader)
	if err := reconciler.Sync(context.Background(), testCluster(2)); err != nil {
		t.Fatalf("Sync() stale error = %v", err)
	}
}

type synchronizerStub struct {
	clusters []catalog.Cluster
}

func (stub *synchronizerStub) Sync(_ context.Context, cluster catalog.Cluster) error {
	stub.clusters = append(stub.clusters, cluster)
	return nil
}

func TestRunnerPollsEveryRegisteredCluster(t *testing.T) {
	clusters := []catalog.Cluster{
		testCluster(1),
		{ID: "22222222-2222-4222-8222-222222222222", ManagedClusterName: "cluster-b"},
	}
	repository := &repositoryStub{clusters: clusters}
	synchronizer := &synchronizerStub{}
	runner, err := NewRunner(slog.New(slog.NewTextHandler(io.Discard, nil)), repository, synchronizer, RunnerConfig{
		PollInterval: time.Second, SyncTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	runner.poll(context.Background())
	if len(synchronizer.clusters) != 2 || synchronizer.clusters[1].ManagedClusterName != "cluster-b" {
		t.Fatalf("synchronized clusters = %#v", synchronizer.clusters)
	}
}

func testCluster(generation int64) catalog.Cluster {
	return catalog.Cluster{
		ID: "11111111-1111-4111-8111-111111111111", ManagedClusterName: "cluster-a",
		InventoryGeneration: generation,
	}
}

func detailedPayload(sequence uint64) []byte {
	return []byte(`{
		"schemaVersion":"gpu.platform.nyaacat.dev/v1alpha1",
		"clusterName":"cluster-a",
		"agentEpoch":"0123456789abcdef0123456789abcdef",
		"sequence":` + strconv.FormatUint(sequence, 10) + `,
		"fencingToken":"addon-uid",
		"fencingEnabled":true,
		"generation":"` + strings.Repeat("a", 64) + `",
		"observedAt":"2026-07-20T12:00:00Z",
		"executionHealthy":true,
		"fenced":false,
		"nodeCount":0,
		"schedulableNodeCount":0,
		"resources":[],
		"nodePools":[]
	}`)
}

func legacyPayload(sequence uint64) []byte {
	return []byte(`{
		"schemaVersion":"gpu.platform.nyaacat.dev/v1alpha1",
		"clusterName":"cluster-a",
		"agentEpoch":"0123456789abcdef0123456789abcdef",
		"sequence":` + strconv.FormatUint(sequence, 10) + `,
		"fencingToken":"addon-uid",
		"fencingEnabled":true,
		"generation":"legacy",
		"observedAt":"2026-07-20T12:00:00Z",
		"executionHealthy":true,
		"fenced":false,
		"nodeCount":0,
		"schedulableNodeCount":0,
		"resources":[]
	}`)
}
