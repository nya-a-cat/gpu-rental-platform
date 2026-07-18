#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_command docker
require_command kubectl

if [[ ! -x "${KIND_BIN}" ]]; then
  echo "kind ${KIND_VERSION} is unavailable; run install-tools.sh first" >&2
  exit 1
fi

create_cluster() {
  local name="$1"
  local config="$2"
  local context="$3"

  if cluster_exists "${name}"; then
    echo "reusing kind cluster: ${name}"
  else
    "${KIND_BIN}" create cluster \
      --name "${name}" \
      --config "${config}" \
      --image "${KIND_NODE_IMAGE}" \
      --wait "${WAIT_TIMEOUT}"
  fi

  kubectl --context "${context}" wait --for=condition=Ready nodes --all --timeout="${WAIT_TIMEOUT}"
  kubectl --context "${context}" get nodes -o json | jq -e \
    --arg version "${KUBERNETES_VERSION}" \
    'all(.items[]; .status.nodeInfo.kubeletVersion == $version)' >/dev/null
}

require_command jq
create_cluster "${HUB_CLUSTER_NAME}" "${DEPLOY_ROOT}/kind/hub.yaml" "${HUB_CONTEXT}"
create_cluster "${SPOKE_CLUSTER_NAME}" "${DEPLOY_ROOT}/kind/cluster1.yaml" "${SPOKE_CONTEXT}"
