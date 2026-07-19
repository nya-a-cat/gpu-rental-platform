#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

export HUB_CLUSTER_SIGNING_DURATION_OVERRIDE="${ADDON_LIFECYCLE_SIGNING_DURATION}"
mkdir -p "${ARTIFACT_DIR}"

on_exit() {
  local status=$?
  local evidence_status=0
  trap - EXIT
  set +e

  if [[ "${status}" -ne 0 ]]; then
    (dump_diagnostics) || true
    set +e
  fi

  bash "${SCRIPT_DIR}/collect-evidence.sh"
  bash "${SCRIPT_DIR}/verify-evidence-policy.sh"
  evidence_status=$?

  jq -n --argjson lifecycle_status "${status}" --argjson evidence_status "${evidence_status}" '{lifecycleExitCode:$lifecycle_status,evidencePolicyExitCode:$evidence_status}' >"${ARTIFACT_DIR}/ci-result.json"
  (cd "${ARTIFACT_DIR}" && find . -type f ! -name artifact-manifest.sha256 -print0 | sort -z | xargs -0 sha256sum) >"${ARTIFACT_DIR}/artifact-manifest.sha256"

  if [[ "${KEEP_CLUSTERS:-0}" != "1" ]]; then
    bash "${SCRIPT_DIR}/cleanup.sh"
  fi

  if [[ "${status}" -eq 0 && "${evidence_status}" -ne 0 ]]; then
    status="${evidence_status}"
  fi
  exit "${status}"
}
trap on_exit EXIT

bash "${SCRIPT_DIR}/validate-versions.sh"
bash "${SCRIPT_DIR}/install-tools.sh"
bash "${SCRIPT_DIR}/create-kind-clusters.sh"
bash "${SCRIPT_DIR}/bootstrap-ocm.sh"
bash "${SCRIPT_DIR}/verify-addon-lifecycle.sh"
