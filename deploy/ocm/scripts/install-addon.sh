#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_command docker
require_command helm
require_command jq
require_command kubectl

case "${ADDON_MANAGER_SUPPORTS_UID_ENV}" in
  0|1) ;;
  *)
    echo "ADDON_MANAGER_SUPPORTS_UID_ENV must be 0 or 1" >&2
    exit 1
    ;;
esac

for image in "${ADDON_MANAGER_IMAGE}" "${ADDON_AGENT_IMAGE}"; do
  if ! docker image inspect "${image}" >/dev/null 2>&1; then
    echo "required local image is unavailable: ${image}" >&2
    exit 1
  fi
done

"${KIND_BIN}" load docker-image "${ADDON_MANAGER_IMAGE}" --name "${HUB_CLUSTER_NAME}"
"${KIND_BIN}" load docker-image "${ADDON_AGENT_IMAGE}" --name "${SPOKE_CLUSTER_NAME}"

manager_repository="${ADDON_MANAGER_IMAGE%:*}"
manager_tag="${ADDON_MANAGER_IMAGE##*:}"
agent_repository="${ADDON_AGENT_IMAGE%:*}"
agent_tag="${ADDON_AGENT_IMAGE##*:}"

addon_manifestwork_is_desired() {
  local addon_uid
  addon_uid="$(kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get managedclusteraddon "${ADDON_NAME}" -o jsonpath="{.metadata.uid}" 2>/dev/null || true)"
  [[ -n "${addon_uid}" ]] || return 1

  kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get manifestwork "${ADDON_WORK_NAME}" -o json 2>/dev/null | jq -e --arg image "${ADDON_AGENT_IMAGE}" --arg addon_uid "${addon_uid}" --argjson expects_uid "${ADDON_MANAGER_SUPPORTS_UID_ENV}" "[.spec.workload.manifests[] | select(.kind == \"Deployment\" and .metadata.name == \"gpu-platform-addon-agent\") | .spec.template.spec.containers[] | select(.name == \"agent\" and .image == \$image) | if \$expects_uid == 1 then select(any(.env[]?; .name == \"GPU_PLATFORM_ADDON_UID\" and .value == \$addon_uid)) else select(all(.env[]?; .name != \"GPU_PLATFORM_ADDON_UID\")) end] | length == 1" >/dev/null
}

addon_deployment_is_desired() {
  local addon_uid
  addon_uid="$(kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get managedclusteraddon "${ADDON_NAME}" -o jsonpath="{.metadata.uid}" 2>/dev/null || true)"
  [[ -n "${addon_uid}" ]] || return 1

  kubectl --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" get deployment gpu-platform-addon-agent -o json 2>/dev/null | jq -e --arg image "${ADDON_AGENT_IMAGE}" --arg addon_uid "${addon_uid}" --argjson expects_uid "${ADDON_MANAGER_SUPPORTS_UID_ENV}" "[.spec.template.spec.containers[] | select(.name == \"agent\" and .image == \$image) | if \$expects_uid == 1 then select(any(.env[]?; .name == \"GPU_PLATFORM_ADDON_UID\" and .value == \$addon_uid)) else select(all(.env[]?; .name != \"GPU_PLATFORM_ADDON_UID\")) end] | length == 1" >/dev/null
}

helm_args=(
  upgrade
  --install "${ADDON_HELM_RELEASE}"
  "${REPO_ROOT}/charts/gpu-platform-addon"
  --kube-context "${HUB_CONTEXT}"
  --namespace "${ADDON_MANAGER_NAMESPACE}"
  --create-namespace
  --set-string image.repository="${manager_repository}"
  --set-string image.tag="${manager_tag}"
  --set-string agent.image.repository="${agent_repository}"
  --set-string agent.image.tag="${agent_tag}"
  --wait
  --timeout "${WAIT_TIMEOUT}"
)
helm "${helm_args[@]}"

kubectl --context "${HUB_CONTEXT}" -n "${ADDON_MANAGER_NAMESPACE}" rollout status deployment/"${ADDON_HELM_RELEASE}" --timeout="${WAIT_TIMEOUT}"
kubectl --context "${HUB_CONTEXT}" apply --filename "${DEPLOY_ROOT}/manifests/managed-cluster-addon.yaml"

wait_until "GPU Platform Add-on ManifestWork" addon_manifestwork_exists
wait_until "GPU Platform Add-on desired ManifestWork" addon_manifestwork_is_desired
wait_until "GPU Platform Add-on desired agent Deployment" addon_deployment_is_desired

kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" wait --for=condition=Applied manifestwork/"${ADDON_WORK_NAME}" --timeout="${WAIT_TIMEOUT}"
kubectl --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" rollout status deployment/gpu-platform-addon-agent --timeout="${WAIT_TIMEOUT}"
kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" wait --for=condition=Available managedclusteraddon/"${ADDON_NAME}" --timeout="${WAIT_TIMEOUT}"

wait_until "approved GPU Platform Add-on CSR" addon_csr_is_approved
wait_until "fresh GPU Platform Add-on Lease" addon_lease_is_fresh
