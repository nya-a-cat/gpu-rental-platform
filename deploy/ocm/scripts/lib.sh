#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${DEPLOY_ROOT}/../.." && pwd)"

# shellcheck source=../versions.env
source "${DEPLOY_ROOT}/versions.env"

TOOLS_ROOT="${TOOLS_ROOT:-${RUNNER_TEMP:-${DEPLOY_ROOT}/.tools}/gpu-cloud-ocm}"
BIN_DIR="${TOOLS_ROOT}/bin"
CLUSTERADM_BIN="${BIN_DIR}/clusteradm"
KIND_BIN="${BIN_DIR}/kind"
KUBECTL_BIN="${BIN_DIR}/kubectl"
HELM_BIN="${BIN_DIR}/helm"
export PATH="${BIN_DIR}:${PATH}"

ARTIFACT_DIR="${ARTIFACT_DIR:-${REPO_ROOT}/artifacts/ocm}"

HUB_CLUSTER_NAME="hub"
SPOKE_CLUSTER_NAME="cluster1"
HUB_CONTEXT="kind-hub"
SPOKE_CONTEXT="kind-cluster1"
MANAGED_CLUSTER_NAME="cluster1"

ADDON_NAME="gpu-platform-addon"
ADDON_IMAGE="${ADDON_IMAGE:-gpu-platform-addon:ci}"
ADDON_WORK_NAME="addon-gpu-platform-addon-deploy-0"
ADDON_INSTALL_NAMESPACE="open-cluster-management-agent-addon"
ADDON_MANAGER_NAMESPACE="gpu-platform-system"
ADDON_HELM_RELEASE="gpu-platform-addon"

WAIT_TIMEOUT="${WAIT_TIMEOUT:-300s}"
RETRY_ATTEMPTS="${RETRY_ATTEMPTS:-90}"
RETRY_INTERVAL_SECONDS="${RETRY_INTERVAL_SECONDS:-5}"

require_command() {
  local command_name="$1"
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "required command is unavailable: ${command_name}" >&2
    return 1
  fi
}

wait_until() {
  local description="$1"
  shift

  local attempt
  for attempt in $(seq 1 "${RETRY_ATTEMPTS}"); do
    if "$@"; then
      echo "ready: ${description}"
      return 0
    fi
    sleep "${RETRY_INTERVAL_SECONDS}"
  done

  echo "timed out waiting for: ${description}" >&2
  return 1
}

cluster_exists() {
  local cluster_name="$1"
  "${KIND_BIN}" get clusters 2>/dev/null | grep -Fxq "${cluster_name}"
}

managed_cluster_lease_is_fresh() {
  local renew_time
  renew_time="$(kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" \
    get lease managed-cluster-lease -o jsonpath='{.spec.renewTime}' 2>/dev/null || true)"
  [[ -n "${renew_time}" ]]
}

addon_lease_is_fresh() {
  local renew_time
  renew_time="$(kubectl --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" \
    get lease "${ADDON_NAME}" -o jsonpath='{.spec.renewTime}' 2>/dev/null || true)"
  [[ -n "${renew_time}" ]]
}

lease_renew_time() {
  local context="$1"
  local namespace="$2"
  local name="$3"

  kubectl --context "${context}" -n "${namespace}" \
    get lease "${name}" -o jsonpath='{.spec.renewTime}' 2>/dev/null || true
}

lease_renewed_since() {
  local context="$1"
  local namespace="$2"
  local name="$3"
  local previous="$4"
  local current

  current="$(lease_renew_time "${context}" "${namespace}" "${name}")"
  [[ -n "${current}" && "${current}" != "${previous}" ]]
}

cluster_csr_is_approved() {
  kubectl --context "${HUB_CONTEXT}" get certificatesigningrequests -o json | jq -e \
    --arg cluster "${MANAGED_CLUSTER_NAME}" '
      any(.items[];
        .metadata.labels["open-cluster-management.io/cluster-name"] == $cluster and
        any(.status.conditions[]?; .type == "Approved" and .status == "True") and
        ((.status.certificate // "") | length > 0)
      )
    ' >/dev/null
}

addon_csr_is_approved() {
  kubectl --context "${HUB_CONTEXT}" get certificatesigningrequests -o json | jq -e \
    --arg cluster "${MANAGED_CLUSTER_NAME}" \
    --arg addon "${ADDON_NAME}" '
      any(.items[];
        .metadata.labels["open-cluster-management.io/cluster-name"] == $cluster and
        .metadata.labels["open-cluster-management.io/addon-name"] == $addon and
        any(.status.conditions[]?; .type == "Approved" and .status == "True") and
        ((.status.certificate // "") | length > 0)
      )
    ' >/dev/null
}

dump_diagnostics() {
  set +e
  echo "===== hub cluster diagnostics =====" >&2
  kubectl --context "${HUB_CONTEXT}" get nodes -o wide >&2
  kubectl --context "${HUB_CONTEXT}" get pods -A -o wide >&2
  kubectl --context "${HUB_CONTEXT}" get managedclusters -o wide >&2
  kubectl --context "${HUB_CONTEXT}" get certificatesigningrequests -o wide >&2
  kubectl --context "${HUB_CONTEXT}" get clustermanagementaddons -o yaml >&2
  kubectl --context "${HUB_CONTEXT}" get managedclusteraddons -A -o yaml >&2
  kubectl --context "${HUB_CONTEXT}" get manifestworks -A -o yaml >&2
  kubectl --context "${HUB_CONTEXT}" get leases -A -o wide >&2
  kubectl --context "${HUB_CONTEXT}" get events -A --sort-by=.lastTimestamp >&2
  kubectl --context "${HUB_CONTEXT}" -n "${ADDON_MANAGER_NAMESPACE}" \
    logs deployment/"${ADDON_HELM_RELEASE}" --all-containers --tail=250 >&2

  echo "===== managed cluster diagnostics =====" >&2
  kubectl --context "${SPOKE_CONTEXT}" get nodes -o wide >&2
  kubectl --context "${SPOKE_CONTEXT}" get pods -A -o wide >&2
  kubectl --context "${SPOKE_CONTEXT}" get events -A --sort-by=.lastTimestamp >&2
  kubectl --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" \
    logs deployment/gpu-platform-addon-agent --all-containers --tail=250 >&2
  set -e
}
