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

init_log="${TOOLS_ROOT}/clusteradm-init.log"
trap 'rm -f "${init_log}"' EXIT
if ! "${CLUSTERADM_BIN}" init \
  --context "${HUB_CONTEXT}" \
  --bundle-version "${OCM_VERSION}" \
  --wait >"${init_log}" 2>&1; then
  sed -E 's/(--hub-token[ =])[^[:space:]]+/\1***MASKED***/g' "${init_log}" >&2
  exit 1
fi

hub_info="$("${CLUSTERADM_BIN}" get token --context "${HUB_CONTEXT}" --output json)"
rm -f "${init_log}"
trap - EXIT
hub_token="$(jq -er '."hub-token"' <<<"${hub_info}")"
hub_apiserver="$(jq -er '."hub-apiserver"' <<<"${hub_info}")"

join_log="${TOOLS_ROOT}/clusteradm-join.log"
trap 'rm -f "${join_log}"' EXIT
if ! "${CLUSTERADM_BIN}" join \
  --context "${SPOKE_CONTEXT}" \
  --bundle-version "${OCM_VERSION}" \
  --hub-token "${hub_token}" \
  --hub-apiserver "${hub_apiserver}" \
  --cluster-name "${MANAGED_CLUSTER_NAME}" \
  --force-internal-endpoint-lookup >"${join_log}" 2>&1; then
  join_output="$(<"${join_log}")"
  printf '%s\n' "${join_output//${hub_token}/***MASKED***}" >&2
  exit 1
fi
rm -f "${join_log}"
trap - EXIT

"${CLUSTERADM_BIN}" accept \
  --context "${HUB_CONTEXT}" \
  --clusters "${MANAGED_CLUSTER_NAME}" \
  --wait

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

kubectl --context "${HUB_CONTEXT}" patch managedcluster "${MANAGED_CLUSTER_NAME}" \
  --type merge \
  --patch '{"spec":{"leaseDurationSeconds":5}}'
cluster_lease_before="$(lease_renew_time "${HUB_CONTEXT}" "${MANAGED_CLUSTER_NAME}" managed-cluster-lease)"
wait_until "renewed managed-cluster Lease" lease_renewed_since \
  "${HUB_CONTEXT}" "${MANAGED_CLUSTER_NAME}" managed-cluster-lease "${cluster_lease_before}"
