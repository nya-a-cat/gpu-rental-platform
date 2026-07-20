#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONTROL_PLANE_DEPLOY_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${CONTROL_PLANE_DEPLOY_ROOT}/../.." && pwd)"
OCM_DEPLOY_ROOT="${REPO_ROOT}/deploy/ocm"

# shellcheck source=../../ocm/versions.env
source "${OCM_DEPLOY_ROOT}/versions.env"

TOOLS_ROOT="${TOOLS_ROOT:-${RUNNER_TEMP:-${CONTROL_PLANE_DEPLOY_ROOT}/.tools}/gpu-cloud-control-plane}"
BIN_DIR="${TOOLS_ROOT}/bin"
KIND_BIN="${BIN_DIR}/kind"
KUBECTL_BIN="${BIN_DIR}/kubectl"
HELM_BIN="${BIN_DIR}/helm"
export TOOLS_ROOT BIN_DIR
export PATH="${BIN_DIR}:${PATH}"

ARTIFACT_DIR="${ARTIFACT_DIR:-${REPO_ROOT}/artifacts/gpu-control-plane-ha}"
RUNTIME_DIR="${RUNTIME_DIR:-${RUNNER_TEMP:-${TOOLS_ROOT}/runtime}/gpu-control-plane-ha}"

HA_CLUSTER_NAME="gpu-control-plane-ha"
HA_CONTEXT="kind-${HA_CLUSTER_NAME}"
HA_NAMESPACE="gpu-control-plane-system"
HA_RELEASE="gpu-control-plane"
CONTROL_PLANE_NAME="gpu-control-plane"
CONTROL_PLANE_BASELINE_IMAGE="${CONTROL_PLANE_BASELINE_IMAGE:-gpu-control-plane:ci-baseline}"
CONTROL_PLANE_CANDIDATE_IMAGE="${CONTROL_PLANE_CANDIDATE_IMAGE:-gpu-control-plane:ci-candidate}"
CONTROL_PLANE_CHART="${REPO_ROOT}/charts/gpu-control-plane"
KIND_CONFIG="${CONTROL_PLANE_DEPLOY_ROOT}/kind/ha.yaml"
POSTGRES_MANIFEST="${CONTROL_PLANE_DEPLOY_ROOT}/manifests/postgres.yaml"

POSTGRES_NAME="gpu-control-plane-postgres"
POSTGRES_DATABASE="gpu_cloud"
POSTGRES_USER="gpu_cloud"
POSTGRES_PASSWORD="ci-only-ha-password"
DATABASE_SECRET_NAME="gpu-control-plane-database"
BAD_DATABASE_SECRET_NAME="gpu-control-plane-database-bad"
DATABASE_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_NAME}:5432/${POSTGRES_DATABASE}?sslmode=disable"
ROTATED_DATABASE_URL="${DATABASE_URL}&application_name=ha-secret-rotation"
BAD_DATABASE_URL="postgres://${POSTGRES_USER}:invalid@127.0.0.1:1/${POSTGRES_DATABASE}?sslmode=disable"

SERVICE_PORT="8080"
PROXY_PORT="${PROXY_PORT:-18080}"
PROXY_BASE_URL="http://127.0.0.1:${PROXY_PORT}"
SERVICE_PROXY_URL="${PROXY_BASE_URL}/api/v1/namespaces/${HA_NAMESPACE}/services/http:${CONTROL_PLANE_NAME}:${SERVICE_PORT}/proxy"

CONTROL_PLANE_SELECTOR="app.kubernetes.io/name=gpu-control-plane,app.kubernetes.io/instance=${HA_RELEASE},app.kubernetes.io/component=control-plane"
MIGRATION_JOB_NAME="${CONTROL_PLANE_NAME}-migrate"
OPERATION_ID="00000000-0000-4000-8000-000000000001"

WAIT_TIMEOUT="${WAIT_TIMEOUT:-300s}"
PROBE_MIN_SAMPLES="${PROBE_MIN_SAMPLES:-20}"

require_command() {
  local command_name="$1"
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "required command is unavailable: ${command_name}" >&2
    return 1
  fi
}

kubectl_ha() {
  "${KUBECTL_BIN}" --context "${HA_CONTEXT}" "$@"
}

helm_ha() {
  "${HELM_BIN}" --kube-context "${HA_CONTEXT}" "$@"
}

utc_now() {
  date -u +"%Y-%m-%dT%H:%M:%SZ"
}

wait_until() {
  local description="$1"
  local attempts="$2"
  local interval_seconds="$3"
  shift 3

  local attempt
  for ((attempt = 1; attempt <= attempts; attempt++)); do
    if "$@"; then
      echo "ready: ${description}"
      return 0
    fi
    sleep "${interval_seconds}"
  done

  echo "timed out waiting for: ${description}" >&2
  return 1
}

deployment_has_three_ready_replicas() {
  kubectl_ha -n "${HA_NAMESPACE}" get deployment "${CONTROL_PLANE_NAME}" -o json 2>/dev/null | jq -e '
    .spec.replicas == 3 and
    .status.readyReplicas == 3 and
    .status.availableReplicas == 3 and
    .status.updatedReplicas == 3 and
    (.status.unavailableReplicas // 0) == 0
  ' >/dev/null
}

resource_is_absent() {
  local resource_type="$1"
  local resource_name="$2"
  local output

  if ! output="$(kubectl_ha -n "${HA_NAMESPACE}" get "${resource_type}" "${resource_name}" --ignore-not-found -o name 2>&1)"; then
    echo "unable to verify absence of ${resource_type}/${resource_name}: ${output}" >&2
    return 1
  fi
  [[ -z "${output}" ]]
}

endpoint_ready_count() {
  kubectl_ha -n "${HA_NAMESPACE}" get endpointslices \
    -l "kubernetes.io/service-name=${CONTROL_PLANE_NAME}" -o json 2>/dev/null | jq '
      [.items[].endpoints[]? | select(.conditions.ready == true)] | length
    '
}

endpoint_count_is_three() {
  [[ "$(endpoint_ready_count)" == "3" ]]
}

service_reports_version() {
  local expected_version="$1"
  local response_file
  local http_code

  response_file="$(mktemp)"
  http_code="$(curl --silent --show-error --max-time 3 --output "${response_file}" --write-out '%{http_code}' \
    "${SERVICE_PROXY_URL}/api/v1/system/info" || true)"
  if [[ "${http_code}" != "200" ]]; then
    rm -f "${response_file}"
    return 1
  fi
  local result=1
  if jq -e --arg version "${expected_version}" '.version == $version' "${response_file}" >/dev/null; then
    result=0
  fi
  rm -f "${response_file}"
  return "${result}"
}

run_in_postgres() {
  local sql="$1"
  kubectl_ha -n "${HA_NAMESPACE}" exec "statefulset/${POSTGRES_NAME}" -- \
    psql --username="${POSTGRES_USER}" --dbname="${POSTGRES_DATABASE}" \
      --no-psqlrc --tuples-only --no-align --set=ON_ERROR_STOP=1 --command="${sql}"
}

stop_background_pid() {
  local pid="${1:-}"
  if [[ -n "${pid}" ]] && kill -0 "${pid}" >/dev/null 2>&1; then
    kill "${pid}" >/dev/null 2>&1 || true
    wait "${pid}" >/dev/null 2>&1 || true
  fi
}

ensure_artifact_dir() {
  mkdir -p "${ARTIFACT_DIR}"
}

ensure_runtime_dir() {
  mkdir -p "${RUNTIME_DIR}"
}
