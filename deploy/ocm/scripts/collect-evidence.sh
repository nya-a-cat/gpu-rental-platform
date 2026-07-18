#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

mkdir -p "${ARTIFACT_DIR}"

capture() {
  local file_name="$1"
  shift

  "$@" >"${ARTIFACT_DIR}/${file_name}" 2>&1 || true
}

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
capture certificate-signing-requests.yaml kubectl --context "${HUB_CONTEXT}" get certificatesigningrequests -o yaml
capture cluster-management-addon.yaml kubectl --context "${HUB_CONTEXT}" get clustermanagementaddon "${ADDON_NAME}" -o yaml
capture managed-cluster-addon.yaml kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get managedclusteraddon "${ADDON_NAME}" -o yaml
capture manifestworks.yaml kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get manifestworks -o yaml
capture hub-leases.yaml kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get leases -o yaml
capture inventory-configmap.yaml kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get configmap gpu-platform-inventory -o yaml
capture spoke-nodes.yaml kubectl --context "${SPOKE_CONTEXT}" get nodes -o yaml
capture spoke-pods.txt kubectl --context "${SPOKE_CONTEXT}" get pods -A -o wide
capture addon-deployment.yaml kubectl --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" get deployment gpu-platform-addon-agent -o yaml
capture addon-lease.yaml kubectl --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" get lease "${ADDON_NAME}" -o yaml
capture addon-manager.log kubectl --context "${HUB_CONTEXT}" -n "${ADDON_MANAGER_NAMESPACE}" logs deployment/"${ADDON_HELM_RELEASE}" --all-containers --tail=500
capture addon-agent.log kubectl --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" logs deployment/gpu-platform-addon-agent --all-containers --tail=500
capture manifestwork-smoke-configmap.yaml kubectl --context "${SPOKE_CONTEXT}" -n default get configmap gpu-platform-ocm-smoke -o yaml
