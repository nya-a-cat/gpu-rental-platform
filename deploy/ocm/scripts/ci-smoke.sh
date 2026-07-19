#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

on_exit() {
  local status=$?
  trap - EXIT
  set +e

  if [[ "${status}" -ne 0 ]]; then
    dump_diagnostics
  fi

  bash "${SCRIPT_DIR}/collect-evidence.sh" || true

  if [[ "${KEEP_CLUSTERS:-0}" != "1" ]]; then
    bash "${SCRIPT_DIR}/cleanup.sh"
  fi

  exit "${status}"
}
trap on_exit EXIT

require_command docker
if ! docker image inspect "${ADDON_IMAGE}" >/dev/null 2>&1; then
  echo "build ${ADDON_IMAGE} before running the OCM smoke suite" >&2
  exit 1
fi

bash "${SCRIPT_DIR}/validate-versions.sh"
bash "${SCRIPT_DIR}/install-tools.sh"
bash "${SCRIPT_DIR}/create-kind-clusters.sh"
bash "${SCRIPT_DIR}/bootstrap-ocm.sh"
bash "${SCRIPT_DIR}/install-addon.sh"
bash "${SCRIPT_DIR}/verify.sh"
bash "${SCRIPT_DIR}/verify-certificate-rotation.sh"
