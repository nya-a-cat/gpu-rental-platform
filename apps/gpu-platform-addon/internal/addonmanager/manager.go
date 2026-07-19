package addonmanager

import (
	"context"
	"embed"
	"fmt"

	"github.com/nya-a-cat/gpu-rental-platform/apps/gpu-platform-addon/internal/inventory"
	"github.com/nya-a-cat/gpu-rental-platform/apps/gpu-platform-addon/internal/kubeconfig"
	"github.com/nya-a-cat/gpu-rental-platform/apps/gpu-platform-addon/internal/options"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	frameworkmanager "open-cluster-management.io/addon-framework/pkg/addonmanager"
	frameworkagent "open-cluster-management.io/addon-framework/pkg/agent"
	"open-cluster-management.io/addon-framework/pkg/utils"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
)

const agentName = "gpu-platform-addon-agent"

//go:embed manifests/templates/*.yaml
var manifestFS embed.FS

type manifestConfiguredAgentAddon struct {
	frameworkagent.AgentAddon
	manifestConfigs []workv1.ManifestConfigOption
}

func (a manifestConfiguredAgentAddon) GetAgentAddonOptions() frameworkagent.AgentAddonOptions {
	options := a.AgentAddon.GetAgentAddonOptions()
	options.ManifestConfigs = append(options.ManifestConfigs, a.manifestConfigs...)
	return options
}

func leaseManifestConfig(addonName, namespace string) workv1.ManifestConfigOption {
	return workv1.ManifestConfigOption{
		ResourceIdentifier: workv1.ResourceIdentifier{
			Group:     "coordination.k8s.io",
			Resource:  "leases",
			Name:      addonName,
			Namespace: namespace,
		},
		UpdateStrategy: &workv1.UpdateStrategy{
			Type: workv1.UpdateStrategyTypeServerSideApply,
		},
	}
}

func Run(ctx context.Context, opts options.Manager) error {
	hubConfig, err := kubeconfig.Load(opts.Kubeconfig)
	if err != nil {
		return fmt.Errorf("load hub client configuration: %w", err)
	}

	registration, err := registrationOption(hubConfig, opts.AddonName)
	if err != nil {
		return err
	}

	agentAddon, err := addonfactory.NewAgentAddonFactory(opts.AddonName, manifestFS, "manifests/templates").
		WithGetValuesFuncs(values(opts)).
		WithAgentRegistrationOption(registration).
		WithAgentInstallNamespace(func(context.Context, *addonv1beta1.ManagedClusterAddOn) (string, error) {
			return opts.AgentInstallNamespace, nil
		}).
		WithAgentHealthProber(&frameworkagent.HealthProber{Type: frameworkagent.HealthProberTypeLease}).
		BuildTemplateAgentAddon()
	if err != nil {
		return fmt.Errorf("build template agent addon: %w", err)
	}

	agentAddon = manifestConfiguredAgentAddon{
		AgentAddon:      agentAddon,
		manifestConfigs: []workv1.ManifestConfigOption{leaseManifestConfig(opts.AddonName, opts.AgentInstallNamespace)},
	}

	manager, err := frameworkmanager.New(hubConfig)
	if err != nil {
		return fmt.Errorf("create addon manager: %w", err)
	}
	if err := manager.AddAgent(agentAddon); err != nil {
		return fmt.Errorf("register addon agent: %w", err)
	}
	if err := manager.Start(ctx); err != nil {
		return fmt.Errorf("start addon manager: %w", err)
	}

	<-ctx.Done()
	return nil
}

func values(opts options.Manager) addonfactory.GetValuesFunc {
	return func(_ *clusterv1.ManagedCluster, addon *addonv1beta1.ManagedClusterAddOn) (addonfactory.Values, error) {
		return addonfactory.StructToValues(struct {
			Image          string
			AddonUID       string
			AddonName      string
			AgentName      string
			ReportInterval string
		}{
			Image:          opts.AgentImage,
			AddonUID:       string(addon.UID),
			AddonName:      opts.AddonName,
			AgentName:      agentName,
			ReportInterval: opts.ReportInterval.String(),
		}), nil
	}
}

func registrationOption(hubConfig *rest.Config, addonName string) (*frameworkagent.RegistrationOption, error) {
	hubClient, err := kubernetes.NewForConfig(hubConfig)
	if err != nil {
		return nil, fmt.Errorf("create hub RBAC client: %w", err)
	}

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name: "open-cluster-management:" + addonName + ":agent",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"create"},
			},
			{
				APIGroups:     []string{""},
				Resources:     []string{"configmaps"},
				ResourceNames: []string{inventory.ConfigMapName},
				Verbs:         []string{"get", "update"},
			},
		},
	}

	permissionConfig := utils.NewRBACPermissionConfigBuilder(hubClient).
		BindKubeClientRole(role).
		Build()

	return &frameworkagent.RegistrationOption{
		Configurations:   frameworkagent.KubeClientSignerConfigurations(addonName, agentName),
		CSRApproveCheck:  utils.DefaultCSRApprover(agentName),
		PermissionConfig: permissionConfig,
	}, nil
}
