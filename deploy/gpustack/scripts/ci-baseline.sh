#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
# shellcheck source=../versions.env
source "${DEPLOY_DIR}/versions.env"

ARTIFACT_DIR="${ARTIFACT_DIR:-${GITHUB_WORKSPACE:-$(pwd)}/artifacts/gpustack-baseline}"
RUNNER_TEMP="${RUNNER_TEMP:-/tmp}"
RAW_LOG="${RUNNER_TEMP}/gpustack-baseline.log"
VENV_DIR="${RUNNER_TEMP}/gpustack-venv"
UV_CACHE_DIR="${RUNNER_TEMP}/gpustack-uv-cache"
DATA_DIR="${RUNNER_TEMP}/gpustack-data"
CACHE_DIR="${RUNNER_TEMP}/gpustack-cache"
WHEEL_PATH="${RUNNER_TEMP}/gpustack-${GPUSTACK_VERSION}-py3-none-any.whl"
COOKIE_JAR="${RUNNER_TEMP}/gpustack-cookie.jar"
API_PORT="${GPUSTACK_API_PORT:-30080}"
BASE_URL="http://127.0.0.1:${API_PORT}"
SERVER_PID=""
SERVER_START_COUNT=0

mkdir -p "${ARTIFACT_DIR}" "${UV_CACHE_DIR}" "${DATA_DIR}" "${CACHE_DIR}"
: >"${RAW_LOG}"
exec > >(tee -a "${RAW_LOG}") 2>&1

utc_now() {
  date -u +'%Y-%m-%dT%H:%M:%SZ'
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "required command is unavailable: $1" >&2
    exit 1
  fi
}

stop_server() {
  if [[ -z "${SERVER_PID}" ]]; then
    return
  fi
  if kill -0 "${SERVER_PID}" >/dev/null 2>&1; then
    kill "${SERVER_PID}" >/dev/null 2>&1 || true
    for _ in $(seq 1 30); do
      if ! kill -0 "${SERVER_PID}" >/dev/null 2>&1; then
        break
      fi
      sleep 1
    done
  fi
  if kill -0 "${SERVER_PID}" >/dev/null 2>&1; then
    kill -9 "${SERVER_PID}" >/dev/null 2>&1 || true
  fi
  wait "${SERVER_PID}" >/dev/null 2>&1 || true
  SERVER_PID=""
}

cleanup_on_exit() {
  local exit_code="$1"
  set +e
  stop_server
  rm -f "${COOKIE_JAR}"
  jq -n \
    --arg generatedAt "$(utc_now)" \
    --argjson processStopped true \
    --argjson serverStarts "${SERVER_START_COUNT}" \
    --arg status "$(if [[ "${exit_code}" -eq 0 ]]; then printf passed; else printf failed; fi)" \
    '{status:$status,generatedAt:$generatedAt,processStopped:$processStopped,serverStarts:$serverStarts}' \
    >"${ARTIFACT_DIR}/cleanup-summary.json"
  trap - EXIT
  exit "${exit_code}"
}

trap 'cleanup_on_exit $?' EXIT

for command_name in curl find grep jq psql python3 sha256sum sort sudo systemctl uv; do
  require_command "${command_name}"
done

: "${GPUSTACK_CI_DATABASE_USER:?GPUSTACK_CI_DATABASE_USER is required}"
: "${GPUSTACK_CI_DATABASE_PASSWORD:?GPUSTACK_CI_DATABASE_PASSWORD is required}"
: "${GPUSTACK_CI_DATABASE_NAME:?GPUSTACK_CI_DATABASE_NAME is required}"
: "${GPUSTACK_DATABASE_URL:?GPUSTACK_DATABASE_URL is required}"
: "${GPUSTACK_BOOTSTRAP_PASSWORD:?GPUSTACK_BOOTSTRAP_PASSWORD is required}"

echo "Starting the preinstalled PostgreSQL service"
sudo systemctl start postgresql.service
for _ in $(seq 1 30); do
  if sudo -u postgres psql -Atqc 'select 1' postgres >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
sudo -u postgres psql -Atqc 'select 1' postgres >/dev/null

if ! sudo -u postgres psql -Atqc "select 1 from pg_roles where rolname='${GPUSTACK_CI_DATABASE_USER}'" postgres | grep -qx '1'; then
  sudo -u postgres psql -v ON_ERROR_STOP=1 postgres -c \
    "create role ${GPUSTACK_CI_DATABASE_USER} login password '${GPUSTACK_CI_DATABASE_PASSWORD}'" >/dev/null
fi
if ! sudo -u postgres psql -Atqc "select 1 from pg_database where datname='${GPUSTACK_CI_DATABASE_NAME}'" postgres | grep -qx '1'; then
  sudo -u postgres createdb --owner "${GPUSTACK_CI_DATABASE_USER}" "${GPUSTACK_CI_DATABASE_NAME}"
fi

echo "Downloading the pinned GPUStack release wheel"
curl --fail --location --retry 3 --silent --show-error \
  "${GPUSTACK_WHEEL_URL}" --output "${WHEEL_PATH}"
printf '%s  %s\n' "${GPUSTACK_WHEEL_SHA256}" "${WHEEL_PATH}" | sha256sum --check -

export UV_CACHE_DIR
uv venv "${VENV_DIR}" --python "${GPUSTACK_PYTHON_VERSION}" --no-python-downloads
VENV_PYTHON="${VENV_DIR}/bin/python"
VENV_GPUSTACK="${VENV_DIR}/bin/gpustack"
uv pip install --python "${VENV_PYTHON}" -r "${DEPLOY_DIR}/requirements-v2.2.1.lock"
uv pip install --python "${VENV_PYTHON}" --no-deps "${WHEEL_PATH}"

installed_version="$("${VENV_PYTHON}" -c 'import importlib.metadata as m; print(m.version("gpustack"))')"
[[ "${installed_version}" == "${GPUSTACK_VERSION}" ]]
uv pip list --python "${VENV_PYTHON}" --format freeze | sort >"${ARTIFACT_DIR}/installed-packages.txt"
package_count="$(grep -Ec '^[A-Za-z0-9_.-]+==' "${ARTIFACT_DIR}/installed-packages.txt")"

oversized_count="$(find "${UV_CACHE_DIR}" -type f -size +"${GPUSTACK_MAX_PACKAGE_BYTES}"c | awk 'END {print NR + 0}')"
[[ "${oversized_count}" -eq 0 ]]
largest_cache_entry="$(find "${UV_CACHE_DIR}" -type f -printf '%s %f\n' | sort -nr | head -n 1)"
largest_cache_bytes="${largest_cache_entry%% *}"
largest_cache_name="${largest_cache_entry#* }"
[[ "${largest_cache_bytes}" =~ ^[0-9]+$ ]]

jq -n \
  --argjson maxPackageBytes "${GPUSTACK_MAX_PACKAGE_BYTES}" \
  --argjson oversizedFiles "${oversized_count}" \
  --argjson largestCachedFileBytes "${largest_cache_bytes}" \
  --arg largestCachedFileName "${largest_cache_name}" \
  --argjson installedPackages "${package_count}" \
  '{status:"passed",maxPackageBytes:$maxPackageBytes,oversizedFiles:$oversizedFiles,largestCachedFile:{name:$largestCachedFileName,bytes:$largestCachedFileBytes},installedPackages:$installedPackages}' \
  >"${ARTIFACT_DIR}/dependency-audit.json"

requirements_sha="$(sha256sum "${DEPLOY_DIR}/requirements-v2.2.1.lock" | awk '{print $1}')"
python_version="$("${VENV_PYTHON}" --version 2>&1)"
uv_version="$(uv --version)"
postgres_version="$(PGPASSWORD="${GPUSTACK_CI_DATABASE_PASSWORD}" psql "${GPUSTACK_DATABASE_URL}" -Atqc 'show server_version')"
jq -n \
  --arg collectedAt "$(utc_now)" \
  --arg version "${GPUSTACK_VERSION}" \
  --arg tag "${GPUSTACK_TAG}" \
  --arg commit "${GPUSTACK_COMMIT}" \
  --arg wheelSHA256 "${GPUSTACK_WHEEL_SHA256}" \
  --arg upstreamLockSHA256 "${GPUSTACK_UPSTREAM_LOCK_SHA256}" \
  --arg requirementsSHA256 "${requirements_sha}" \
  --arg python "${python_version}" \
  --arg uv "${uv_version}" \
  --arg postgres "${postgres_version}" \
  --arg runnerOS "${ImageOS:-ubuntu24}" \
  --arg runnerImageVersion "${ImageVersion:-unknown}" \
  '{status:"passed",collectedAt:$collectedAt,gpustack:{version:$version,tag:$tag,commit:$commit,wheelSHA256:$wheelSHA256,upstreamLockSHA256:$upstreamLockSHA256,requirementsSHA256:$requirementsSHA256},runtime:{python:$python,uv:$uv,postgres:$postgres,runnerOS:$runnerOS,runnerImageVersion:$runnerImageVersion},profile:{gateway:"disabled",builtinObservability:false,embeddedWorker:false,updateCheck:false,externalDatabase:true}}' \
  >"${ARTIFACT_DIR}/provenance.json"

export GPUSTACK_DATA_DIR="${DATA_DIR}"
export GPUSTACK_CACHE_DIR="${CACHE_DIR}"
export GPUSTACK_GATEWAY_MODE=disabled
export GPUSTACK_DISABLE_BUILTIN_OBSERVABILITY=true
export GPUSTACK_DISABLE_UPDATE_CHECK=true
export GPUSTACK_ENABLE_WORKER=false
export GPUSTACK_API_PORT="${API_PORT}"
export GPUSTACK_SERVER_EXTERNAL_URL="${BASE_URL}"

start_server() {
  "${VENV_GPUSTACK}" start &
  SERVER_PID=$!
  SERVER_START_COUNT=$((SERVER_START_COUNT + 1))
}

wait_for_ready() {
  for _ in $(seq 1 180); do
    if ! kill -0 "${SERVER_PID}" >/dev/null 2>&1; then
      echo "GPUStack Server exited before readiness" >&2
      return 1
    fi
    if curl --fail --silent --max-time 2 "${BASE_URL}/readyz" | jq -e '. == "ok"' >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "GPUStack Server readiness timed out" >&2
  return 1
}

login_admin() {
  rm -f "${COOKIE_JAR}"
  curl --fail-with-body --silent --show-error \
    --cookie-jar "${COOKIE_JAR}" \
    --data-urlencode 'username=admin' \
    --data-urlencode "password=${GPUSTACK_BOOTSTRAP_PASSWORD}" \
    "${BASE_URL}/auth/login" --output /dev/null
  test -s "${COOKIE_JAR}"
}

fetch_authenticated_json() {
  local path="$1"
  local output_file="$2"
  curl --fail-with-body --silent --show-error \
    --cookie "${COOKIE_JAR}" \
    "${BASE_URL}${path}" --output "${output_file}"
  jq empty "${output_file}"
}

summarize_json() {
  local input_file="$1"
  local output_file="$2"
  jq '
    {
      topLevelKeys:(if type == "object" then (keys | sort) else [] end),
      itemCount:(
        if type == "array" then length
        elif (.items? | type) == "array" then (.items | length)
        elif (.data? | type) == "array" then (.data | length)
        else null
        end
      )
    }
  ' "${input_file}" >"${output_file}"
}

start_server
wait_for_ready

health="$(curl --fail --silent "${BASE_URL}/healthz" | jq -r '.')"
ready="$(curl --fail --silent "${BASE_URL}/readyz" | jq -r '.')"
curl --fail --silent "${BASE_URL}/version" --output "${RUNNER_TEMP}/gpustack-version.json"
runtime_version="$(jq -r '.version' "${RUNNER_TEMP}/gpustack-version.json")"
runtime_git_commit="$(jq -r '.git_commit // ""' "${RUNNER_TEMP}/gpustack-version.json")"
[[ "${health}" == "ok" ]]
[[ "${ready}" == "ok" ]]
[[ "${runtime_version}" == "${GPUSTACK_VERSION}" || "${runtime_version}" == "${GPUSTACK_TAG}" ]]
jq -n \
  --arg healthz "${health}" \
  --arg readyz "${ready}" \
  --arg version "${runtime_version}" \
  --arg gitCommit "${runtime_git_commit}" \
  '{status:"passed",healthz:$healthz,readyz:$readyz,version:$version,gitCommit:$gitCommit}' \
  >"${ARTIFACT_DIR}/probe-results.json"

curl --fail --silent "${BASE_URL}/openapi.json" --output "${RUNNER_TEMP}/gpustack-openapi.json"
jq -e '
  def hasmethod($path; $method): ((.paths[$path] // {})[$method] != null);
  hasmethod("/v2/clusters"; "get") and
  hasmethod("/v2/clusters"; "post") and
  hasmethod("/v2/clusters/{id}/registration-token"; "get") and
  hasmethod("/v2/gpu-instances"; "get") and
  hasmethod("/v2/gpu-instances"; "post") and
  hasmethod("/v2/gpu-instances/{id}"; "delete") and
  hasmethod("/v2/gpu-instances/{id}/stop"; "put") and
  hasmethod("/v2/gpu-instances/{id}/start"; "put") and
  hasmethod("/v2/gpu-instance-ssh-public-keys"; "get") and
  hasmethod("/v2/gpu-instance-ssh-public-keys"; "post") and
  hasmethod("/v2/gpu-instance-persistent-volumes"; "get") and
  hasmethod("/v2/gpu-instance-persistent-volumes"; "post") and
  hasmethod("/v2/usage/meta"; "get")
' "${RUNNER_TEMP}/gpustack-openapi.json" >/dev/null
jq '
  def methods($path): ((.paths[$path] // {}) | keys | map(select(. != "parameters")) | sort);
  {
    status:"passed",
    gs04:{clusters:methods("/v2/clusters"),registrationToken:methods("/v2/clusters/{id}/registration-token")},
    gs07:{instances:methods("/v2/gpu-instances"),instance:methods("/v2/gpu-instances/{id}"),stop:methods("/v2/gpu-instances/{id}/stop"),start:methods("/v2/gpu-instances/{id}/start")},
    gs08:{sshPublicKeys:methods("/v2/gpu-instance-ssh-public-keys")},
    gs09:{persistentVolumes:methods("/v2/gpu-instance-persistent-volumes")},
    gs10:{usageMeta:methods("/v2/usage/meta")}
  }
' "${RUNNER_TEMP}/gpustack-openapi.json" >"${ARTIFACT_DIR}/api-surface.json"

login_admin
fetch_authenticated_json '/v2/users/me' "${RUNNER_TEMP}/user-before.json"
fetch_authenticated_json '/v2/clusters' "${RUNNER_TEMP}/clusters.json"
fetch_authenticated_json '/v2/gpu-instances' "${RUNNER_TEMP}/gpu-instances.json"
fetch_authenticated_json '/v2/gpu-instance-ssh-public-keys' "${RUNNER_TEMP}/ssh-public-keys.json"
fetch_authenticated_json '/v2/gpu-instance-persistent-volumes' "${RUNNER_TEMP}/persistent-volumes.json"
fetch_authenticated_json '/v2/usage/meta' "${RUNNER_TEMP}/usage-meta.json"

jq '{id,name,isAdmin:(.isAdmin // .is_admin // false),isActive:(.isActive // .is_active // false)}' \
  "${RUNNER_TEMP}/user-before.json" >"${ARTIFACT_DIR}/user-before-restart.json"
summarize_json "${RUNNER_TEMP}/clusters.json" "${RUNNER_TEMP}/clusters-summary.json"
summarize_json "${RUNNER_TEMP}/gpu-instances.json" "${RUNNER_TEMP}/gpu-instances-summary.json"
summarize_json "${RUNNER_TEMP}/ssh-public-keys.json" "${RUNNER_TEMP}/ssh-public-keys-summary.json"
summarize_json "${RUNNER_TEMP}/persistent-volumes.json" "${RUNNER_TEMP}/persistent-volumes-summary.json"
summarize_json "${RUNNER_TEMP}/usage-meta.json" "${RUNNER_TEMP}/usage-meta-summary.json"
jq -s \
  '{status:"passed",clusters:.[0],gpuInstances:.[1],sshPublicKeys:.[2],persistentVolumes:.[3],usageMeta:.[4]}' \
  "${RUNNER_TEMP}/clusters-summary.json" \
  "${RUNNER_TEMP}/gpu-instances-summary.json" \
  "${RUNNER_TEMP}/ssh-public-keys-summary.json" \
  "${RUNNER_TEMP}/persistent-volumes-summary.json" \
  "${RUNNER_TEMP}/usage-meta-summary.json" \
  >"${ARTIFACT_DIR}/collection-summary.json"

stop_server
start_server
wait_for_ready
login_admin
fetch_authenticated_json '/v2/users/me' "${RUNNER_TEMP}/user-after.json"
jq '{id,name,isAdmin:(.isAdmin // .is_admin // false),isActive:(.isActive // .is_active // false)}' \
  "${RUNNER_TEMP}/user-after.json" >"${ARTIFACT_DIR}/user-after-restart.json"

before_id="$(jq -r '.id' "${ARTIFACT_DIR}/user-before-restart.json")"
after_id="$(jq -r '.id' "${ARTIFACT_DIR}/user-after-restart.json")"
before_name="$(jq -r '.name' "${ARTIFACT_DIR}/user-before-restart.json")"
after_name="$(jq -r '.name' "${ARTIFACT_DIR}/user-after-restart.json")"
[[ -n "${before_id}" && "${before_id}" != "null" && "${before_id}" == "${after_id}" ]]
[[ "${before_name}" == "admin" && "${after_name}" == "admin" ]]
jq -n \
  --arg beforeUserID "${before_id}" \
  --arg afterUserID "${after_id}" \
  --argjson userIDStable true \
  --argjson loginAfterRestart true \
  --argjson readyAfterRestart true \
  '{status:"passed",beforeUserID:$beforeUserID,afterUserID:$afterUserID,userIDStable:$userIDStable,loginAfterRestart:$loginAfterRestart,readyAfterRestart:$readyAfterRestart}' \
  >"${ARTIFACT_DIR}/persistence-validation.json"

jq -n '
  {
    status:"passed",
    checks:{
      releaseProvenance:"passed",
      dependencySize:"passed",
      healthAndReadiness:"passed",
      authentication:"passed",
      apiSurface:"passed",
      collectionAccess:"passed",
      restartPersistence:"passed"
    }
  }
' >"${ARTIFACT_DIR}/baseline-assertions.json"

echo "GPUStack ${GPUSTACK_TAG} server baseline passed"
