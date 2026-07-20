#!/usr/bin/env bash
set -euo pipefail

ARTIFACT_DIR="${1:?artifact directory is required}"
mode="${2:-full}"
case "${mode}" in
  full | partial) ;;
  *)
    echo "evidence policy mode must be full or partial: ${mode}" >&2
    exit 2
    ;;
esac

mkdir -p "${ARTIFACT_DIR}"
violations=()

add_violation() {
  violations+=("$1")
}

required_files=(
  ci-observability.log
  collection.json
  tool-versions.txt
  audit-first.json
  audit-second.json
  audit-fixture.json
  audit-object.jsonl
  audit-validation.json
  prometheus-ready.json
  prometheus-targets.json
  prometheus-rules.json
  alertmanager-status.json
  otel-health.json
  otel-otlp.json
  otel-metrics.txt
  pods.json
  deployments.json
  baseline-assertions.json
  cleanup-summary.json
)

if [[ "${mode}" == "full" ]]; then
  for file_name in "${required_files[@]}"; do
    if [[ ! -s "${ARTIFACT_DIR}/${file_name}" ]]; then
      add_violation "missing or empty required artifact: ${file_name}"
    fi
  done
elif [[ ! -s "${ARTIFACT_DIR}/ci-observability.log" ]]; then
  add_violation "missing or empty partial-run log: ci-observability.log"
fi

while IFS= read -r file_path; do
  base_name="$(basename "${file_path}")"
  case "${base_name}" in
    *.json | *.jsonl | *.txt | *.log) ;;
    *) add_violation "artifact has a disallowed file type: ${base_name}" ;;
  esac
  if [[ "${base_name}" =~ (kubeconfig|credential|private[-_]?key|access[-_]?token|password|secret[-_]?value|env[-_]?dump) ]]; then
    add_violation "artifact filename is credential-like: ${base_name}"
  fi
done < <(find "${ARTIFACT_DIR}" -type f ! -name evidence-sha256.txt ! -name evidence-policy-report.json | sort)

while IFS= read -r json_file; do
  if ! jq empty "${json_file}" >/dev/null 2>&1; then
    add_violation "invalid JSON: $(basename "${json_file}")"
  fi
done < <(find "${ARTIFACT_DIR}" -type f -name '*.json' ! -name evidence-policy-report.json | sort)

while IFS= read -r jsonl_file; do
  if ! jq -s empty "${jsonl_file}" >/dev/null 2>&1; then
    add_violation "invalid JSON Lines: $(basename "${jsonl_file}")"
  fi
done < <(find "${ARTIFACT_DIR}" -type f -name '*.jsonl' | sort)

if grep -RIlE --exclude=evidence-policy-report.json --exclude=evidence-sha256.txt \
  -e '-----BEGIN ([A-Z ]+ )?PRIVATE KEY-----|client-key-data|client-certificate-data|certificate-authority-data|postgres(ql)?://|ci_only_observability|/home/runner|[A-Za-z]:\\Users\\|/var/lib/(docker|containerd)' \
  "${ARTIFACT_DIR}" >/dev/null 2>&1; then
  add_violation "artifact content includes a forbidden credential, runner path, or raw container-storage field"
fi

check_json() {
  local file_name="$1"
  local expression="$2"
  local description="$3"
  if [[ -s "${ARTIFACT_DIR}/${file_name}" ]] && ! jq -e "${expression}" "${ARTIFACT_DIR}/${file_name}" >/dev/null 2>&1; then
    add_violation "${description}: ${file_name}"
  fi
}

if [[ "${mode}" == "full" ]]; then
  check_json audit-validation.json '.status == "passed" and .firstStatus == "uploaded" and .secondStatus == "existing" and .rows == 1 and .sha256Matches == true and .objectLock == "GOVERNANCE" and .putCount == 1 and .authorizationV4 == true' "audit archive validation failed"
  check_json audit-fixture.json '.status == "passed" and .retentionDatePresent == true and (.metadataKeys | length) == 5' "S3 Object Lock fixture validation failed"
  check_json prometheus-targets.json '.status == "success" and (.activeTargets | length) == 2 and all(.activeTargets[]; .health == "up" and .lastError == "")' "Prometheus target validation failed"
  check_json prometheus-rules.json '([.groups[].rules[].name] | sort) == (["GPUControlPlaneMetricsUnavailable","GPUControlPlaneRecoveredPanic","GPUPlatformOTelCollectorUnavailable"] | sort) and all(.groups[].rules[]; .health == "ok")' "Prometheus alert rule validation failed"
  check_json alertmanager-status.json '.status == "passed"' "Alertmanager validation failed"
  check_json otel-health.json '.status == "passed"' "OpenTelemetry health validation failed"
  check_json otel-otlp.json '.status == "passed" and .httpCode == "200"' "OTLP receiver validation failed"
  check_json pods.json '.status == "passed" and (.pods | length) >= 6 and all(.pods[]; .phase == "Running" and .ready and .serviceAccountTokenDisabled and .runAsNonRoot and .secureContainers)' "runtime Pod security validation failed"
  check_json deployments.json '.status == "passed" and (.deployments | length) == 4 and all(.deployments[]; .ready == .desired and .available == .desired)' "runtime Deployment validation failed"
  check_json baseline-assertions.json '.status == "passed" and all(.checks[]; . == "passed")' "observability aggregate validation failed"
  check_json cleanup-summary.json '.status == "passed" and .clusterRemoved == true' "cleanup validation failed"
  if [[ -s "${ARTIFACT_DIR}/otel-metrics.txt" ]] && ! grep -Eq '^otelcol_' "${ARTIFACT_DIR}/otel-metrics.txt"; then
    add_violation "OpenTelemetry internal metrics are missing"
  fi
fi

violation_file="${ARTIFACT_DIR}/.evidence-policy-violations.jsonl"
: >"${violation_file}"
for violation in "${violations[@]}"; do
  jq -cn --arg violation "${violation}" '$violation' >>"${violation_file}"
done

checked_files="$(find "${ARTIFACT_DIR}" -type f ! -name evidence-sha256.txt ! -name evidence-policy-report.json ! -name .evidence-policy-violations.jsonl | awk 'END {print NR + 0}')"
jq -s \
  --arg generatedAt "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg mode "${mode}" \
  --argjson checkedFiles "${checked_files}" '
    {status:(if length == 0 then "passed" else "failed" end),mode:$mode,generatedAt:$generatedAt,checkedFiles:$checkedFiles,violations:.}
  ' "${violation_file}" >"${ARTIFACT_DIR}/evidence-policy-report.json"
rm -f "${violation_file}"

(
  cd "${ARTIFACT_DIR}"
  find . -type f ! -name evidence-sha256.txt -print0 | sort -z | xargs -0 sha256sum >evidence-sha256.txt
)

if [[ "${#violations[@]}" -ne 0 ]]; then
  jq -r '.violations[]' "${ARTIFACT_DIR}/evidence-policy-report.json" >&2
  exit 1
fi

echo "observability evidence policy passed: ${ARTIFACT_DIR}/evidence-policy-report.json"
