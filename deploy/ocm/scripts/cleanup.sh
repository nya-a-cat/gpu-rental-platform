#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

if [[ ! -x "${KIND_BIN}" ]]; then
  exit 0
fi

for cluster_name in "${SPOKE_CLUSTER_NAME}" "${HUB_CLUSTER_NAME}"; do
  if cluster_exists "${cluster_name}"; then
    "${KIND_BIN}" delete cluster --name "${cluster_name}"
  fi
done
