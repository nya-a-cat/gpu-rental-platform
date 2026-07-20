#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_command jq
require_command kubectl

if [[ ! -x "${CLUSTERADM_BIN}" ]]; then
  echo "clusteradm ${CLUSTERADM_VERSION} is unavailable; run install-tools.sh first" >&2
  exit 1
fi

require_command sed

sensitive_logs=()
cleanup_sensitive_logs() {
  if (( ${#sensitive_logs[@]} > 0 )); then
    rm -f -- "${sensitive_logs[@]}"
  fi
}
trap cleanup_sensitive_logs EXIT

init_log="${TOOLS_ROOT}/clusteradm-init.log"
sensitive_logs+=("${init_log}")
if ! "${CLUSTERADM_BIN}" init \
  --context "${HUB_CONTEXT}" \
  --bundle-version "${OCM_VERSION}" \
  --wait >"${init_log}" 2>&1; then
  sed -E 's/(--hub-token[ =])[^[:space:]]+/\1***MASKED***/g' "${init_log}" >&2
  exit 1
fi

hub_info="$("${CLUSTERADM_BIN}" get token --context "${HUB_CONTEXT}" --output json)"
rm -f "${init_log}"
hub_token="$(jq -er '."hub-token"' <<<"${hub_info}")"
hub_apiserver="$(jq -er '."hub-apiserver"' <<<"${hub_info}")"

join_and_accept_cluster() {
  local spoke_context="$1"
  local cluster_name="$2"
  local join_log="${TOOLS_ROOT}/clusteradm-join-${cluster_name}.log"
  local join_output
  local cluster_lease_before

  sensitive_logs+=("${join_log}")
  if ! "${CLUSTERADM_BIN}" join \
    --context "${spoke_context}" \
    --bundle-version "${OCM_VERSION}" \
    --hub-token "${hub_token}" \
    --hub-apiserver "${hub_apiserver}" \
    --cluster-name "${cluster_name}" \
    --force-internal-endpoint-lookup >"${join_log}" 2>&1; then
    join_output="$(<"${join_log}")"
    printf '%s\n' "${join_output//${hub_token}/***MASKED***}" >&2
    exit 1
  fi
  rm -f "${join_log}"

  "${CLUSTERADM_BIN}" accept \
    --context "${HUB_CONTEXT}" \
    --clusters "${cluster_name}" \
    --wait

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

  kubectl --context "${HUB_CONTEXT}" patch managedcluster "${cluster_name}" \
    --type merge \
    --patch '{"spec":{"leaseDurationSeconds":5}}'
  cluster_lease_before="$(lease_renew_time "${HUB_CONTEXT}" "${cluster_name}" managed-cluster-lease)"
  wait_until "renewed ${cluster_name} managed-cluster Lease" lease_renewed_since \
    "${HUB_CONTEXT}" "${cluster_name}" managed-cluster-lease "${cluster_lease_before}"
}

join_and_accept_cluster "${SPOKE_CONTEXT}" "${MANAGED_CLUSTER_NAME}"
if [[ "${OCM_SECONDARY_CLUSTER_ENABLED}" == "1" ]]; then
  join_and_accept_cluster "${SECONDARY_SPOKE_CONTEXT}" "${SECONDARY_MANAGED_CLUSTER_NAME}"
fi
