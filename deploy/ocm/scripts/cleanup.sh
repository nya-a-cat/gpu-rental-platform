#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

if [[ ! -x "${KIND_BIN}" ]]; then
  exit 0
fi

cluster_names=("${SPOKE_CLUSTER_NAME}" "${HUB_CLUSTER_NAME}")
if [[ "${OCM_SECONDARY_CLUSTER_ENABLED}" == "1" ]]; then
  cluster_names=("${SECONDARY_SPOKE_CLUSTER_NAME}" "${cluster_names[@]}")
fi

for cluster_name in "${cluster_names[@]}"; do
  if cluster_exists "${cluster_name}"; then
    "${KIND_BIN}" delete cluster --name "${cluster_name}"
  fi
done
