package inventorysync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/catalog"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

type Reader interface {
	ReadInventory(context.Context, string) ([]byte, error)
}

type Repository interface {
	ListClusters(context.Context) ([]catalog.Cluster, error)
	GetCluster(context.Context, string) (catalog.Cluster, error)
	ObserveClusterHeartbeat(context.Context, catalog.ObserveClusterHeartbeatParams) error
	ReplaceClusterInventory(context.Context, catalog.ReplaceInventoryParams) (tenancy.Acceptance, error)
}

type Reconciler struct {
	repository Repository
	reader     Reader
}

func NewReconciler(repository Repository, reader Reader) (*Reconciler, error) {
	if repository == nil || reader == nil {
		return nil, errors.New("inventory sync repository and reader are required")
	}
	return &Reconciler{repository: repository, reader: reader}, nil
}

func (reconciler *Reconciler) Sync(ctx context.Context, cluster catalog.Cluster) error {
	payload, err := reconciler.reader.ReadInventory(ctx, cluster.ManagedClusterName)
	if err != nil {
		return fmt.Errorf("read inventory for %s: %w", cluster.ManagedClusterName, err)
	}
	report, err := Decode(payload, cluster.ManagedClusterName)
	if err != nil {
		return fmt.Errorf("decode inventory for %s: %w", cluster.ManagedClusterName, err)
	}
	if !report.Detailed {
		err := reconciler.repository.ObserveClusterHeartbeat(ctx, heartbeatParams(cluster.ID, report))
		if errors.Is(err, catalog.ErrStaleReport) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("observe legacy inventory heartbeat for %s: %w", cluster.ManagedClusterName, err)
		}
		return nil
	}

	params := replacementParams(cluster.ID, cluster.InventoryGeneration, report, payload)
	_, err = reconciler.repository.ReplaceClusterInventory(ctx, params)
	if errors.Is(err, catalog.ErrGenerationConflict) {
		current, getErr := reconciler.repository.GetCluster(ctx, cluster.ID)
		if getErr != nil {
			return fmt.Errorf("reload cluster after inventory generation conflict: %w", getErr)
		}
		params.ExpectedGeneration = current.InventoryGeneration
		_, err = reconciler.repository.ReplaceClusterInventory(ctx, params)
	}
	if errors.Is(err, catalog.ErrStaleReport) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("replace inventory for %s: %w", cluster.ManagedClusterName, err)
	}
	return nil
}

func heartbeatParams(clusterID string, report Report) catalog.ObserveClusterHeartbeatParams {
	return catalog.ObserveClusterHeartbeatParams{
		ClusterID: clusterID, AgentEpoch: report.AgentEpoch, ReportSequence: report.Sequence,
		FencingToken: report.FencingToken, FencingEnabled: report.FencingEnabled,
		ExecutionHealthy: report.ExecutionHealthy, Fenced: report.Fenced, ObservedAt: report.ObservedAt,
	}
}

func replacementParams(clusterID string, expectedGeneration int64, report Report, payload []byte) catalog.ReplaceInventoryParams {
	payloadDigest := sha256.Sum256(payload)
	identityDigest := sha256.Sum256([]byte(clusterID + "\x00" + report.AgentEpoch + "\x00" + strconv.FormatUint(report.Sequence, 10)))
	identity := hex.EncodeToString(identityDigest[:])
	return catalog.ReplaceInventoryParams{
		Mutation: tenancy.MutationContext{
			PrincipalID: "system:inventory-sync", RequestID: "inventory-sync-" + identity[:32],
			IdempotencyKey: "inventory-sync-" + identity, RequestHash: hex.EncodeToString(payloadDigest[:]),
		},
		ClusterID: clusterID, ExpectedGeneration: expectedGeneration, SourceGeneration: report.SourceGeneration,
		AgentEpoch: report.AgentEpoch, ReportSequence: report.Sequence,
		FencingToken: report.FencingToken, FencingEnabled: report.FencingEnabled,
		ExecutionHealthy: report.ExecutionHealthy, Fenced: report.Fenced,
		ObservedAt: report.ObservedAt, NodePools: report.NodePools,
	}
}
