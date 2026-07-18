#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_command docker
require_command helm
require_command kubectl

if ! docker image inspect "${ADDON_IMAGE}" >/dev/null 2>&1; then
  echo "required local image is unavailable: ${ADDON_IMAGE}" >&2
  exit 1
fi

"${KIND_BIN}" load docker-image "${ADDON_IMAGE}" --name "${HUB_CLUSTER_NAME}"
"${KIND_BIN}" load docker-image "${ADDON_IMAGE}" --name "${SPOKE_CLUSTER_NAME}"

helm upgrade --install "${ADDON_HELM_RELEASE}" "${REPO_ROOT}/charts/gpu-platform-addon" \
  --kube-context "${HUB_CONTEXT}" \
  --namespace "${ADDON_MANAGER_NAMESPACE}" \
  --create-namespace \
  --set-string image.repository="${ADDON_IMAGE%:*}" \
  --set-string image.tag="${ADDON_IMAGE##*:}" \
  --wait \
  --timeout "${WAIT_TIMEOUT}"

kubectl --context "${HUB_CONTEXT}" -n "${ADDON_MANAGER_NAMESPACE}" \
  rollout status deployment/"${ADDON_HELM_RELEASE}" --timeout="${WAIT_TIMEOUT}"

kubectl --context "${HUB_CONTEXT}" apply \
  --filename "${DEPLOY_ROOT}/manifests/managed-cluster-addon.yaml"

kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" wait \
  --for=condition=Applied \
  manifestwork/"${ADDON_WORK_NAME}" \
  --timeout="${WAIT_TIMEOUT}"

kubectl --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" \
  rollout status deployment/gpu-platform-addon-agent --timeout="${WAIT_TIMEOUT}"

kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" wait \
  --for=condition=Available \
  managedclusteraddon/"${ADDON_NAME}" \
  --timeout="${WAIT_TIMEOUT}"

wait_until "approved GPU Platform Add-on CSR" addon_csr_is_approved
wait_until "fresh GPU Platform Add-on Lease" addon_lease_is_fresh
