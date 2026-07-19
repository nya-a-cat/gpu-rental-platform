#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_command jq
require_command kubectl

kubectl --context "${HUB_CONTEXT}" wait \
  --for=condition=HubAcceptedManagedCluster \
  managedcluster/"${MANAGED_CLUSTER_NAME}" \
  --timeout="${WAIT_TIMEOUT}"
kubectl --context "${HUB_CONTEXT}" wait \
  --for=condition=ManagedClusterJoined \
  managedcluster/"${MANAGED_CLUSTER_NAME}" \
  --timeout="${WAIT_TIMEOUT}"
kubectl --context "${HUB_CONTEXT}" wait \
  --for=condition=ManagedClusterConditionAvailable \
  managedcluster/"${MANAGED_CLUSTER_NAME}" \
  --timeout="${WAIT_TIMEOUT}"
wait_until "approved managed-cluster CSR" cluster_csr_is_approved
wait_until "fresh managed-cluster Lease" managed_cluster_lease_is_fresh

kubectl --context "${HUB_CONTEXT}" apply \
  --filename "${DEPLOY_ROOT}/manifests/manifestwork-smoke.yaml"
kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" wait \
  --for=condition=Applied \
  manifestwork/gpu-platform-ocm-smoke \
  --timeout="${WAIT_TIMEOUT}"
kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" wait \
  --for=condition=Available \
  manifestwork/gpu-platform-ocm-smoke \
  --timeout="${WAIT_TIMEOUT}"

kubectl --context "${SPOKE_CONTEXT}" -n default get configmap gpu-platform-ocm-smoke -o json | jq -e '
  .data["delivered-by"] == "manifestwork" and
  .data["managed-cluster"] == "cluster1"
' >/dev/null

kubectl --context "${HUB_CONTEXT}" get clustermanagementaddon "${ADDON_NAME}" >/dev/null
kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" wait \
  --for=condition=Applied \
  manifestwork/"${ADDON_WORK_NAME}" \
  --timeout="${WAIT_TIMEOUT}"
kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" wait \
  --for=condition=Available \
  managedclusteraddon/"${ADDON_NAME}" \
  --timeout="${WAIT_TIMEOUT}"
kubectl --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" \
  rollout status deployment/gpu-platform-addon-agent --timeout="${WAIT_TIMEOUT}"
kubectl --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" \
  get secret gpu-platform-addon-hub-kubeconfig -o json | jq -e \
  '.data.kubeconfig != null and .data.kubeconfig != ""' >/dev/null
wait_until "approved GPU Platform Add-on CSR" addon_csr_is_approved
wait_until "fresh GPU Platform Add-on Lease" addon_lease_is_fresh
addon_lease_before="$(lease_renew_time "${SPOKE_CONTEXT}" "${ADDON_INSTALL_NAMESPACE}" "${ADDON_NAME}")"
wait_until "renewed GPU Platform Add-on Lease" lease_renewed_since \
  "${SPOKE_CONTEXT}" "${ADDON_INSTALL_NAMESPACE}" "${ADDON_NAME}" "${addon_lease_before}"

kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" \
  get configmap gpu-platform-inventory -o json | jq -e \
  '.data["inventory.json"]
    | fromjson
    | .schemaVersion == "gpu.platform.nyaacat.dev/v1alpha1" and
      .clusterName == "cluster1" and
      (.generation | test("^[a-f0-9]{64}$")) and
      (.resources | type == "array")' >/dev/null

addon_uid="$(kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get managedclusteraddon "${ADDON_NAME}" -o jsonpath='{.metadata.uid}')"
kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get configmap gpu-platform-inventory -o json | jq -e --arg addon_uid "${addon_uid}" '
  any(.metadata.ownerReferences[]?;
    .apiVersion == "addon.open-cluster-management.io/v1beta1" and
    .kind == "ManagedClusterAddOn" and
    .name == "gpu-platform-addon" and
    .uid == $addon_uid and
    .controller == true
  )
' >/dev/null

echo "OCM ${OCM_VERSION}, Kubernetes ${KUBERNETES_VERSION}, ManifestWork, and GPU Platform Add-on smoke checks passed"
