#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

if [[ "$#" -gt 0 ]]; then
  ARTIFACT_DIR="$1"
fi
mode="${2:-full}"
case "${mode}" in
  full | partial) ;;
  *)
    echo "evidence policy mode must be full or partial: ${mode}" >&2
    exit 2
    ;;
esac

ensure_artifact_dir

require_command find
require_command grep
require_command jq
require_command sha256sum
require_command sort
require_command xargs

violations=()

add_violation() {
  violations+=("$1")
}

required_files=(
  ci-ha.log
  collection.json
  tool-versions.txt
  nodes.json
  deployment.json
  service.json
  service-account.json
  pdb.json
  pods.json
  postgres.json
  database-secret.json
  bad-database-secret.json
  migration-job.json
  endpoint-slices.json
  helm-release.json
  events.json
  control-plane.log
  migration-job.log
  postgres.log
  baseline-image.json
  image.json
  migration-validation.json
  pdb-validation.json
  security-validation.json
  baseline-pods.json
  baseline-state.json
  shared-persistence-baseline.json
  post-secret-rotation-pods.json
  post-secret-rotation-state.json
  secret-rotation-validation.json
  shared-persistence-secret-rotated.json
  bad-upgrade-probe.jsonl
  bad-upgrade-probe-summary.json
  bad-migration-job.json
  post-bad-upgrade-state.json
  bad-upgrade-validation.json
  rolling-upgrade-probe.jsonl
  rolling-upgrade-probe-summary.json
  rolling-upgrade-endpoint-slice-monitor.jsonl
  rolling-upgrade-endpoint-slice-summary.json
  post-upgrade-state.json
  post-upgrade-pods.json
  rolling-upgrade-validation.json
  shared-persistence-upgraded.json
  shared-persistence.json
  pre-delete-pods.json
  pod-delete-probe.jsonl
  pod-delete-probe-summary.json
  endpoint-slice-monitor.jsonl
  endpoint-slice-summary.json
  post-delete-pods.json
  pod-delete-validation.json
  post-uninstall-validation.json
  cleanup-summary.json
  ha-assertions.json
)

if [[ "${mode}" == "full" ]]; then
  for file_name in "${required_files[@]}"; do
    if [[ ! -s "${ARTIFACT_DIR}/${file_name}" ]]; then
      add_violation "missing or empty required artifact: ${file_name}"
    fi
  done
elif [[ ! -s "${ARTIFACT_DIR}/ci-ha.log" ]]; then
  add_violation "missing or empty partial-run log: ci-ha.log"
fi

while IFS= read -r file_path; do
  base_name="$(basename "${file_path}")"
  case "${base_name}" in
    *.json|*.jsonl|*.txt|*.log)
      ;;
    *)
      add_violation "artifact has a disallowed file type: ${base_name}"
      ;;
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
  -e '-----BEGIN ([A-Z ]+ )?PRIVATE KEY-----|client-key-data|client-certificate-data|certificate-authority-data|postgres://|ci-only-ha-password|DATABASE_URL=|"GraphDriver"|"RootFS"|/home/runner|[A-Za-z]:\\Users\\|/var/lib/(docker|containerd)' \
  "${ARTIFACT_DIR}" >/dev/null 2>&1; then
  add_violation "artifact content includes a forbidden credential, runner path, or raw container-storage field"
fi

if [[ -f "${ARTIFACT_DIR}/nodes.json" ]] && jq -e '
  .. | objects | select(has("addresses") or has("providerID") or has("machineID") or has("systemUUID") or has("bootID") or has("annotations"))
' "${ARTIFACT_DIR}/nodes.json" >/dev/null 2>&1; then
  add_violation "nodes.json includes forbidden node identity or address fields"
fi

while IFS= read -r json_file; do
  if jq -e '
    .. | objects | select(.kind? == "Secret" and (has("data") or has("stringData")))
  ' "${json_file}" >/dev/null 2>&1; then
    add_violation "Secret payload found in $(basename "${json_file}")"
  fi
done < <(find "${ARTIFACT_DIR}" -type f -name '*.json' ! -name evidence-policy-report.json | sort)

check_json() {
  local file_name="$1"
  local expression="$2"
  local description="$3"
  if [[ -s "${ARTIFACT_DIR}/${file_name}" ]] && ! jq -e "${expression}" "${ARTIFACT_DIR}/${file_name}" >/dev/null 2>&1; then
    add_violation "${description}: ${file_name}"
  fi
}

if [[ "${mode}" == "full" ]]; then
  check_json ha-assertions.json '.status == "passed" and (.checks | has("security") and has("secretRotation") and has("rollingEndpoints") and has("podFailure")) and all(.checks[]; . == "passed")' "HA assertion aggregate failed"
  check_json cleanup-summary.json '.status == "passed" and .namespaceRemoved == true and .clusterRemoved == true' "cleanup assertion failed"
  check_json migration-validation.json '.status == "passed" and .version == "000001_phase0_foundation" and (.checksum | test("^[0-9a-f]{64}$")) and (.requiredTables | sort) == (["audit_events","audit_events_default","idempotency_records","operations","outbox_events"] | sort)' "migration validation failed"
  check_json pdb-validation.json '.status == "passed" and .minAvailable == 2 and .currentHealthy == 3 and .desiredHealthy == 2 and .disruptionsAllowed >= 1' "PDB validation failed"
  check_json security-validation.json '.status == "passed" and all(.deployment[]; . == true) and .pods.podCount == 3 and .pods.allReady == true and .pods.automountServiceAccountToken == true and .pods.podSecurityContext == true and .pods.containerSecurityContext == true and .pods.noServiceAccountTokenProjectedVolume == true and all(.migrationJob[]; . == true) and .serviceAccount.automountServiceAccountToken == true' "runtime security validation failed"
  check_json secret-rotation-validation.json '.status == "passed" and .secretObjectRetained == true and .secretRevision == "ha-rotated" and .podsReplaced == true and .readyReplicas == 3 and .serviceVersion == "ha-baseline" and .baselineImageSelected == true' "database Secret rotation validation failed"
  check_json bad-upgrade-probe-summary.json '.status == "passed" and .total >= 20 and .failures == 0 and .versions == ["ha-baseline"]' "failed-upgrade availability probe failed"
  check_json bad-upgrade-validation.json '.status == "passed" and .helmUpgradeFailed == true and .deploymentUnchanged == true and .replicaSetsUnchanged == true and .podsUnchanged == true and .versionUnchanged == true and .probeFailures == 0' "failed migration invariance check failed"
  check_json bad-migration-job.json '.backoffLimit == 0 and .activeDeadlineSeconds == 60 and any(.status.conditions[]; .type == "Failed" and .status == "True") and .databaseSecret.name == "gpu-control-plane-database-bad" and .databaseSecret.key == "DATABASE_URL"' "failed migration Job evidence failed"
  check_json rolling-upgrade-probe-summary.json '.status == "passed" and .total >= 20 and .failures == 0 and (.versions | index("ha-upgraded")) != null and (.unexpectedVersions | length) == 0' "rolling-upgrade availability probe failed"
  check_json rolling-upgrade-endpoint-slice-summary.json '.status == "passed" and .total >= 20 and .queryFailures == 0 and .requiredMinimumReady == 3 and .minReady >= 3 and .finalReady == 3' "rolling-upgrade EndpointSlice validation failed"
  check_json rolling-upgrade-validation.json '.status == "passed" and .strategyValid == true and .podsReplaced == true and .baselineImage == "gpu-control-plane:ci-baseline" and .candidateImage == "gpu-control-plane:ci-candidate" and .baselineImageSelected == true and .candidateImageSelected == true and .imageIDsChanged == true and .previousReplicaSetsScaledToZero == true and .allNonCurrentReplicaSetsScaledToZero == true and .readyReplicaSetCount == 1 and .readyReplicas == 3 and .finalVersion == "ha-upgraded" and .probeFailures == 0 and .minReadyEndpoints >= 3 and .finalReadyEndpoints == 3' "rolling-upgrade validation failed"
  check_json shared-persistence.json '.status == "passed" and (.phases | length) == 3 and all(.phases[]; .podCount == 3 and all(.responses[]; .ok == true and .httpCode == "200" and .operation.id == "00000000-0000-4000-8000-000000000001"))' "shared PostgreSQL persistence validation failed"
  check_json pod-delete-probe-summary.json '.status == "passed" and .total >= 20 and .failures == 0 and .versions == ["ha-upgraded"]' "Pod-deletion availability probe failed"
  check_json endpoint-slice-summary.json '.status == "passed" and .total >= 20 and .queryFailures == 0 and .minReady >= 2 and .finalReady == 3' "EndpointSlice availability validation failed"
  check_json pod-delete-validation.json '.status == "passed" and .failureMode == "force-delete-zero-grace" and .deletedUidAbsent == true and .replacementCount == 1 and .readyReplicas == 3 and .probeFailures == 0 and .minReadyEndpoints >= 2 and .finalReadyEndpoints == 3' "Pod failure replacement validation failed"
  check_json post-uninstall-validation.json '.status == "passed" and all(.removed[]; . == true) and .externalDatabaseSecret.name == "gpu-control-plane-database" and .externalDatabaseSecret.retained == true and .migrationHookJobRemovedBeforeUninstall == true and .migrationHookJobRetained == false' "Helm uninstall boundary validation failed"
  check_json deployment.json '.spec.replicas == 3 and .spec.strategy.type == "RollingUpdate" and .spec.strategy.rollingUpdate.maxUnavailable == 0 and .spec.strategy.rollingUpdate.maxSurge == 1 and .spec.template.spec.containers[0].imagePullPolicy == "Never" and .status.readyReplicas == 3 and .status.availableReplicas == 3' "Deployment HA strategy evidence failed"
  check_json pdb.json '.spec.minAvailable == 2 and .status.currentHealthy == 3 and .status.desiredHealthy == 2 and .status.disruptionsAllowed >= 1' "PDB resource evidence failed"
  check_json migration-job.json '.metadata.annotations.hook == "pre-install,pre-upgrade" and .metadata.annotations.hookDeletePolicy == "before-hook-creation" and .spec.backoffLimit == 0 and .spec.activeDeadlineSeconds == 60 and .spec.template.spec.automountServiceAccountToken == false and .spec.template.spec.securityContext.runAsNonRoot == true and .spec.template.spec.containers[0].securityContext.readOnlyRootFilesystem == true and .status.succeeded == 1' "migration hook resource evidence failed"
  check_json database-secret.json '.kind == "Secret" and .metadata.name == "gpu-control-plane-database" and .dataKeys == ["DATABASE_URL"] and (has("data") | not) and (has("stringData") | not)' "database Secret metadata evidence failed"
  check_json bad-database-secret.json '.kind == "Secret" and .metadata.name == "gpu-control-plane-database-bad" and .dataKeys == ["DATABASE_URL"] and (has("data") | not) and (has("stringData") | not)' "bad database Secret metadata evidence failed"
  check_json service-account.json '.kind == "ServiceAccount" and .metadata.name == "gpu-control-plane" and .automountServiceAccountToken == false' "ServiceAccount evidence failed"
  check_json collection.json '.stage == "pre-uninstall" and .cluster == "gpu-control-plane-ha" and .namespace == "gpu-control-plane-system" and .release == "gpu-control-plane" and .versions.kubernetes == "v1.34.8"' "collection metadata failed"
  check_json baseline-image.json '(.repoTags | index("gpu-control-plane:ci-baseline")) != null and .id != null and .size > 0 and .labels["org.opencontainers.image.revision"] != null' "baseline image evidence failed"
  check_json image.json '(.repoTags | index("gpu-control-plane:ci-candidate")) != null and .id != null and .size > 0 and .labels["org.opencontainers.image.revision"] != null' "candidate image evidence failed"
  if [[ -s "${ARTIFACT_DIR}/baseline-image.json" && -s "${ARTIFACT_DIR}/image.json" ]] && ! jq -s '.[0].id != .[1].id' "${ARTIFACT_DIR}/baseline-image.json" "${ARTIFACT_DIR}/image.json" >/dev/null 2>&1; then
    add_violation "baseline and candidate image IDs are identical"
  fi
fi

violation_file="${ARTIFACT_DIR}/.evidence-policy-violations.jsonl"
: >"${violation_file}"
for violation in "${violations[@]}"; do
  jq -cn --arg violation "${violation}" '$violation' >>"${violation_file}"
done

checked_files="$(find "${ARTIFACT_DIR}" -type f ! -name evidence-sha256.txt ! -name evidence-policy-report.json ! -name .evidence-policy-violations.jsonl | awk 'END {print NR + 0}')"
jq -s \
  --arg generatedAt "$(utc_now)" \
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

echo "evidence policy passed: ${ARTIFACT_DIR}/evidence-policy-report.json"
