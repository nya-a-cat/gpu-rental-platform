#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

PROXY_PID=""
ACTIVE_PROBE_PID=""
ACTIVE_PROBE_STOP=""
ACTIVE_ENDPOINT_PID=""
ACTIVE_ENDPOINT_STOP=""
CLEANUP_COMPLETE=0
LAST_BACKGROUND_PID=""
HTTP_CODE="000"
HTTP_BODY=""

stop_active_monitors() {
  if [[ -n "${ACTIVE_PROBE_STOP}" ]]; then
    touch "${ACTIVE_PROBE_STOP}" 2>/dev/null || true
  fi
  if [[ -n "${ACTIVE_ENDPOINT_STOP}" ]]; then
    touch "${ACTIVE_ENDPOINT_STOP}" 2>/dev/null || true
  fi
  stop_background_pid "${ACTIVE_PROBE_PID}"
  stop_background_pid "${ACTIVE_ENDPOINT_PID}"
  ACTIVE_PROBE_PID=""
  ACTIVE_PROBE_STOP=""
  ACTIVE_ENDPOINT_PID=""
  ACTIVE_ENDPOINT_STOP=""
}

remove_transient_artifacts() {
  find "${ARTIFACT_DIR}" -maxdepth 1 -type f \( -name '*.stop' -o -name '*.records' -o -name '*.tmp' \) -delete 2>/dev/null || true
}

cleanup_on_exit() {
  local exit_code="$1"
  set +e
  stop_active_monitors
  remove_transient_artifacts
  stop_background_pid "${PROXY_PID}"

  if [[ "${exit_code}" -ne 0 && "${CLEANUP_COMPLETE}" -eq 0 ]]; then
    if [[ -x "${KUBECTL_BIN}" ]] && "${KUBECTL_BIN}" config get-contexts "${HA_CONTEXT}" >/dev/null 2>&1; then
      bash "${SCRIPT_DIR}/collect-evidence.sh" failure >/dev/null 2>&1 || true
      remove_transient_artifacts
    fi
    if [[ -x "${HELM_BIN}" ]]; then
      helm_ha uninstall "${HA_RELEASE}" --namespace "${HA_NAMESPACE}" --ignore-not-found >/dev/null 2>&1 || true
    fi
    if [[ -x "${KUBECTL_BIN}" ]]; then
      kubectl_ha delete namespace "${HA_NAMESPACE}" --ignore-not-found --wait=false >/dev/null 2>&1 || true
    fi
    if [[ -x "${KIND_BIN}" ]]; then
      "${KIND_BIN}" delete cluster --name "${HA_CLUSTER_NAME}" >/dev/null 2>&1 || true
    fi
  fi
  exit "${exit_code}"
}
trap 'cleanup_on_exit $?' EXIT

http_get() {
  local url="$1"
  local result

  result="$(curl --silent --max-time 3 --write-out $'\n%{http_code}' "${url}" 2>/dev/null || true)"
  if [[ "${result}" == *$'\n'* ]]; then
    HTTP_CODE="${result##*$'\n'}"
    HTTP_BODY="${result%$'\n'*}"
  else
    HTTP_CODE="000"
    HTTP_BODY=""
  fi
}

start_service_probe() {
  local output_file="$1"
  local stop_file="$2"

  : >"${output_file}"
  rm -f "${stop_file}"
  (
    while [[ ! -f "${stop_file}" ]]; do
      local_timestamp="$(utc_now)"

      http_get "${SERVICE_PROXY_URL}/health/ready"
      ready_code="${HTTP_CODE}"
      ready_status="$(printf '%s' "${HTTP_BODY}" | jq -r '.status // empty' 2>/dev/null || true)"

      http_get "${SERVICE_PROXY_URL}/api/v1/system/info"
      info_code="${HTTP_CODE}"
      version="$(printf '%s' "${HTTP_BODY}" | jq -r '.version // empty' 2>/dev/null || true)"

      ok=false
      if [[ "${ready_code}" == "200" && "${ready_status}" == "ready" && "${info_code}" == "200" && -n "${version}" ]]; then
        ok=true
      fi

      jq -cn \
        --arg timestamp "${local_timestamp}" \
        --arg readyHttp "${ready_code}" \
        --arg readyStatus "${ready_status}" \
        --arg infoHttp "${info_code}" \
        --arg version "${version}" \
        --argjson ok "${ok}" \
        '{timestamp:$timestamp,readyHttp:$readyHttp,readyStatus:$readyStatus,infoHttp:$infoHttp,version:$version,ok:$ok}' \
        >>"${output_file}"
      sleep 0.25
    done
  ) &
  LAST_BACKGROUND_PID=$!
}

start_endpoint_monitor() {
  local output_file="$1"
  local stop_file="$2"

  : >"${output_file}"
  rm -f "${stop_file}"
  (
    while [[ ! -f "${stop_file}" ]]; do
      local_timestamp="$(utc_now)"
      count="$(endpoint_ready_count 2>/dev/null || true)"
      query_ok=false
      if [[ "${count}" =~ ^[0-9]+$ ]]; then
        query_ok=true
      else
        count="0"
      fi
      jq -cn \
        --arg timestamp "${local_timestamp}" \
        --argjson readyEndpoints "${count}" \
        --argjson queryOk "${query_ok}" \
        '{timestamp:$timestamp,readyEndpoints:$readyEndpoints,queryOk:$queryOk}' \
        >>"${output_file}"
      sleep 0.25
    done
  ) &
  LAST_BACKGROUND_PID=$!
}

probe_has_minimum_samples() {
  local output_file="$1"
  local count
  count="$(awk 'END {print NR + 0}' "${output_file}" 2>/dev/null || printf '0')"
  [[ "${count}" -ge "${PROBE_MIN_SAMPLES}" ]]
}

summarize_probe() {
  local input_file="$1"
  local output_file="$2"
  local allowed_versions_json="$3"
  local final_version="$4"

  jq -s \
    --argjson minimumSamples "${PROBE_MIN_SAMPLES}" \
    --argjson allowedVersions "${allowed_versions_json}" \
    --arg finalVersion "${final_version}" '
      ([.[] | select(.ok != true)] | length) as $failures |
      ([.[].version | select(length > 0)] | unique) as $versions |
      ($versions - $allowedVersions) as $unexpectedVersions |
      {
        status:(if length >= $minimumSamples and $failures == 0 and ($unexpectedVersions | length) == 0 and ($versions | index($finalVersion)) != null then "passed" else "failed" end),
        total:length,
        failures:$failures,
        versions:$versions,
        unexpectedVersions:$unexpectedVersions,
        finalExpectedVersion:$finalVersion,
        startedAt:(.[0].timestamp // null),
        finishedAt:(.[-1].timestamp // null)
      }
    ' "${input_file}" >"${output_file}"
  jq -e '.status == "passed"' "${output_file}" >/dev/null
}

summarize_endpoint_monitor() {
  local input_file="$1"
  local output_file="$2"
  local minimum_required="$3"

  jq -s --argjson minimumRequired "${minimum_required}" '
    [.[] | select(.queryOk == true)] as $successful |
    ([.[] | select(.queryOk != true)] | length) as $queryFailures |
    ($successful | map(.readyEndpoints) | min) as $minimumReady |
    ($successful | last | .readyEndpoints) as $finalReady |
    {
      status:(if length >= 20 and $queryFailures == 0 and $minimumReady >= $minimumRequired and $finalReady == 3 then "passed" else "failed" end),
      total:length,
      queryFailures:$queryFailures,
      requiredMinimumReady:$minimumRequired,
      minReady:$minimumReady,
      finalReady:$finalReady,
      startedAt:(.[0].timestamp // null),
      finishedAt:(.[-1].timestamp // null)
    }
  ' "${input_file}" >"${output_file}"
  jq -e '.status == "passed"' "${output_file}" >/dev/null
}

capture_pod_identity() {
  local output_file="$1"
  kubectl_ha -n "${HA_NAMESPACE}" get pods -l "${CONTROL_PLANE_SELECTOR}" -o json | jq \
    --arg capturedAt "$(utc_now)" '
      {
        status:(if (.items | length) == 3 and all(.items[]; any(.status.conditions[]?; .type == "Ready" and .status == "True")) then "passed" else "failed" end),
        capturedAt:$capturedAt,
        pods:[.items[] | {name:.metadata.name,uid:.metadata.uid} | .] | sort_by(.name)
      }
    ' >"${output_file}"
  jq -e '.status == "passed"' "${output_file}" >/dev/null
}

capture_control_plane_state() {
  local output_file="$1"
  local deployment_json
  local deployment_uid
  local template_sha256
  local replica_sets_json
  local pods_json
  local service_version
  local database_secret_revision

  deployment_json="$(kubectl_ha -n "${HA_NAMESPACE}" get deployment "${CONTROL_PLANE_NAME}" -o json)"
  deployment_uid="$(printf '%s' "${deployment_json}" | jq -r '.metadata.uid')"
  template_sha256="$(printf '%s' "${deployment_json}" | jq -cS '.spec.template' | sha256sum | awk '{print $1}')"
  replica_sets_json="$(kubectl_ha -n "${HA_NAMESPACE}" get replicasets -l "${CONTROL_PLANE_SELECTOR}" -o json | jq -c \
    --arg deploymentUid "${deployment_uid}" '[.items[] | select(any(.metadata.ownerReferences[]?; .uid == $deploymentUid)) | {name:.metadata.name,uid:.metadata.uid,revision:(.metadata.annotations["deployment.kubernetes.io/revision"] // ""),desiredReplicas:(.spec.replicas // 0),readyReplicas:(.status.readyReplicas // 0),availableReplicas:(.status.availableReplicas // 0)}] | sort_by(.name)')"
  pods_json="$(kubectl_ha -n "${HA_NAMESPACE}" get pods -l "${CONTROL_PLANE_SELECTOR}" -o json | jq -c \
    '[.items[] | {
      name:.metadata.name,
      uid:.metadata.uid,
      image:([.spec.containers[] | select(.name == "control-plane") | .image] | first),
      imageID:([.status.containerStatuses[]? | select(.name == "control-plane") | .imageID] | first)
    }] | sort_by(.name)')"
  database_secret_revision="$(printf '%s' "${deployment_json}" | jq -r '.spec.template.metadata.annotations["gpu.platform.nyaacat.dev/database-secret-revision"] // ""')"

  http_get "${SERVICE_PROXY_URL}/api/v1/system/info"
  [[ "${HTTP_CODE}" == "200" ]]
  service_version="$(printf '%s' "${HTTP_BODY}" | jq -r '.version')"

  jq -n \
    --arg capturedAt "$(utc_now)" \
    --arg deploymentUid "${deployment_uid}" \
    --argjson deploymentGeneration "$(printf '%s' "${deployment_json}" | jq '.metadata.generation')" \
    --arg deploymentRevision "$(printf '%s' "${deployment_json}" | jq -r '.metadata.annotations["deployment.kubernetes.io/revision"] // ""')" \
    --arg templateSha256 "${template_sha256}" \
    --argjson replicaSets "${replica_sets_json}" \
    --argjson pods "${pods_json}" \
    --arg serviceVersion "${service_version}" \
    --arg databaseSecretRevision "${database_secret_revision}" \
    '{capturedAt:$capturedAt,deployment:{uid:$deploymentUid,generation:$deploymentGeneration,revision:$deploymentRevision,templateSha256:$templateSha256,databaseSecretRevision:$databaseSecretRevision},replicaSets:$replicaSets,pods:$pods,serviceVersion:$serviceVersion}' \
    >"${output_file}"
}

assert_operation_through_all_pods() {
  local phase="$1"
  local output_file="$2"
  local records_file="${RUNTIME_DIR}/$(basename "${output_file}").records"
  local pod_count=0
  local all_ok=true

  : >"${records_file}"
  while IFS=$'\t' read -r pod_name pod_uid; do
    [[ -n "${pod_name}" ]] || continue
    pod_count=$((pod_count + 1))
    http_get "${PROXY_BASE_URL}/api/v1/namespaces/${HA_NAMESPACE}/pods/${pod_name}:${SERVICE_PORT}/proxy/api/v1/operations/${OPERATION_ID}"

    operation_ok=false
    operation_kind=""
    operation_status=""
    operation_target_type=""
    operation_target_id=""
    if [[ "${HTTP_CODE}" == "200" ]] && printf '%s' "${HTTP_BODY}" | jq -e \
      --arg id "${OPERATION_ID}" '
        .id == $id and .kind == "ha-probe" and .status == "succeeded" and
        .target.resourceType == "control-plane" and .target.resourceId == "ha-shared-state" and
        .progress == 100 and .retryable == false
      ' >/dev/null 2>&1; then
      operation_ok=true
      operation_kind="$(printf '%s' "${HTTP_BODY}" | jq -r '.kind')"
      operation_status="$(printf '%s' "${HTTP_BODY}" | jq -r '.status')"
      operation_target_type="$(printf '%s' "${HTTP_BODY}" | jq -r '.target.resourceType')"
      operation_target_id="$(printf '%s' "${HTTP_BODY}" | jq -r '.target.resourceId')"
    else
      all_ok=false
    fi

    jq -cn \
      --arg podName "${pod_name}" \
      --arg podUid "${pod_uid}" \
      --arg httpCode "${HTTP_CODE}" \
      --arg operationId "${OPERATION_ID}" \
      --arg kind "${operation_kind}" \
      --arg operationStatus "${operation_status}" \
      --arg targetType "${operation_target_type}" \
      --arg targetId "${operation_target_id}" \
      --argjson ok "${operation_ok}" \
      '{podName:$podName,podUid:$podUid,httpCode:$httpCode,ok:$ok,operation:{id:$operationId,kind:$kind,status:$operationStatus,target:{resourceType:$targetType,resourceId:$targetId}}}' \
      >>"${records_file}"
  done < <(kubectl_ha -n "${HA_NAMESPACE}" get pods -l "${CONTROL_PLANE_SELECTOR}" -o json | jq -r '
    .items[] | select(any(.status.conditions[]?; .type == "Ready" and .status == "True")) | [.metadata.name,.metadata.uid] | @tsv
  ' | sort)

  jq -s \
    --arg phase "${phase}" \
    --argjson podCount "${pod_count}" \
    --argjson allOk "${all_ok}" \
    '{status:(if $podCount == 3 and $allOk then "passed" else "failed" end),phase:$phase,podCount:$podCount,responses:.}' \
    "${records_file}" >"${output_file}"
  rm -f "${records_file}"
  jq -e '.status == "passed"' "${output_file}" >/dev/null
}

pdb_is_valid() {
  kubectl_ha -n "${HA_NAMESPACE}" get poddisruptionbudget "${CONTROL_PLANE_NAME}" -o json 2>/dev/null | jq -e '
    .spec.minAvailable == 2 and
    .status.currentHealthy == 3 and
    .status.desiredHealthy == 2 and
    .status.disruptionsAllowed >= 1
  ' >/dev/null
}

write_security_validation() {
  local deployment_security
  local pods_security
  local migration_job_security
  local service_account_security

  deployment_security="$(kubectl_ha -n "${HA_NAMESPACE}" get deployment "${CONTROL_PLANE_NAME}" -o json | jq -c '
    {
      automountServiceAccountToken:(.spec.template.spec.automountServiceAccountToken == false),
      podSecurityContext:(
        .spec.template.spec.securityContext.runAsNonRoot == true and
        .spec.template.spec.securityContext.runAsUser == 65532 and
        .spec.template.spec.securityContext.runAsGroup == 65532 and
        .spec.template.spec.securityContext.seccompProfile.type == "RuntimeDefault"
      ),
      containerSecurityContext:all(.spec.template.spec.containers[];
        .securityContext.allowPrivilegeEscalation == false and
        .securityContext.readOnlyRootFilesystem == true and
        .securityContext.runAsNonRoot == true and
        .securityContext.runAsUser == 65532 and
        .securityContext.runAsGroup == 65532 and
        ((.securityContext.capabilities.drop // []) | index("ALL")) != null
      )
    }
  ')"
  pods_security="$(kubectl_ha -n "${HA_NAMESPACE}" get pods -l "${CONTROL_PLANE_SELECTOR}" -o json | jq -c '
    {
      podCount:(.items | length),
      allReady:all(.items[]; any(.status.conditions[]?; .type == "Ready" and .status == "True")),
      automountServiceAccountToken:all(.items[]; .spec.automountServiceAccountToken == false),
      podSecurityContext:all(.items[];
        .spec.securityContext.runAsNonRoot == true and
        .spec.securityContext.runAsUser == 65532 and
        .spec.securityContext.runAsGroup == 65532 and
        .spec.securityContext.seccompProfile.type == "RuntimeDefault"
      ),
      containerSecurityContext:all(.items[].spec.containers[];
        .securityContext.allowPrivilegeEscalation == false and
        .securityContext.readOnlyRootFilesystem == true and
        .securityContext.runAsNonRoot == true and
        .securityContext.runAsUser == 65532 and
        .securityContext.runAsGroup == 65532 and
        ((.securityContext.capabilities.drop // []) | index("ALL")) != null
      ),
      noServiceAccountTokenProjectedVolume:([.items[].spec.volumes[]? | .projected.sources[]? | select(has("serviceAccountToken"))] | length) == 0
    }
  ')"
  migration_job_security="$(kubectl_ha -n "${HA_NAMESPACE}" get job "${MIGRATION_JOB_NAME}" -o json | jq -c '
    {
      automountServiceAccountToken:(.spec.template.spec.automountServiceAccountToken == false),
      podSecurityContext:(
        .spec.template.spec.securityContext.runAsNonRoot == true and
        .spec.template.spec.securityContext.runAsUser == 65532 and
        .spec.template.spec.securityContext.runAsGroup == 65532 and
        .spec.template.spec.securityContext.seccompProfile.type == "RuntimeDefault"
      ),
      containerSecurityContext:all(.spec.template.spec.containers[];
        .securityContext.allowPrivilegeEscalation == false and
        .securityContext.readOnlyRootFilesystem == true and
        .securityContext.runAsNonRoot == true and
        .securityContext.runAsUser == 65532 and
        .securityContext.runAsGroup == 65532 and
        ((.securityContext.capabilities.drop // []) | index("ALL")) != null
      )
    }
  ')"
  service_account_security="$(kubectl_ha -n "${HA_NAMESPACE}" get serviceaccount "${CONTROL_PLANE_NAME}" -o json | jq -c '
    {automountServiceAccountToken:(.automountServiceAccountToken == false)}
  ')"

  jq -n \
    --argjson deployment "${deployment_security}" \
    --argjson pods "${pods_security}" \
    --argjson migrationJob "${migration_job_security}" \
    --argjson serviceAccount "${service_account_security}" '
      {deployment:$deployment,pods:$pods,migrationJob:$migrationJob,serviceAccount:$serviceAccount}
      | .status = (if
          all(.deployment[]; . == true) and
          .pods.podCount == 3 and .pods.allReady == true and
          .pods.automountServiceAccountToken == true and .pods.podSecurityContext == true and
          .pods.containerSecurityContext == true and .pods.noServiceAccountTokenProjectedVolume == true and
          all(.migrationJob[]; . == true) and
          .serviceAccount.automountServiceAccountToken == true
        then "passed" else "failed" end)
    ' >"${ARTIFACT_DIR}/security-validation.json"
  jq -e '.status == "passed"' "${ARTIFACT_DIR}/security-validation.json" >/dev/null
}

migration_job_failed() {
  kubectl_ha -n "${HA_NAMESPACE}" get job "${MIGRATION_JOB_NAME}" -o json 2>/dev/null | jq -e '
    any(.status.conditions[]?; .type == "Failed" and .status == "True")
  ' >/dev/null
}

old_pod_is_gone() {
  local pod_name="$1"
  local pod_uid="$2"
  local current_uid
  current_uid="$(kubectl_ha -n "${HA_NAMESPACE}" get pod "${pod_name}" --ignore-not-found -o jsonpath='{.metadata.uid}' 2>/dev/null || true)"
  [[ "${current_uid}" != "${pod_uid}" ]]
}

ensure_artifact_dir
ensure_runtime_dir

require_command curl
require_command docker
require_command jq
require_command sha256sum
require_command awk
require_command find

export TOOLS_ROOT
bash "${OCM_DEPLOY_ROOT}/scripts/install-tools.sh"

[[ -x "${KIND_BIN}" && -x "${KUBECTL_BIN}" && -x "${HELM_BIN}" ]]
[[ -d "${CONTROL_PLANE_CHART}" ]]
[[ -f "${KIND_CONFIG}" && -f "${POSTGRES_MANIFEST}" ]]
docker image inspect "${CONTROL_PLANE_BASELINE_IMAGE}" >/dev/null
docker image inspect "${CONTROL_PLANE_CANDIDATE_IMAGE}" >/dev/null
baseline_image_id="$(docker image inspect --format '{{.Id}}' "${CONTROL_PLANE_BASELINE_IMAGE}")"
candidate_image_id="$(docker image inspect --format '{{.Id}}' "${CONTROL_PLANE_CANDIDATE_IMAGE}")"
[[ "${baseline_image_id}" != "${candidate_image_id}" ]]

if "${KIND_BIN}" get clusters 2>/dev/null | grep -Fxq "${HA_CLUSTER_NAME}"; then
  "${KIND_BIN}" delete cluster --name "${HA_CLUSTER_NAME}"
fi

"${KIND_BIN}" create cluster \
  --name "${HA_CLUSTER_NAME}" \
  --image "${KIND_NODE_IMAGE}" \
  --config "${KIND_CONFIG}" \
  --wait "${WAIT_TIMEOUT}"
"${KIND_BIN}" load docker-image "${CONTROL_PLANE_BASELINE_IMAGE}" --name "${HA_CLUSTER_NAME}"
"${KIND_BIN}" load docker-image "${CONTROL_PLANE_CANDIDATE_IMAGE}" --name "${HA_CLUSTER_NAME}"

kubectl_ha create namespace "${HA_NAMESPACE}"
kubectl_ha apply -f "${POSTGRES_MANIFEST}"
kubectl_ha -n "${HA_NAMESPACE}" create secret generic "${DATABASE_SECRET_NAME}" \
  --from-literal="DATABASE_URL=${DATABASE_URL}"
kubectl_ha -n "${HA_NAMESPACE}" create secret generic "${BAD_DATABASE_SECRET_NAME}" \
  --from-literal="DATABASE_URL=${BAD_DATABASE_URL}"
database_secret_uid="$(kubectl_ha -n "${HA_NAMESPACE}" get secret "${DATABASE_SECRET_NAME}" -o jsonpath='{.metadata.uid}')"

kubectl_ha -n "${HA_NAMESPACE}" rollout status "statefulset/${POSTGRES_NAME}" --timeout="${WAIT_TIMEOUT}"

baseline_image_repository="${CONTROL_PLANE_BASELINE_IMAGE%:*}"
baseline_image_tag="${CONTROL_PLANE_BASELINE_IMAGE##*:}"
candidate_image_repository="${CONTROL_PLANE_CANDIDATE_IMAGE%:*}"
candidate_image_tag="${CONTROL_PLANE_CANDIDATE_IMAGE##*:}"
helm_values=(
  --set replicaCount=3
  --set-string "image.repository=${baseline_image_repository}"
  --set-string "image.tag=${baseline_image_tag}"
  --set-string image.pullPolicy=Never
  --set-string "database.existingSecret=${DATABASE_SECRET_NAME}"
  --set-string database.secretKey=DATABASE_URL
  --set service.port=8080
  --set podDisruptionBudget.enabled=true
  --set podDisruptionBudget.minAvailable=2
  --set-string migration.hookDeletePolicy=before-hook-creation
  --set migration.ttlSecondsAfterFinished=3600
  --set migration.backoffLimit=0
  --set migration.activeDeadlineSeconds=60
)

helm_ha upgrade --install "${HA_RELEASE}" "${CONTROL_PLANE_CHART}" \
  --namespace "${HA_NAMESPACE}" \
  --wait --timeout "${WAIT_TIMEOUT}" \
  "${helm_values[@]}" \
  --set-string database.secretRevision=ha-initial \
  --set-string config.version=ha-baseline \
  >"${ARTIFACT_DIR}/helm-install.txt"

kubectl_ha -n "${HA_NAMESPACE}" rollout status "deployment/${CONTROL_PLANE_NAME}" --timeout="${WAIT_TIMEOUT}"
wait_until "three ready control-plane replicas" 60 2 deployment_has_three_ready_replicas

"${KUBECTL_BIN}" --context "${HA_CONTEXT}" proxy \
  --address=127.0.0.1 --port="${PROXY_PORT}" \
  >"${ARTIFACT_DIR}/kubectl-proxy.log" 2>&1 &
PROXY_PID=$!
wait_until "Kubernetes API proxy" 30 1 curl --silent --fail --max-time 2 "${PROXY_BASE_URL}/version"
wait_until "baseline service version" 30 1 service_reports_version ha-baseline

migration_row="$(run_in_postgres "SELECT version || '|' || checksum FROM control_plane_schema_migrations WHERE version = '000001_phase0_foundation';")"
migration_version="${migration_row%%|*}"
migration_checksum="${migration_row#*|}"
migration_tables="$(run_in_postgres "SELECT string_agg(table_name, ',' ORDER BY table_name) FROM information_schema.tables WHERE table_schema = 'public' AND table_name = ANY (ARRAY['audit_events','audit_events_default','idempotency_records','operations','outbox_events']::text[]);")"
[[ "${migration_version}" == "000001_phase0_foundation" ]]
[[ "${migration_checksum}" =~ ^[0-9a-f]{64}$ ]]
[[ "${migration_tables}" == "audit_events,audit_events_default,idempotency_records,operations,outbox_events" ]]
jq -n \
  --arg version "${migration_version}" \
  --arg checksum "${migration_checksum}" \
  --arg tables "${migration_tables}" \
  '{status:"passed",version:$version,checksum:$checksum,requiredTables:($tables | split(","))}' \
  >"${ARTIFACT_DIR}/migration-validation.json"

wait_until "PodDisruptionBudget health" 30 2 pdb_is_valid
kubectl_ha -n "${HA_NAMESPACE}" get poddisruptionbudget "${CONTROL_PLANE_NAME}" -o json | jq '
  {status:"passed",minAvailable:.spec.minAvailable,currentHealthy:.status.currentHealthy,desiredHealthy:.status.desiredHealthy,disruptionsAllowed:.status.disruptionsAllowed,expectedPods:.status.expectedPods}
' >"${ARTIFACT_DIR}/pdb-validation.json"
write_security_validation

run_in_postgres "INSERT INTO operations (id, kind, status, target_type, target_id, steps, progress, retryable, request_id, created_at, updated_at, started_at, finished_at) VALUES ('${OPERATION_ID}', 'ha-probe', 'succeeded', 'control-plane', 'ha-shared-state', '[]'::jsonb, 100, false, 'ha-probe', now(), now(), now(), now()) ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status, updated_at = now(), finished_at = now();" >/dev/null
assert_operation_through_all_pods baseline "${ARTIFACT_DIR}/shared-persistence-baseline.json"
capture_pod_identity "${ARTIFACT_DIR}/baseline-pods.json"
capture_control_plane_state "${ARTIFACT_DIR}/baseline-state.json"

kubectl_ha -n "${HA_NAMESPACE}" create secret generic "${DATABASE_SECRET_NAME}" \
  --from-literal="DATABASE_URL=${ROTATED_DATABASE_URL}" \
  --dry-run=client -o yaml | kubectl_ha apply -f -
helm_ha upgrade "${HA_RELEASE}" "${CONTROL_PLANE_CHART}" \
  --namespace "${HA_NAMESPACE}" \
  --wait --timeout "${WAIT_TIMEOUT}" \
  "${helm_values[@]}" \
  --set-string database.secretRevision=ha-rotated \
  --set-string config.version=ha-baseline \
  >"${ARTIFACT_DIR}/secret-rotation-helm.txt"
kubectl_ha -n "${HA_NAMESPACE}" rollout status "deployment/${CONTROL_PLANE_NAME}" --timeout="${WAIT_TIMEOUT}"
wait_until "three replicas after database Secret rotation" 60 2 deployment_has_three_ready_replicas
wait_until "baseline version after database Secret rotation" 30 1 service_reports_version ha-baseline
capture_pod_identity "${ARTIFACT_DIR}/post-secret-rotation-pods.json"
capture_control_plane_state "${ARTIFACT_DIR}/post-secret-rotation-state.json"
rotated_secret_uid="$(kubectl_ha -n "${HA_NAMESPACE}" get secret "${DATABASE_SECRET_NAME}" -o jsonpath='{.metadata.uid}')"
[[ "${rotated_secret_uid}" == "${database_secret_uid}" ]]
jq -n \
  --arg baselineImage "${CONTROL_PLANE_BASELINE_IMAGE}" \
  --arg initialSecretUid "${database_secret_uid}" \
  --arg rotatedSecretUid "${rotated_secret_uid}" \
  --slurpfile before "${ARTIFACT_DIR}/baseline-state.json" \
  --slurpfile after "${ARTIFACT_DIR}/post-secret-rotation-state.json" '
    ($before[0]) as $beforeState |
    ($after[0]) as $afterState |
    (all($beforeState.pods[]; .uid as $uid | all($afterState.pods[]; .uid != $uid))) as $podsReplaced |
    {
      secretObjectRetained:($initialSecretUid == $rotatedSecretUid),
      secretRevision:$afterState.deployment.databaseSecretRevision,
      podsReplaced:$podsReplaced,
      readyReplicas:($afterState.pods | length),
      serviceVersion:$afterState.serviceVersion,
      baselineImageSelected:all($afterState.pods[]; .image == $baselineImage and (.imageID | length) > 0)
    }
    | .status = (if .secretObjectRetained and .secretRevision == "ha-rotated" and .podsReplaced and .readyReplicas == 3 and .serviceVersion == "ha-baseline" and .baselineImageSelected then "passed" else "failed" end)
  ' >"${ARTIFACT_DIR}/secret-rotation-validation.json"
jq -e '.status == "passed"' "${ARTIFACT_DIR}/secret-rotation-validation.json" >/dev/null
assert_operation_through_all_pods secret-rotated "${ARTIFACT_DIR}/shared-persistence-secret-rotated.json"

bad_probe_file="${ARTIFACT_DIR}/bad-upgrade-probe.jsonl"
bad_probe_summary="${ARTIFACT_DIR}/bad-upgrade-probe-summary.json"
bad_probe_stop="${RUNTIME_DIR}/bad-upgrade-probe.stop"
start_service_probe "${bad_probe_file}" "${bad_probe_stop}"
ACTIVE_PROBE_PID="${LAST_BACKGROUND_PID}"
ACTIVE_PROBE_STOP="${bad_probe_stop}"

set +e
helm_ha upgrade "${HA_RELEASE}" "${CONTROL_PLANE_CHART}" \
  --namespace "${HA_NAMESPACE}" \
  --wait --timeout 90s \
  "${helm_values[@]}" \
  --set-string "database.existingSecret=${BAD_DATABASE_SECRET_NAME}" \
  --set-string database.secretRevision=ha-rotated \
  --set-string config.version=ha-bad \
  >"${ARTIFACT_DIR}/bad-upgrade-helm.txt" 2>&1
bad_upgrade_exit_code=$?
set -e
[[ "${bad_upgrade_exit_code}" -ne 0 ]]

wait_until "failed migration Job" 50 2 migration_job_failed
kubectl_ha -n "${HA_NAMESPACE}" get job "${MIGRATION_JOB_NAME}" -o json | jq '
  {
    name:.metadata.name,
    uid:.metadata.uid,
    backoffLimit:.spec.backoffLimit,
    activeDeadlineSeconds:.spec.activeDeadlineSeconds,
    databaseSecret:.spec.template.spec.containers[0].env[] | select(.name == "DATABASE_URL") | .valueFrom.secretKeyRef,
    status:{succeeded:.status.succeeded,failed:.status.failed,conditions:[.status.conditions[]? | {type,status,reason}]}
  }
' >"${ARTIFACT_DIR}/bad-migration-job.json"

wait_until "minimum bad-upgrade probe samples" 80 1 probe_has_minimum_samples "${bad_probe_file}"
touch "${bad_probe_stop}"
stop_background_pid "${ACTIVE_PROBE_PID}"
rm -f "${bad_probe_stop}"
ACTIVE_PROBE_PID=""
ACTIVE_PROBE_STOP=""
summarize_probe "${bad_probe_file}" "${bad_probe_summary}" '["ha-baseline"]' ha-baseline
wait_until "baseline remains serving after failed migration" 30 1 service_reports_version ha-baseline
wait_until "three replicas remain ready after failed migration" 30 2 deployment_has_three_ready_replicas
capture_control_plane_state "${ARTIFACT_DIR}/post-bad-upgrade-state.json"

jq -n \
  --argjson helmExitCode "${bad_upgrade_exit_code}" \
  --slurpfile before "${ARTIFACT_DIR}/post-secret-rotation-state.json" \
  --slurpfile after "${ARTIFACT_DIR}/post-bad-upgrade-state.json" \
  --slurpfile probe "${bad_probe_summary}" '
    ($before[0]) as $beforeState |
    ($after[0]) as $afterState |
    {
      helmExitCode:$helmExitCode,
      helmUpgradeFailed:($helmExitCode != 0),
      deploymentUnchanged:($beforeState.deployment == $afterState.deployment),
      replicaSetsUnchanged:($beforeState.replicaSets == $afterState.replicaSets),
      podsUnchanged:($beforeState.pods == $afterState.pods),
      versionUnchanged:($beforeState.serviceVersion == "ha-baseline" and $afterState.serviceVersion == "ha-baseline"),
      probeFailures:$probe[0].failures
    }
    | .status = (if .helmUpgradeFailed and .deploymentUnchanged and .replicaSetsUnchanged and .podsUnchanged and .versionUnchanged and .probeFailures == 0 then "passed" else "failed" end)
  ' >"${ARTIFACT_DIR}/bad-upgrade-validation.json"
jq -e '.status == "passed"' "${ARTIFACT_DIR}/bad-upgrade-validation.json" >/dev/null

rollout_probe_file="${ARTIFACT_DIR}/rolling-upgrade-probe.jsonl"
rollout_probe_summary="${ARTIFACT_DIR}/rolling-upgrade-probe-summary.json"
rollout_probe_stop="${RUNTIME_DIR}/rolling-upgrade-probe.stop"
rollout_endpoint_file="${ARTIFACT_DIR}/rolling-upgrade-endpoint-slice-monitor.jsonl"
rollout_endpoint_summary="${ARTIFACT_DIR}/rolling-upgrade-endpoint-slice-summary.json"
rollout_endpoint_stop="${RUNTIME_DIR}/rolling-upgrade-endpoint-slice-monitor.stop"
start_service_probe "${rollout_probe_file}" "${rollout_probe_stop}"
ACTIVE_PROBE_PID="${LAST_BACKGROUND_PID}"
ACTIVE_PROBE_STOP="${rollout_probe_stop}"
start_endpoint_monitor "${rollout_endpoint_file}" "${rollout_endpoint_stop}"
ACTIVE_ENDPOINT_PID="${LAST_BACKGROUND_PID}"
ACTIVE_ENDPOINT_STOP="${rollout_endpoint_stop}"

helm_ha upgrade "${HA_RELEASE}" "${CONTROL_PLANE_CHART}" \
  --namespace "${HA_NAMESPACE}" \
  --wait --timeout "${WAIT_TIMEOUT}" \
  "${helm_values[@]}" \
  --set-string "image.repository=${candidate_image_repository}" \
  --set-string "image.tag=${candidate_image_tag}" \
  --set-string "database.existingSecret=${DATABASE_SECRET_NAME}" \
  --set-string database.secretRevision=ha-rotated \
  --set-string config.version=ha-upgraded \
  >"${ARTIFACT_DIR}/rolling-upgrade-helm.txt"
kubectl_ha -n "${HA_NAMESPACE}" rollout status "deployment/${CONTROL_PLANE_NAME}" --timeout="${WAIT_TIMEOUT}"
wait_until "three upgraded replicas" 60 2 deployment_has_three_ready_replicas
wait_until "upgraded service version" 30 1 service_reports_version ha-upgraded
wait_until "minimum rolling-upgrade probe samples" 80 1 probe_has_minimum_samples "${rollout_probe_file}"
wait_until "minimum rolling-upgrade EndpointSlice samples" 80 1 probe_has_minimum_samples "${rollout_endpoint_file}"
touch "${rollout_probe_stop}" "${rollout_endpoint_stop}"
stop_background_pid "${ACTIVE_PROBE_PID}"
stop_background_pid "${ACTIVE_ENDPOINT_PID}"
rm -f "${rollout_probe_stop}" "${rollout_endpoint_stop}"
ACTIVE_PROBE_PID=""
ACTIVE_PROBE_STOP=""
summarize_probe "${rollout_probe_file}" "${rollout_probe_summary}" '["ha-baseline","ha-upgraded"]' ha-upgraded
ACTIVE_ENDPOINT_PID=""
ACTIVE_ENDPOINT_STOP=""

summarize_endpoint_monitor "${rollout_endpoint_file}" "${rollout_endpoint_summary}" 3
strategy_valid=false
if kubectl_ha -n "${HA_NAMESPACE}" get deployment "${CONTROL_PLANE_NAME}" -o json | jq -e '
  .spec.strategy.type == "RollingUpdate" and
  .spec.strategy.rollingUpdate.maxUnavailable == 0 and
  .spec.strategy.rollingUpdate.maxSurge == 1
' >/dev/null; then
  strategy_valid=true
fi
capture_control_plane_state "${ARTIFACT_DIR}/post-upgrade-state.json"
capture_pod_identity "${ARTIFACT_DIR}/post-upgrade-pods.json"

jq -n \
  --argjson strategyValid "${strategy_valid}" \
  --arg baselineImage "${CONTROL_PLANE_BASELINE_IMAGE}" \
  --arg candidateImage "${CONTROL_PLANE_CANDIDATE_IMAGE}" \
  --slurpfile before "${ARTIFACT_DIR}/post-secret-rotation-state.json" \
  --slurpfile after "${ARTIFACT_DIR}/post-upgrade-state.json" \
  --slurpfile probe "${rollout_probe_summary}" \
  --slurpfile endpoints "${rollout_endpoint_summary}" '
    ($before[0]) as $beforeState |
    ($after[0]) as $afterState |
    (all($beforeState.pods[]; .uid as $uid | all($afterState.pods[]; .uid != $uid))) as $podsReplaced |
    (all($beforeState.pods[]; .image == $baselineImage and (.imageID | length) > 0)) as $baselineImageSelected |
    (all($afterState.pods[]; .image == $candidateImage and (.imageID | length) > 0)) as $candidateImageSelected |
    (all($beforeState.pods[]; .imageID as $imageID | all($afterState.pods[]; .imageID != $imageID))) as $imageIDsChanged |
    (all($beforeState.replicaSets[]; .uid as $uid | any($afterState.replicaSets[]; .uid == $uid and .desiredReplicas == 0 and .readyReplicas == 0 and .availableReplicas == 0))) as $previousReplicaSetsScaledToZero |
    ([ $afterState.replicaSets[] | select(.desiredReplicas == 3 and .readyReplicas == 3 and .availableReplicas == 3) ] | length) as $readyReplicaSetCount |
    (all($afterState.replicaSets[]; if .revision == $afterState.deployment.revision then .desiredReplicas == 3 and .readyReplicas == 3 and .availableReplicas == 3 else .desiredReplicas == 0 and .readyReplicas == 0 and .availableReplicas == 0 end)) as $allNonCurrentReplicaSetsScaledToZero |
    {
      strategyValid:$strategyValid,
      podsReplaced:$podsReplaced,
      baselineImage:$baselineImage,
      candidateImage:$candidateImage,
      baselineImageSelected:$baselineImageSelected,
      candidateImageSelected:$candidateImageSelected,
      imageIDsChanged:$imageIDsChanged,
      previousReplicaSetsScaledToZero:$previousReplicaSetsScaledToZero,
      allNonCurrentReplicaSetsScaledToZero:$allNonCurrentReplicaSetsScaledToZero,
      readyReplicaSetCount:$readyReplicaSetCount,
      readyReplicas:($afterState.pods | length),
      finalVersion:$afterState.serviceVersion,
      probeFailures:$probe[0].failures,
      minReadyEndpoints:$endpoints[0].minReady,
      finalReadyEndpoints:$endpoints[0].finalReady
    }
    | .status = (if .strategyValid and .podsReplaced and .baselineImageSelected and .candidateImageSelected and .imageIDsChanged and .previousReplicaSetsScaledToZero and .allNonCurrentReplicaSetsScaledToZero and .readyReplicaSetCount == 1 and .readyReplicas == 3 and .finalVersion == "ha-upgraded" and .probeFailures == 0 and .minReadyEndpoints >= 3 and .finalReadyEndpoints == 3 then "passed" else "failed" end)
  ' >"${ARTIFACT_DIR}/rolling-upgrade-validation.json"
jq -e '.status == "passed"' "${ARTIFACT_DIR}/rolling-upgrade-validation.json" >/dev/null

assert_operation_through_all_pods upgraded "${ARTIFACT_DIR}/shared-persistence-upgraded.json"
jq -s '
  {status:(if all(.[]; .status == "passed" and .podCount == 3) then "passed" else "failed" end),phases:.}
' "${ARTIFACT_DIR}/shared-persistence-baseline.json" \
  "${ARTIFACT_DIR}/shared-persistence-secret-rotated.json" \
  "${ARTIFACT_DIR}/shared-persistence-upgraded.json" \
  >"${ARTIFACT_DIR}/shared-persistence.json"
jq -e '.status == "passed"' "${ARTIFACT_DIR}/shared-persistence.json" >/dev/null

capture_pod_identity "${ARTIFACT_DIR}/pre-delete-pods.json"
pod_to_delete="$(jq -r '.pods[0].name' "${ARTIFACT_DIR}/pre-delete-pods.json")"
pod_uid_to_delete="$(jq -r '.pods[0].uid' "${ARTIFACT_DIR}/pre-delete-pods.json")"

delete_probe_file="${ARTIFACT_DIR}/pod-delete-probe.jsonl"
delete_probe_summary="${ARTIFACT_DIR}/pod-delete-probe-summary.json"
delete_probe_stop="${RUNTIME_DIR}/pod-delete-probe.stop"
endpoint_file="${ARTIFACT_DIR}/endpoint-slice-monitor.jsonl"
endpoint_summary="${ARTIFACT_DIR}/endpoint-slice-summary.json"
endpoint_stop="${RUNTIME_DIR}/endpoint-slice-monitor.stop"

start_service_probe "${delete_probe_file}" "${delete_probe_stop}"
ACTIVE_PROBE_PID="${LAST_BACKGROUND_PID}"
ACTIVE_PROBE_STOP="${delete_probe_stop}"
start_endpoint_monitor "${endpoint_file}" "${endpoint_stop}"
ACTIVE_ENDPOINT_PID="${LAST_BACKGROUND_PID}"
ACTIVE_ENDPOINT_STOP="${endpoint_stop}"

kubectl_ha -n "${HA_NAMESPACE}" delete pod "${pod_to_delete}" --force --grace-period=0 --wait=false
wait_until "deleted Pod UID is gone" 60 1 old_pod_is_gone "${pod_to_delete}" "${pod_uid_to_delete}"
wait_until "replacement returns Deployment to three ready replicas" 60 1 deployment_has_three_ready_replicas
wait_until "EndpointSlice returns to three ready endpoints" 60 1 endpoint_count_is_three
wait_until "minimum pod-delete probe samples" 80 1 probe_has_minimum_samples "${delete_probe_file}"
wait_until "minimum EndpointSlice samples" 80 1 probe_has_minimum_samples "${endpoint_file}"

touch "${delete_probe_stop}" "${endpoint_stop}"
stop_background_pid "${ACTIVE_PROBE_PID}"
stop_background_pid "${ACTIVE_ENDPOINT_PID}"
rm -f "${delete_probe_stop}" "${endpoint_stop}"
ACTIVE_PROBE_PID=""
ACTIVE_PROBE_STOP=""
ACTIVE_ENDPOINT_PID=""
ACTIVE_ENDPOINT_STOP=""

summarize_probe "${delete_probe_file}" "${delete_probe_summary}" '["ha-upgraded"]' ha-upgraded
summarize_endpoint_monitor "${endpoint_file}" "${endpoint_summary}" 2
capture_pod_identity "${ARTIFACT_DIR}/post-delete-pods.json"

jq -n \
  --arg deletedPod "${pod_to_delete}" \
  --arg deletedUid "${pod_uid_to_delete}" \
  --arg failureMode "force-delete-zero-grace" \
  --slurpfile before "${ARTIFACT_DIR}/pre-delete-pods.json" \
  --slurpfile after "${ARTIFACT_DIR}/post-delete-pods.json" \
  --slurpfile probe "${delete_probe_summary}" \
  --slurpfile endpoints "${endpoint_summary}" '
    ($before[0].pods | map(.uid)) as $beforeUids |
    ($after[0].pods | map(.uid)) as $afterUids |
    ($afterUids - $beforeUids) as $replacementUids |
    {
      deletedPod:$deletedPod,
      deletedUid:$deletedUid,
      failureMode:$failureMode,
      replacementUids:$replacementUids,
      deletedUidAbsent:($afterUids | index($deletedUid) == null),
      replacementCount:($replacementUids | length),
      readyReplicas:($afterUids | length),
      probeFailures:$probe[0].failures,
      minReadyEndpoints:$endpoints[0].minReady,
      finalReadyEndpoints:$endpoints[0].finalReady
    }
    | .status = (if .failureMode == "force-delete-zero-grace" and .deletedUidAbsent and .replacementCount == 1 and .readyReplicas == 3 and .probeFailures == 0 and .minReadyEndpoints >= 2 and .finalReadyEndpoints == 3 then "passed" else "failed" end)
  ' >"${ARTIFACT_DIR}/pod-delete-validation.json"
jq -e '.status == "passed"' "${ARTIFACT_DIR}/pod-delete-validation.json" >/dev/null

bash "${SCRIPT_DIR}/collect-evidence.sh" pre-uninstall

kubectl_ha -n "${HA_NAMESPACE}" delete job "${MIGRATION_JOB_NAME}" --wait=true --timeout=60s
wait_until "retained migration evidence Job removed" 30 1 resource_is_absent job "${MIGRATION_JOB_NAME}"
helm_ha uninstall "${HA_RELEASE}" --namespace "${HA_NAMESPACE}" >"${ARTIFACT_DIR}/helm-uninstall.txt"
wait_until "Deployment removed" 30 1 resource_is_absent deployment "${CONTROL_PLANE_NAME}"
wait_until "Service removed" 30 1 resource_is_absent service "${CONTROL_PLANE_NAME}"
wait_until "PodDisruptionBudget removed" 30 1 resource_is_absent poddisruptionbudget "${CONTROL_PLANE_NAME}"
wait_until "ServiceAccount removed" 30 1 resource_is_absent serviceaccount "${CONTROL_PLANE_NAME}"

retained_secret_uid="$(kubectl_ha -n "${HA_NAMESPACE}" get secret "${DATABASE_SECRET_NAME}" -o jsonpath='{.metadata.uid}')"
[[ "${retained_secret_uid}" == "${database_secret_uid}" ]]
migration_job_retained=false
if kubectl_ha -n "${HA_NAMESPACE}" get job "${MIGRATION_JOB_NAME}" >/dev/null 2>&1; then
  migration_job_retained=true
fi
jq -n \
  --arg secretName "${DATABASE_SECRET_NAME}" \
  --arg secretUid "${retained_secret_uid}" \
  --argjson migrationJobRetained "${migration_job_retained}" \
  '{removed:{deployment:true,service:true,podDisruptionBudget:true,serviceAccount:true},externalDatabaseSecret:{name:$secretName,uid:$secretUid,retained:true},migrationHookJobRemovedBeforeUninstall:true,migrationHookJobRetained:$migrationJobRetained}
  | .status = (if (.removed | all(.[]; . == true)) and .externalDatabaseSecret.retained and .migrationHookJobRemovedBeforeUninstall and (.migrationHookJobRetained | not) then "passed" else "failed" end)' \
  >"${ARTIFACT_DIR}/post-uninstall-validation.json"

kubectl_ha delete namespace "${HA_NAMESPACE}" --wait=true --timeout=120s
stop_background_pid "${PROXY_PID}"
PROXY_PID=""
"${KIND_BIN}" delete cluster --name "${HA_CLUSTER_NAME}"
cluster_removed=true
if "${KIND_BIN}" get clusters 2>/dev/null | grep -Fxq "${HA_CLUSTER_NAME}"; then
  cluster_removed=false
fi
[[ "${cluster_removed}" == "true" ]]

jq -n \
  --arg completedAt "$(utc_now)" \
  --argjson clusterRemoved "${cluster_removed}" \
  '{status:(if $clusterRemoved then "passed" else "failed" end),completedAt:$completedAt,namespaceRemoved:true,clusterRemoved:$clusterRemoved}' \
  >"${ARTIFACT_DIR}/cleanup-summary.json"

jq -n \
  --slurpfile migration "${ARTIFACT_DIR}/migration-validation.json" \
  --slurpfile pdb "${ARTIFACT_DIR}/pdb-validation.json" \
  --slurpfile security "${ARTIFACT_DIR}/security-validation.json" \
  --slurpfile secretRotation "${ARTIFACT_DIR}/secret-rotation-validation.json" \
  --slurpfile badUpgrade "${ARTIFACT_DIR}/bad-upgrade-validation.json" \
  --slurpfile rollingUpgrade "${ARTIFACT_DIR}/rolling-upgrade-validation.json" \
  --slurpfile rollingEndpoints "${ARTIFACT_DIR}/rolling-upgrade-endpoint-slice-summary.json" \
  --slurpfile persistence "${ARTIFACT_DIR}/shared-persistence.json" \
  --slurpfile podDelete "${ARTIFACT_DIR}/pod-delete-validation.json" \
  --slurpfile uninstall "${ARTIFACT_DIR}/post-uninstall-validation.json" \
  --slurpfile cleanup "${ARTIFACT_DIR}/cleanup-summary.json" '
    [$migration[0],$pdb[0],$security[0],$secretRotation[0],$badUpgrade[0],$rollingUpgrade[0],$rollingEndpoints[0],$persistence[0],$podDelete[0],$uninstall[0],$cleanup[0]] as $checks |
    {
      status:(if all($checks[]; .status == "passed") then "passed" else "failed" end),
      checks:{migration:$migration[0].status,pdb:$pdb[0].status,security:$security[0].status,secretRotation:$secretRotation[0].status,badUpgrade:$badUpgrade[0].status,rollingUpgrade:$rollingUpgrade[0].status,rollingEndpoints:$rollingEndpoints[0].status,sharedPersistence:$persistence[0].status,podFailure:$podDelete[0].status,uninstall:$uninstall[0].status,cleanup:$cleanup[0].status}
    }
  ' >"${ARTIFACT_DIR}/ha-assertions.json"
jq -e '.status == "passed"' "${ARTIFACT_DIR}/ha-assertions.json" >/dev/null

CLEANUP_COMPLETE=1
echo "GPU control-plane HA verification passed; evidence: ${ARTIFACT_DIR}"
