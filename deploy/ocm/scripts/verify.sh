#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_command base64
require_command grep
require_command jq
require_command kubectl
require_command mktemp
require_command sed

mkdir -p "${ARTIFACT_DIR}" "${TOOLS_ROOT}"

verify_managed_cluster() {
  local spoke_context="$1"
  local cluster_name="$2"
  local addon_lease_before
  local addon_uid

  kubectl --context "${HUB_CONTEXT}" wait \
    --for=condition=HubAcceptedManagedCluster \
    managedcluster/"${cluster_name}" \
    --timeout="${WAIT_TIMEOUT}"
  kubectl --context "${HUB_CONTEXT}" wait \
    --for=condition=ManagedClusterJoined \
    managedcluster/"${cluster_name}" \
    --timeout="${WAIT_TIMEOUT}"
  kubectl --context "${HUB_CONTEXT}" wait \
    --for=condition=ManagedClusterConditionAvailable \
    managedcluster/"${cluster_name}" \
    --timeout="${WAIT_TIMEOUT}"
  wait_until "approved ${cluster_name} managed-cluster CSR" cluster_csr_is_approved "${cluster_name}"
  wait_until "fresh ${cluster_name} managed-cluster Lease" managed_cluster_lease_is_fresh "${cluster_name}"

  sed "s/cluster1/${cluster_name}/g" "${DEPLOY_ROOT}/manifests/manifestwork-smoke.yaml" |
    kubectl --context "${HUB_CONTEXT}" apply --filename -
  kubectl --context "${HUB_CONTEXT}" -n "${cluster_name}" wait \
    --for=condition=Applied \
    manifestwork/gpu-platform-ocm-smoke \
    --timeout="${WAIT_TIMEOUT}"
  kubectl --context "${HUB_CONTEXT}" -n "${cluster_name}" wait \
    --for=condition=Available \
    manifestwork/gpu-platform-ocm-smoke \
    --timeout="${WAIT_TIMEOUT}"

  kubectl --context "${spoke_context}" -n default get configmap gpu-platform-ocm-smoke -o json | jq -e \
    --arg cluster "${cluster_name}" '
      .data["delivered-by"] == "manifestwork" and
      .data["managed-cluster"] == $cluster
    ' >/dev/null

  kubectl --context "${HUB_CONTEXT}" get clustermanagementaddon "${ADDON_NAME}" >/dev/null
  kubectl --context "${HUB_CONTEXT}" -n "${cluster_name}" wait \
    --for=condition=Applied \
    manifestwork/"${ADDON_WORK_NAME}" \
    --timeout="${WAIT_TIMEOUT}"
  kubectl --context "${HUB_CONTEXT}" -n "${cluster_name}" wait \
    --for=condition=Available \
    managedclusteraddon/"${ADDON_NAME}" \
    --timeout="${WAIT_TIMEOUT}"
  kubectl --context "${spoke_context}" -n "${ADDON_INSTALL_NAMESPACE}" \
    rollout status deployment/gpu-platform-addon-agent --timeout="${WAIT_TIMEOUT}"
  kubectl --context "${spoke_context}" -n "${ADDON_INSTALL_NAMESPACE}" \
    get secret "${ADDON_HUB_KUBECONFIG_SECRET}" -o json | jq -e \
    '.data.kubeconfig != null and .data.kubeconfig != ""' >/dev/null
  wait_until "approved ${cluster_name} GPU Platform Add-on CSR" addon_csr_is_approved "${cluster_name}"
  wait_until "fresh ${cluster_name} GPU Platform Add-on Lease" addon_lease_is_fresh "${spoke_context}"
  addon_lease_before="$(lease_renew_time "${spoke_context}" "${ADDON_INSTALL_NAMESPACE}" "${ADDON_NAME}")"
  wait_until "renewed ${cluster_name} GPU Platform Add-on Lease" lease_renewed_since \
    "${spoke_context}" "${ADDON_INSTALL_NAMESPACE}" "${ADDON_NAME}" "${addon_lease_before}"

  addon_uid="$(kubectl --context "${HUB_CONTEXT}" -n "${cluster_name}" get managedclusteraddon "${ADDON_NAME}" -o jsonpath='{.metadata.uid}')"
  kubectl --context "${HUB_CONTEXT}" -n "${cluster_name}" \
    get configmap gpu-platform-inventory -o json | jq -e --arg cluster "${cluster_name}" --arg addon_uid "${addon_uid}" '
      .data["inventory.json"]
        | fromjson
        | .schemaVersion == "gpu.platform.nyaacat.dev/v1alpha1" and
          .clusterName == $cluster and
          (.agentEpoch | test("^[a-f0-9]{32}$")) and
          (.sequence | type == "number" and . >= 1) and
          .fencingToken == $addon_uid and
          .fencingEnabled == true and
          (.generation | test("^[a-f0-9]{64}$")) and
          (.resources | type == "array")
    ' >/dev/null

  kubectl --context "${HUB_CONTEXT}" -n "${cluster_name}" get configmap gpu-platform-inventory -o json | jq -e --arg addon_uid "${addon_uid}" '
    any(.metadata.ownerReferences[]?;
      .apiVersion == "addon.open-cluster-management.io/v1beta1" and
      .kind == "ManagedClusterAddOn" and
      .name == "gpu-platform-addon" and
      .uid == $addon_uid and
      .controller == true
    )
  ' >/dev/null
}

materialize_addon_credentials() {
  local spoke_context="$1"
  local destination="$2"
  local key

  mkdir -p "${destination}"
  for key in kubeconfig tls.crt tls.key; do
    kubectl --context "${spoke_context}" -n "${ADDON_INSTALL_NAMESPACE}" \
      get secret "${ADDON_HUB_KUBECONFIG_SECRET}" -o json |
      jq -er --arg key "${key}" '.data[$key]' |
      base64 --decode >"${destination}/${key}"
    chmod 600 "${destination}/${key}"
  done
}

authorization_result() {
  local kubeconfig="$1"
  local hub_api_server="$2"
  local output
  shift 2

  if output="$("${KUBECTL_BIN}" --kubeconfig "${kubeconfig}" --server "${hub_api_server}" "$@" 2>&1)"; then
    printf 'true'
    return 0
  fi
  if grep -qi 'forbidden' <<<"${output}"; then
    printf 'false'
    return 0
  fi

  echo "authorization probe failed unexpectedly: ${output}" >&2
  return 1
}

assert_authorization() {
  local description="$1"
  local expected="$2"
  local actual="$3"

  if [[ "${actual}" != "${expected}" ]]; then
    echo "${description}: expected ${expected}, got ${actual}" >&2
    return 1
  fi
}

verify_cross_cluster_authorization() {
  local credential_root
  local credential_root_quoted
  local primary_credential_dir
  local primary_kubeconfig
  local secondary_credential_dir
  local secondary_kubeconfig
  local primary_inventory_manifest
  local secondary_inventory_manifest
  local primary_own_get
  local primary_own_update
  local primary_own_create
  local primary_foreign_get
  local primary_foreign_update
  local primary_foreign_create
  local secondary_own_get
  local secondary_own_update
  local secondary_own_create
  local secondary_foreign_get
  local secondary_foreign_update
  local secondary_foreign_create
  local hub_api_server

  credential_root="$(mktemp -d "${TOOLS_ROOT}/addon-authorization.XXXXXX")"
  printf -v credential_root_quoted '%q' "${credential_root}"
  trap "rm -rf -- ${credential_root_quoted}" EXIT
  primary_credential_dir="${credential_root}/primary"
  secondary_credential_dir="${credential_root}/secondary"
  materialize_addon_credentials "${SPOKE_CONTEXT}" "${primary_credential_dir}"
  materialize_addon_credentials "${SECONDARY_SPOKE_CONTEXT}" "${secondary_credential_dir}"
  primary_kubeconfig="${primary_credential_dir}/kubeconfig"
  secondary_kubeconfig="${secondary_credential_dir}/kubeconfig"
  primary_inventory_manifest="${credential_root}/primary-inventory.json"
  secondary_inventory_manifest="${credential_root}/secondary-inventory.json"
  kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get configmap gpu-platform-inventory -o json >"${primary_inventory_manifest}"
  kubectl --context "${HUB_CONTEXT}" -n "${SECONDARY_MANAGED_CLUSTER_NAME}" get configmap gpu-platform-inventory -o json >"${secondary_inventory_manifest}"

  hub_api_server="$(kubectl --context "${HUB_CONTEXT}" config view --minify -o jsonpath='{.clusters[0].cluster.server}')"

  primary_own_get="$(authorization_result "${primary_kubeconfig}" "${hub_api_server}" get configmap gpu-platform-inventory --namespace "${MANAGED_CLUSTER_NAME}")"
  primary_own_update="$(authorization_result "${primary_kubeconfig}" "${hub_api_server}" replace --filename "${primary_inventory_manifest}" --dry-run=server)"
  primary_own_create="$(authorization_result "${primary_kubeconfig}" "${hub_api_server}" create configmap gpu-platform-authorization-probe --namespace "${MANAGED_CLUSTER_NAME}" --from-literal=probe=true --dry-run=server --output name)"
  primary_foreign_get="$(authorization_result "${primary_kubeconfig}" "${hub_api_server}" get configmap gpu-platform-inventory --namespace "${SECONDARY_MANAGED_CLUSTER_NAME}")"
  primary_foreign_update="$(authorization_result "${primary_kubeconfig}" "${hub_api_server}" replace --filename "${secondary_inventory_manifest}" --dry-run=server)"
  primary_foreign_create="$(authorization_result "${primary_kubeconfig}" "${hub_api_server}" create configmap gpu-platform-authorization-probe --namespace "${SECONDARY_MANAGED_CLUSTER_NAME}" --from-literal=probe=true --dry-run=server --output name)"

  secondary_own_get="$(authorization_result "${secondary_kubeconfig}" "${hub_api_server}" get configmap gpu-platform-inventory --namespace "${SECONDARY_MANAGED_CLUSTER_NAME}")"
  secondary_own_update="$(authorization_result "${secondary_kubeconfig}" "${hub_api_server}" replace --filename "${secondary_inventory_manifest}" --dry-run=server)"
  secondary_own_create="$(authorization_result "${secondary_kubeconfig}" "${hub_api_server}" create configmap gpu-platform-authorization-probe --namespace "${SECONDARY_MANAGED_CLUSTER_NAME}" --from-literal=probe=true --dry-run=server --output name)"
  secondary_foreign_get="$(authorization_result "${secondary_kubeconfig}" "${hub_api_server}" get configmap gpu-platform-inventory --namespace "${MANAGED_CLUSTER_NAME}")"
  secondary_foreign_update="$(authorization_result "${secondary_kubeconfig}" "${hub_api_server}" replace --filename "${primary_inventory_manifest}" --dry-run=server)"
  secondary_foreign_create="$(authorization_result "${secondary_kubeconfig}" "${hub_api_server}" create configmap gpu-platform-authorization-probe --namespace "${MANAGED_CLUSTER_NAME}" --from-literal=probe=true --dry-run=server --output name)"
  assert_authorization "primary Add-on reads own inventory" true "${primary_own_get}"
  assert_authorization "primary Add-on updates own inventory" true "${primary_own_update}"
  assert_authorization "primary Add-on creates own inventory" true "${primary_own_create}"
  assert_authorization "primary Add-on reads secondary inventory" false "${primary_foreign_get}"
  assert_authorization "primary Add-on updates secondary inventory" false "${primary_foreign_update}"
  assert_authorization "primary Add-on creates secondary inventory" false "${primary_foreign_create}"
  assert_authorization "secondary Add-on reads own inventory" true "${secondary_own_get}"
  assert_authorization "secondary Add-on updates own inventory" true "${secondary_own_update}"
  assert_authorization "secondary Add-on creates own inventory" true "${secondary_own_create}"
  assert_authorization "secondary Add-on reads primary inventory" false "${secondary_foreign_get}"
  assert_authorization "secondary Add-on updates primary inventory" false "${secondary_foreign_update}"
  assert_authorization "secondary Add-on creates primary inventory" false "${secondary_foreign_create}"

  jq -n \
    --arg primary "${MANAGED_CLUSTER_NAME}" \
    --arg secondary "${SECONDARY_MANAGED_CLUSTER_NAME}" \
    --argjson primary_own_get "${primary_own_get}" \
    --argjson primary_own_update "${primary_own_update}" \
    --argjson primary_own_create "${primary_own_create}" \
    --argjson primary_foreign_get "${primary_foreign_get}" \
    --argjson primary_foreign_update "${primary_foreign_update}" \
    --argjson primary_foreign_create "${primary_foreign_create}" \
    --argjson secondary_own_get "${secondary_own_get}" \
    --argjson secondary_own_update "${secondary_own_update}" \
    --argjson secondary_own_create "${secondary_own_create}" \
    --argjson secondary_foreign_get "${secondary_foreign_get}" \
    --argjson secondary_foreign_update "${secondary_foreign_update}" \
    --argjson secondary_foreign_create "${secondary_foreign_create}" '
      {
        schemaVersion: 1,
        clusters: [$primary, $secondary],
        agents: {
          ($primary): {
            ownNamespace: {
              getInventory: $primary_own_get,
              updateInventory: $primary_own_update,
              createConfigMaps: $primary_own_create
            },
            foreignNamespace: {
              namespace: $secondary,
              getInventory: $primary_foreign_get,
              updateInventory: $primary_foreign_update,
              createConfigMaps: $primary_foreign_create
            }
          },
          ($secondary): {
            ownNamespace: {
              getInventory: $secondary_own_get,
              updateInventory: $secondary_own_update,
              createConfigMaps: $secondary_own_create
            },
            foreignNamespace: {
              namespace: $primary,
              getInventory: $secondary_foreign_get,
              updateInventory: $secondary_foreign_update,
              createConfigMaps: $secondary_foreign_create
            }
          }
        }
      }
    ' >"${ARTIFACT_DIR}/addon-cross-cluster-authorization.json"

  rm -rf -- "${credential_root}"
  trap - EXIT
}

verify_managed_cluster "${SPOKE_CONTEXT}" "${MANAGED_CLUSTER_NAME}"
if [[ "${OCM_SECONDARY_CLUSTER_ENABLED}" == "1" ]]; then
  verify_managed_cluster "${SECONDARY_SPOKE_CONTEXT}" "${SECONDARY_MANAGED_CLUSTER_NAME}"
  verify_cross_cluster_authorization
fi

echo "OCM ${OCM_VERSION}, Kubernetes ${KUBERNETES_VERSION}, ManifestWork, and GPU Platform Add-on smoke checks passed"
