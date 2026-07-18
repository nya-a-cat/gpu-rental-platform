package addonagent

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/gpu-platform-addon/internal/inventory"
	"github.com/nya-a-cat/gpu-rental-platform/apps/gpu-platform-addon/internal/kubeconfig"
	"github.com/nya-a-cat/gpu-rental-platform/apps/gpu-platform-addon/internal/options"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"open-cluster-management.io/addon-framework/pkg/lease"
)

const managedByLabel = "gpu-platform-addon"

func Run(ctx context.Context, opts options.Agent) error {
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
	}
	reporter.Run(ctx, opts.ReportInterval)
	return nil
}

type reporter struct {
	managedClient kubernetes.Interface
	hubClient     kubernetes.Interface
	clusterName   string
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
	nodes, err := r.managedClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list managed-cluster nodes: %w", err)
	}

	snapshot := inventory.Aggregate(r.clusterName, nodes.Items, observedAt)
	data, err := inventory.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("serialize accelerator inventory: %w", err)
	}

	configMaps := r.hubClient.CoreV1().ConfigMaps(r.clusterName)
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
		_, err = configMaps.Update(ctx, updated, metav1.UpdateOptions{})
		return err
	})
}
