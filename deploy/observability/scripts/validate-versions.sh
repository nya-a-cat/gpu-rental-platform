#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${DEPLOY_ROOT}/../.." && pwd)"
# shellcheck source=../versions.env
source "${DEPLOY_ROOT}/versions.env"

values_file="${REPO_ROOT}/charts/gpu-observability/values.yaml"

require_value() {
  local value="$1"
  if ! grep -Fq -- "${value}" "${values_file}"; then
    echo "observability chart is missing pinned value: ${value}" >&2
    exit 1
  fi
}

require_value "tag: ${PROMETHEUS_VERSION}"
require_value "digest: ${PROMETHEUS_DIGEST}"
require_value "tag: ${ALERTMANAGER_VERSION}"
require_value "digest: ${ALERTMANAGER_DIGEST}"
require_value "tag: ${OTEL_COLLECTOR_VERSION}"
require_value "digest: ${OTEL_COLLECTOR_DIGEST}"

for layer_size in \
  "${PROMETHEUS_AMD64_LARGEST_LAYER_BYTES}" \
  "${ALERTMANAGER_AMD64_LARGEST_LAYER_BYTES}" \
  "${OTEL_COLLECTOR_AMD64_LARGEST_LAYER_BYTES}"; do
  if (( layer_size > MAX_IMAGE_LAYER_BYTES )); then
    echo "observability image layer exceeds ${MAX_IMAGE_LAYER_BYTES} bytes: ${layer_size}" >&2
    exit 1
  fi
done

echo "observability versions and image layer limits are valid"
