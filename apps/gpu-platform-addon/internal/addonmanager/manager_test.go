package addonmanager

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"text/template"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	frameworkagent "open-cluster-management.io/addon-framework/pkg/agent"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
)

func TestAgentDeploymentTemplateIncludesManagedClusterAddOnUID(t *testing.T) {
	source, err := manifestFS.ReadFile("manifests/templates/30-deployment.yaml")
	if err != nil {
		t.Fatalf("read agent Deployment template: %v", err)
	}
	tmpl, err := template.New("agent-deployment").Parse(string(source))
	if err != nil {
		t.Fatalf("parse agent Deployment template: %v", err)
	}

	values := struct {
		AgentName             string
		AddonInstallNamespace string
		Image                 string
		AddonUID              string
		ClusterName           string
		AddonName             string
		ReportInterval        string
		HubKubeConfigSecret   string
	}{
		AgentName:             "gpu-platform-addon-agent",
		AddonInstallNamespace: "open-cluster-management-agent-addon",
		Image:                 "gpu-platform-addon:current",
		AddonUID:              "managed-cluster-addon-uid",
		ClusterName:           "cluster-a",
		AddonName:             "gpu-platform-addon",
		ReportInterval:        "15s",
		HubKubeConfigSecret:   "gpu-platform-addon-hub-kubeconfig",
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, values); err != nil {
		t.Fatalf("render agent Deployment template: %v", err)
	}
	if !strings.Contains(rendered.String(), "value: \"managed-cluster-addon-uid\"") {
		t.Fatalf("rendered Deployment is missing the add-on UID environment value:\n%s", rendered.String())
	}
}

func TestAgentLeaseTemplateIsManagedWithTheWorkload(t *testing.T) {
	source, err := manifestFS.ReadFile("manifests/templates/24-lease.yaml")
	if err != nil {
		t.Fatalf("read agent Lease template: %v", err)
	}
	tmpl, err := template.New("agent-lease").Parse(string(source))
	if err != nil {
		t.Fatalf("parse agent Lease template: %v", err)
	}

	values := struct {
		AddonName             string
		AddonInstallNamespace string
	}{
		AddonName:             "gpu-platform-addon",
		AddonInstallNamespace: "open-cluster-management-agent-addon",
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, values); err != nil {
		t.Fatalf("render agent Lease template: %v", err)
	}
	for _, expected := range []string{
		"kind: Lease",
		"name: gpu-platform-addon",
		"namespace: open-cluster-management-agent-addon",
	} {
		if !strings.Contains(rendered.String(), expected) {
			t.Fatalf("rendered Lease is missing %q:\n%s", expected, rendered.String())
		}
	}
}

func TestAgentLeaseUsesServerSideApply(t *testing.T) {
	config := leaseManifestConfig(
		"gpu-platform-addon",
		"open-cluster-management-agent-addon",
	)

	if config.ResourceIdentifier.Group != "coordination.k8s.io" ||
		config.ResourceIdentifier.Resource != "leases" ||
		config.ResourceIdentifier.Name != "gpu-platform-addon" ||
		config.ResourceIdentifier.Namespace != "open-cluster-management-agent-addon" {
		t.Fatalf("unexpected Lease resource identifier: %#v", config.ResourceIdentifier)
	}
	if config.UpdateStrategy == nil {
		t.Fatal("Lease update strategy is missing")
	}
	if config.UpdateStrategy.Type != workv1.UpdateStrategyTypeServerSideApply {
		t.Fatalf(
			"unexpected Lease update strategy %q",
			config.UpdateStrategy.Type,
		)
	}
}

func TestInventoryPermissionConfigBindsOnlyClusterUser(t *testing.T) {
	const roleName = "open-cluster-management:gpu-platform-addon:agent"
	existingBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: "cluster1",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: addonv1beta1.GroupVersion.String(),
				Kind:       "ManagedClusterAddOn",
				Name:       "gpu-platform-addon",
				UID:        "uid-cluster1",
			}},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     roleName,
		},
		Subjects: []rbacv1.Subject{{
			APIGroup: rbacv1.GroupName,
			Kind:     rbacv1.GroupKind,
			Name:     "system:open-cluster-management:addon:gpu-platform-addon",
		}},
	}
	client := fake.NewSimpleClientset(existingBinding)
	permissionConfig := inventoryPermissionConfig(client, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: roleName},
	})

	tests := []struct {
		clusterName string
		user        string
		uid         types.UID
	}{
		{
			clusterName: "cluster1",
			user:        "system:open-cluster-management:cluster:cluster1:addon:gpu-platform-addon:agent:gpu-platform-addon-agent",
			uid:         "uid-cluster1",
		},
		{
			clusterName: "cluster2",
			user:        "system:open-cluster-management:cluster:cluster2:addon:gpu-platform-addon:agent:gpu-platform-addon-agent",
			uid:         "uid-cluster2",
		},
	}

	for _, test := range tests {
		cluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: test.clusterName}}
		addon := registeredAddon(test.clusterName, test.user, test.uid)
		if err := permissionConfig(context.Background(), cluster, addon); err != nil {
			t.Fatalf("configure %s inventory permission: %v", test.clusterName, err)
		}

		binding, err := client.RbacV1().RoleBindings(test.clusterName).Get(context.Background(), roleName, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get %s RoleBinding: %v", test.clusterName, err)
		}
		if len(binding.Subjects) != 1 || binding.Subjects[0].Kind != rbacv1.UserKind || binding.Subjects[0].Name != test.user {
			t.Fatalf("%s RoleBinding subjects = %#v, want only cluster user", test.clusterName, binding.Subjects)
		}
		if binding.RoleRef.Kind != "Role" || binding.RoleRef.Name != roleName {
			t.Fatalf("%s RoleBinding roleRef = %#v", test.clusterName, binding.RoleRef)
		}
		if len(binding.OwnerReferences) != 1 || binding.OwnerReferences[0].UID != test.uid {
			t.Fatalf("%s RoleBinding owner references = %#v", test.clusterName, binding.OwnerReferences)
		}

		role, err := client.RbacV1().Roles(test.clusterName).Get(context.Background(), roleName, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get %s Role: %v", test.clusterName, err)
		}
		if len(role.OwnerReferences) != 1 || role.OwnerReferences[0].UID != test.uid {
			t.Fatalf("%s Role owner references = %#v", test.clusterName, role.OwnerReferences)
		}
	}
}

func TestInventoryPermissionConfigWaitsForClusterUser(t *testing.T) {
	client := fake.NewSimpleClientset()
	permissionConfig := inventoryPermissionConfig(client, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "inventory-role"},
	})
	cluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}
	addon := &addonv1beta1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-platform-addon", Namespace: "cluster1"},
		Status: addonv1beta1.ManagedClusterAddOnStatus{
			Registrations: []addonv1beta1.RegistrationConfig{{
				Type: addonv1beta1.KubeClient,
				KubeClient: &addonv1beta1.KubeClientConfig{
					Subject: addonv1beta1.KubeClientSubject{
						BaseSubject: addonv1beta1.BaseSubject{
							Groups: []string{"system:open-cluster-management:addon:gpu-platform-addon"},
						},
					},
				},
			}},
		},
	}

	err := permissionConfig(context.Background(), cluster, addon)
	var subjectNotReady *frameworkagent.SubjectNotReadyError
	if !errors.As(err, &subjectNotReady) {
		t.Fatalf("permission error = %v, want SubjectNotReadyError", err)
	}
}

func registeredAddon(clusterName, user string, uid types.UID) *addonv1beta1.ManagedClusterAddOn {
	return &addonv1beta1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gpu-platform-addon",
			Namespace: clusterName,
			UID:       uid,
		},
		Status: addonv1beta1.ManagedClusterAddOnStatus{
			Registrations: []addonv1beta1.RegistrationConfig{{
				Type: addonv1beta1.KubeClient,
				KubeClient: &addonv1beta1.KubeClientConfig{
					Subject: addonv1beta1.KubeClientSubject{
						BaseSubject: addonv1beta1.BaseSubject{
							User: user,
							Groups: []string{
								"system:open-cluster-management:cluster:" + clusterName + ":addon:gpu-platform-addon",
								"system:open-cluster-management:addon:gpu-platform-addon",
							},
						},
					},
				},
			}},
		},
	}
}
