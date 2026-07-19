#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_command jq

mkdir -p "${ARTIFACT_DIR}"

capture() {
  local file_name="$1"
  shift

  "$@" >"${ARTIFACT_DIR}/${file_name}" 2>&1 || true
}

capture_secret_metadata() {
  local file_name="$1"
  local namespace="$2"
  local secret_name="$3"

  kubectl --context "${SPOKE_CONTEXT}" -n "${namespace}" \
    get secret "${secret_name}" -o json | jq '
      {
        apiVersion,
        kind,
        metadata: {
          name: .metadata.name,
          namespace: .metadata.namespace,
          uid: .metadata.uid,
          resourceVersion: .metadata.resourceVersion,
          creationTimestamp: .metadata.creationTimestamp,
          labels: .metadata.labels
        },
        type,
        dataKeys: ((.data // {}) | keys)
      }
    ' >"${ARTIFACT_DIR}/${file_name}" 2>&1 || true
}

kubectl --context "${HUB_CONTEXT}" -n kube-system \
  get pods -l component=kube-controller-manager -o json | jq '
    {
      items: [
        .items[]
        | {
            metadata: {
              name: .metadata.name,
              uid: .metadata.uid
            },
            containers: [
              .spec.containers[]
              | {
                  name,
                  image,
                  command,
                  args
                }
            ]
          }
      ]
    }
  ' >"${ARTIFACT_DIR}/hub-kube-controller-manager.json" 2>&1 || true

{
  echo "collected_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "ocm_version=${OCM_VERSION}"
  echo "kubernetes_version=${KUBERNETES_VERSION}"
  "${KIND_BIN}" version
  "${CLUSTERADM_BIN}" version
  "${KUBECTL_BIN}" version --client --output=yaml
  "${HELM_BIN}" version --short
  docker version
  docker image inspect "${ADDON_IMAGE}"
} >"${ARTIFACT_DIR}/tool-and-image-versions.txt" 2>&1 || true

capture hub-nodes.yaml kubectl --context "${HUB_CONTEXT}" get nodes -o yaml
capture hub-pods.txt kubectl --context "${HUB_CONTEXT}" get pods -A -o wide
capture managed-cluster.yaml kubectl --context "${HUB_CONTEXT}" get managedcluster "${MANAGED_CLUSTER_NAME}" -o yaml
kubectl --context "${HUB_CONTEXT}" get certificatesigningrequests -o json | jq '
  {
    apiVersion,
    kind,
    items: [
      .items[]
      | {
          metadata: {
            name: .metadata.name,
            uid: .metadata.uid,
            resourceVersion: .metadata.resourceVersion,
            creationTimestamp: .metadata.creationTimestamp,
            labels: .metadata.labels
          },
          spec: {
            signerName: .spec.signerName,
            expirationSeconds: .spec.expirationSeconds,
            usages: .spec.usages,
            username: .spec.username,
            groups: .spec.groups
          },
          status: {
            conditions: .status.conditions
          }
        }
    ]
  }
' >"${ARTIFACT_DIR}/certificate-signing-requests.json" 2>&1 || true
capture cluster-management-addon.yaml kubectl --context "${HUB_CONTEXT}" get clustermanagementaddon "${ADDON_NAME}" -o yaml
capture managed-cluster-addon.yaml kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get managedclusteraddon "${ADDON_NAME}" -o yaml
capture manifestworks.yaml kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get manifestworks -o yaml
capture hub-leases.yaml kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get leases -o yaml
capture inventory-configmap.yaml kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get configmap gpu-platform-inventory -o yaml
capture spoke-nodes.yaml kubectl --context "${SPOKE_CONTEXT}" get nodes -o yaml
capture spoke-pods.txt kubectl --context "${SPOKE_CONTEXT}" get pods -A -o wide
capture addon-deployment.yaml kubectl --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" get deployment gpu-platform-addon-agent -o yaml
capture addon-lease.yaml kubectl --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" get lease "${ADDON_NAME}" -o yaml
capture_secret_metadata managed-cluster-hub-secret-metadata.json \
  "${MANAGED_CLUSTER_AGENT_NAMESPACE}" "${MANAGED_CLUSTER_HUB_KUBECONFIG_SECRET}"
capture_secret_metadata addon-hub-secret-metadata.json \
  "${ADDON_INSTALL_NAMESPACE}" "${ADDON_HUB_KUBECONFIG_SECRET}"
capture addon-manager.log kubectl --context "${HUB_CONTEXT}" -n "${ADDON_MANAGER_NAMESPACE}" logs deployment/"${ADDON_HELM_RELEASE}" --all-containers --tail=500
capture addon-agent.log kubectl --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" logs deployment/gpu-platform-addon-agent --all-containers --tail=500
capture manifestwork-smoke-configmap.yaml kubectl --context "${SPOKE_CONTEXT}" -n default get configmap gpu-platform-ocm-smoke -o yaml
