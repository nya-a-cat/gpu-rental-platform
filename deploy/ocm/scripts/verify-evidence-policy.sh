#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_command basename
require_command find
require_command grep
require_command jq
require_command sha256sum
require_command sort
require_command tr
require_command xargs

mkdir -p "${ARTIFACT_DIR}"
violations=()

record_violation() {
  violations+=("$1")
}

required_files=(
  tool-versions.txt
  addon-images.json
  hub-nodes.json
  spoke-nodes.json
  certificate-signing-requests.json
  managed-cluster-hub-secret-metadata.json
  addon-hub-secret-metadata.json
)
for file_name in "${required_files[@]}"; do
  if [[ ! -s "${ARTIFACT_DIR}/${file_name}" ]]; then
    record_violation "required evidence is missing: ${file_name}"
  fi
done

required_json_files=(
  addon-images.json
  hub-nodes.json
  spoke-nodes.json
  certificate-signing-requests.json
  managed-cluster-hub-secret-metadata.json
  addon-hub-secret-metadata.json
)
for file_name in "${required_json_files[@]}"; do
  if [[ -s "${ARTIFACT_DIR}/${file_name}" ]] && ! jq -e 'if type == "object" then (has("captureStatus") | not) else true end' "${ARTIFACT_DIR}/${file_name}" >/dev/null 2>&1; then
    record_violation "required evidence capture is unavailable: ${file_name}"
  fi
done

if [[ -s "${ARTIFACT_DIR}/addon-images.json" ]] && ! jq -e 'type == "array" and length > 0' "${ARTIFACT_DIR}/addon-images.json" >/dev/null; then
  record_violation "add-on image evidence is empty"
fi

while IFS= read -r -d '' artifact_file; do
  lower_name="$(basename "${artifact_file}" | tr '[:upper:]' '[:lower:]')"
  case "${lower_name}" in
    *kubeconfig*|*.key|*.pem|*private-key*)
      record_violation "credential-like artifact filename detected: ${lower_name}"
      ;;
  esac
done < <(find "${ARTIFACT_DIR}" -type f -print0)

if grep -R -I -q -E -e "-----BEGIN ([A-Z ]+ )?PRIVATE KEY-----|client-key-data|client-certificate-data|certificate-authority-data" "${ARTIFACT_DIR}"; then
  record_violation "credential material marker detected"
fi

if grep -R -I -q -E '"(GraphDriver|RootFS)"[[:space:]]*:|/home/runner/work/|/var/lib/docker/(overlay|containers)' "${ARTIFACT_DIR}"; then
  record_violation "container runtime internals or runner path detected"
fi

for node_file in "${ARTIFACT_DIR}/hub-nodes.json" "${ARTIFACT_DIR}/spoke-nodes.json"; do
  if [[ -s "${node_file}" ]] && grep -q -E '"(addresses|annotations|providerID|bootID|machineID|systemUUID)"[[:space:]]*:' "${node_file}"; then
    record_violation "node identity or address field detected in $(basename "${node_file}")"
  fi
done

csr_file="${ARTIFACT_DIR}/certificate-signing-requests.json"
if [[ -s "${csr_file}" ]] && ! jq -e '(.items // []) | all(.[]; ((.spec // {}) | has("request") | not) and ((.status // {}) | has("certificate") | not))' "${csr_file}" >/dev/null; then
  record_violation "CSR request or issued certificate body detected"
fi

while IFS= read -r -d '' secret_file; do
  if ! jq -e '(has("data") | not) and (has("stringData") | not)' "${secret_file}" >/dev/null; then
    record_violation "Secret data field detected in $(basename "${secret_file}")"
  fi
done < <(find "${ARTIFACT_DIR}" -type f -name '*secret-metadata.json' -print0)

while IFS= read -r -d '' json_file; do
  if ! jq empty "${json_file}" >/dev/null 2>&1; then
    record_violation "invalid JSON evidence: $(basename "${json_file}")"
  fi
done < <(find "${ARTIFACT_DIR}" -type f -name '*.json' ! -name 'evidence-policy-report.json' -print0)

if [[ -d "${ARTIFACT_DIR}/lifecycle" ]]; then
  if ! jq -e '.status == "passed"' "${ARTIFACT_DIR}/lifecycle/assertions.json" >/dev/null 2>&1; then
    record_violation "lifecycle assertions did not reach passed state"
  fi
  if ! jq -e '.current.sourceTree != .nMinusOne.sourceTree and (.images | length) == 2' "${ARTIFACT_DIR}/image-provenance.json" >/dev/null 2>&1; then
    record_violation "lifecycle image provenance is incomplete"
  fi
fi

status=passed
if (( ${#violations[@]} > 0 )); then
  status=failed
fi

printf '%s\n' "${violations[@]:-}" | jq -R -s --arg status "${status}" '{status:$status,violations:(split("\n")|map(select(length>0)))}' >"${ARTIFACT_DIR}/evidence-policy-report.json"
(cd "${ARTIFACT_DIR}" && find . -type f ! -name artifact-manifest.sha256 -print0 | sort -z | xargs -0 sha256sum) >"${ARTIFACT_DIR}/artifact-manifest.sha256"

if [[ "${status}" != "passed" ]]; then
  printf 'evidence policy violation: %s\n' "${violations[@]}" >&2
  exit 1
fi

echo "OCM evidence policy checks passed"
