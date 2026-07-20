package addonagent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/gpu-platform-addon/internal/inventory"
	"github.com/nya-a-cat/gpu-rental-platform/apps/gpu-platform-addon/internal/kubeconfig"
	"github.com/nya-a-cat/gpu-rental-platform/apps/gpu-platform-addon/internal/options"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"open-cluster-management.io/addon-framework/pkg/lease"
)

const (
	managedByLabel  = "gpu-platform-addon"
	agentEpochBytes = 16
)

func Run(ctx context.Context, opts options.Agent) error {
	agentEpoch, err := newAgentEpoch()
	if err != nil {
		return fmt.Errorf("generate agent epoch: %w", err)
	}
	managedConfig, err := kubeconfig.Load(opts.ManagedKubeconfig)
	if err != nil {
		return fmt.Errorf("load managed-cluster client configuration: %w", err)
	}
	hubConfig, err := kubeconfig.Load(opts.HubKubeconfig)
	if err != nil {
		return fmt.Errorf("load hub client configuration: %w", err)
	}

	managedClient, err := kubernetes.NewForConfig(managedConfig)
	if err != nil {
		return fmt.Errorf("create managed-cluster client: %w", err)
	}
	hubClient, err := kubernetes.NewForConfig(hubConfig)
	if err != nil {
		return fmt.Errorf("create hub client: %w", err)
	}

	leaseUpdater := lease.NewLeaseUpdater(managedClient, opts.AddonName, opts.AddonInstallNamespace)
	go leaseUpdater.Start(ctx)

	reporter := &reporter{
		managedClient: managedClient,
		hubClient:     hubClient,
		clusterName:   opts.ClusterName,
		addonName:     opts.AddonName,
		addonUID:      opts.AddonUID,
		agentEpoch:    agentEpoch,
	}
	reporter.Run(ctx, opts.ReportInterval)
	return nil
}

type reporter struct {
	managedClient kubernetes.Interface
	hubClient     kubernetes.Interface
	clusterName   string
	addonName     string
	addonUID      string
	agentEpoch    string
	sequence      uint64
}

func (r *reporter) Run(ctx context.Context, interval time.Duration) {
	r.reportAndLog(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reportAndLog(ctx)
		}
	}
}

func (r *reporter) reportAndLog(ctx context.Context) {
	if err := r.report(ctx, time.Now()); err != nil {
		log.Printf("inventory report failed: %v", err)
	}
}

func (r *reporter) report(ctx context.Context, observedAt time.Time) error {
	if r.agentEpoch == "" {
		return fmt.Errorf("agent epoch is required")
	}

	nodes, err := r.managedClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list managed-cluster nodes: %w", err)
	}

	snapshot := inventory.Aggregate(r.clusterName, nodes.Items, observedAt)
	r.sequence++
	snapshot.AgentEpoch = r.agentEpoch
	snapshot.Sequence = r.sequence
	snapshot.FencingToken = r.addonUID
	snapshot.FencingEnabled = r.addonUID != ""
	data, err := inventory.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("serialize accelerator inventory: %w", err)
	}

	configMaps := r.hubClient.CoreV1().ConfigMaps(r.clusterName)
	ownerReferences := r.inventoryOwnerReferences()
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing, err := configMaps.Get(ctx, inventory.ConfigMapName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			_, createErr := configMaps.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      inventory.ConfigMapName,
					Namespace: r.clusterName,
					Labels: map[string]string{
						"app.kubernetes.io/name":       managedByLabel,
						"app.kubernetes.io/managed-by": managedByLabel,
					},
					OwnerReferences: ownerReferences,
				},
				Data: map[string]string{inventory.ConfigMapDataKey: string(data)},
			}, metav1.CreateOptions{})
			return createErr
		}
		if err != nil {
			return err
		}

		updated := existing.DeepCopy()
		if updated.Data == nil {
			updated.Data = map[string]string{}
		}
		updated.Data[inventory.ConfigMapDataKey] = string(data)
		if updated.Labels == nil {
			updated.Labels = map[string]string{}
		}
		updated.Labels["app.kubernetes.io/name"] = managedByLabel
		updated.Labels["app.kubernetes.io/managed-by"] = managedByLabel
		if len(ownerReferences) != 0 {
			updated.OwnerReferences = ownerReferences
		}
		_, err = configMaps.Update(ctx, updated, metav1.UpdateOptions{})
		return err
	})
}

func (r *reporter) inventoryOwnerReferences() []metav1.OwnerReference {
	if r.addonUID == "" {
		return nil
	}

	controller := true
	return []metav1.OwnerReference{{
		APIVersion: "addon.open-cluster-management.io/v1beta1",
		Kind:       "ManagedClusterAddOn",
		Name:       r.addonName,
		UID:        types.UID(r.addonUID),
		Controller: &controller,
	}}
}

func newAgentEpoch() (string, error) {
	value := make([]byte, agentEpochBytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
