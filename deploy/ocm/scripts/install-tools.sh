#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_command curl
require_command install
require_command sha256sum
require_command tar

mkdir -p "${BIN_DIR}" "${TOOLS_ROOT}/downloads"

download_checked() {
  local url="$1"
  local sha256="$2"
  local destination="$3"
  local temporary="${destination}.download"

  if [[ -f "${destination}" ]] && printf '%s  %s\n' "${sha256}" "${destination}" | sha256sum --check --status; then
    echo "using verified download: ${destination}"
    return
  fi

  rm -f "${temporary}"
  curl --fail --location --show-error --silent \
    --retry 5 --retry-delay 2 --retry-all-errors \
    --output "${temporary}" "${url}"
  printf '%s  %s\n' "${sha256}" "${temporary}" | sha256sum --check --status
  mv "${temporary}" "${destination}"
}

kind_download="${TOOLS_ROOT}/downloads/kind-${KIND_VERSION}-linux-amd64"
download_checked "${KIND_URL}" "${KIND_SHA256}" "${kind_download}"
install -m 0755 "${kind_download}" "${KIND_BIN}"

kubectl_download="${TOOLS_ROOT}/downloads/kubectl-${KUBECTL_VERSION}-linux-amd64"
download_checked "${KUBECTL_URL}" "${KUBECTL_SHA256}" "${kubectl_download}"
install -m 0755 "${kubectl_download}" "${KUBECTL_BIN}"

helm_archive="${TOOLS_ROOT}/downloads/helm-${HELM_VERSION}-linux-amd64.tar.gz"
download_checked "${HELM_URL}" "${HELM_SHA256}" "${helm_archive}"
helm_extract_dir="${TOOLS_ROOT}/helm-${HELM_VERSION}"
mkdir -p "${helm_extract_dir}"
tar --extract --gzip --file "${helm_archive}" --directory "${helm_extract_dir}" --strip-components=1 linux-amd64/helm
install -m 0755 "${helm_extract_dir}/helm" "${HELM_BIN}"

clusteradm_archive="${TOOLS_ROOT}/downloads/clusteradm-${CLUSTERADM_VERSION}-linux-amd64.tar.gz"
download_checked "${CLUSTERADM_URL}" "${CLUSTERADM_SHA256}" "${clusteradm_archive}"
tar --extract --gzip --file "${clusteradm_archive}" --directory "${BIN_DIR}" clusteradm
chmod 0755 "${CLUSTERADM_BIN}"

"${KIND_BIN}" version
"${CLUSTERADM_BIN}" version
"${KUBECTL_BIN}" version --client --output=yaml
"${HELM_BIN}" version --short
