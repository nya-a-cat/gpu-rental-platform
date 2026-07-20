#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
# shellcheck source=../versions.env
source "${DEPLOY_DIR}/versions.env"

required_variables=(
  GPUSTACK_VERSION
  GPUSTACK_TAG
  GPUSTACK_COMMIT
  GPUSTACK_WHEEL_URL
  GPUSTACK_WHEEL_SHA256
  GPUSTACK_UPSTREAM_LOCK_SHA256
  GPUSTACK_REQUIREMENTS_SHA256
  GPUSTACK_PYTHON_VERSION
  GPUSTACK_UV_VERSION
  GPUSTACK_MAX_PACKAGE_BYTES
)

for variable_name in "${required_variables[@]}"; do
  if [[ -z "${!variable_name:-}" ]]; then
    echo "missing GPUStack version variable: ${variable_name}" >&2
    exit 1
  fi
done

[[ "${GPUSTACK_TAG}" == "v${GPUSTACK_VERSION}" ]]
[[ "${GPUSTACK_COMMIT}" =~ ^[0-9a-f]{40}$ ]]
[[ "${GPUSTACK_WHEEL_SHA256}" =~ ^[0-9a-f]{64}$ ]]
[[ "${GPUSTACK_UPSTREAM_LOCK_SHA256}" =~ ^[0-9a-f]{64}$ ]]
[[ "${GPUSTACK_REQUIREMENTS_SHA256}" =~ ^[0-9a-f]{64}$ ]]
[[ "${GPUSTACK_WHEEL_URL}" == "https://github.com/gpustack/gpustack/releases/download/${GPUSTACK_TAG}/gpustack-${GPUSTACK_VERSION}-py3-none-any.whl" ]]
[[ "${GPUSTACK_MAX_PACKAGE_BYTES}" == "104857600" ]]

requirements_file="${DEPLOY_DIR}/requirements-v2.2.1.lock"
actual_requirements_sha="$(sha256sum "${requirements_file}" | awk '{print $1}')"
[[ "${actual_requirements_sha}" == "${GPUSTACK_REQUIREMENTS_SHA256}" ]]

package_count="$(grep -Ec '^[A-Za-z0-9_.-]+==' "${requirements_file}")"
if [[ "${package_count}" -lt 100 ]]; then
  echo "GPUStack dependency lock is unexpectedly small: ${package_count}" >&2
  exit 1
fi

if grep -Eq '^gpustack==' "${requirements_file}"; then
  echo "GPUStack release wheel must be installed separately from the dependency lock" >&2
  exit 1
fi

awk '
  /^[[:space:]]*($|#)/ { next }
  /^[A-Za-z0-9_.-]+==[^[:space:]]+([[:space:]]*;[[:space:]].+)?$/ { next }
  { print "unlocked dependency line: " $0 > "/dev/stderr"; exit 1 }
' "${requirements_file}"

required_dependencies=(
  'fastapi==0.115.14'
  'gpustack-higress-plugins==0.2.3.post5'
  'gpustack-runner==0.1.26.post5'
  'gpustack-runtime==0.2.0.post5'
  'lxml==5.2.1'
  'pyarrow==18.1.0'
  'transformers==5.6.2'
  'xmlsec==1.3.14'
)

for dependency in "${required_dependencies[@]}"; do
  grep -Fxq "${dependency}" "${requirements_file}"
done

echo "GPUStack ${GPUSTACK_TAG} version inputs are consistent (${package_count} locked dependencies)"
