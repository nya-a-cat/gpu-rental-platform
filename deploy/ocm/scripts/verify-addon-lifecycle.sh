#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_command helm
require_command jq
require_command kubectl

if [[ -z "${ADDON_CURRENT_IMAGE:-}" || -z "${ADDON_N_MINUS_ONE_IMAGE:-}" ]]; then
  echo "ADDON_CURRENT_IMAGE and ADDON_N_MINUS_ONE_IMAGE are required" >&2
  exit 1
fi

LIFECYCLE_DIR="${ARTIFACT_DIR}/lifecycle"
mkdir -p "${LIFECYCLE_DIR}"

assert_equal() {
  local description="$1"
  local expected="$2"
  local actual="$3"

  if [[ "${actual}" != "${expected}" ]]; then
    echo "${description}: expected ${expected}, got ${actual}" >&2
    return 1
  fi
}

assert_not_equal() {
  local description="$1"
  local previous="$2"
  local current="$3"

  if [[ -z "${current}" || "${current}" == "${previous}" ]]; then
    echo "${description}: expected a changed non-empty value, got ${current:-<empty>}" >&2
    return 1
  fi
}

resource_is_absent() {
  local output
  if ! output="$(kubectl "$@" --ignore-not-found -o name 2>/dev/null)"; then
    return 1
  fi
  [[ -z "${output}" ]]
}

ready_pod() {
  local context="$1"
  local namespace="$2"
  local selector="$3"
  kubectl --context "${context}" -n "${namespace}" get pods -l "${selector}" -o json | jq -er '[.items[] | select(.metadata.deletionTimestamp == null) | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))] | if length == 1 then .[0] else error("expected exactly one non-terminating Ready Pod") end'
}

manager_ready_pod() {
  ready_pod "${HUB_CONTEXT}" "${ADDON_MANAGER_NAMESPACE}" app.kubernetes.io/component=hub-manager
}

agent_ready_pod() {
  ready_pod "${SPOKE_CONTEXT}" "${ADDON_INSTALL_NAMESPACE}" app.kubernetes.io/component=agent
}

manager_pod_uid() { manager_ready_pod | jq -er '.metadata.uid'; }
manager_image_id() { manager_ready_pod | jq -er '.status.containerStatuses[0].imageID'; }
agent_pod_uid() { agent_ready_pod | jq -er '.metadata.uid'; }
agent_image_id() { agent_ready_pod | jq -er '.status.containerStatuses[0].imageID'; }

manager_image() {
  kubectl --context "${HUB_CONTEXT}" -n "${ADDON_MANAGER_NAMESPACE}" get deployment "${ADDON_HELM_RELEASE}" -o jsonpath='{.spec.template.spec.containers[0].image}'
}

agent_image() {
  kubectl --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" get deployment gpu-platform-addon-agent -o jsonpath='{.spec.template.spec.containers[0].image}'
}

addon_secret_uid() {
  kubectl --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" get secret "${ADDON_HUB_KUBECONFIG_SECRET}" -o jsonpath='{.metadata.uid}'
}

managed_cluster_addon_uid() {
  kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get managedclusteraddon "${ADDON_NAME}" -o jsonpath='{.metadata.uid}'
}

inventory_observed_at() {
  kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get configmap gpu-platform-inventory -o json | jq -er '.data["inventory.json"] | fromjson | .observedAt'
}

inventory_generation() {
  kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get configmap gpu-platform-inventory -o json | jq -er '.data["inventory.json"] | fromjson | .generation'
}

inventory_owner_uid() {
  kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get configmap gpu-platform-inventory -o jsonpath='{.metadata.ownerReferences[0].uid}'
}

inventory_agent_epoch() {
  kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get configmap gpu-platform-inventory -o json | jq -er '.data["inventory.json"] | fromjson | .agentEpoch'
}

inventory_sequence() {
  kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get configmap gpu-platform-inventory -o json | jq -er '.data["inventory.json"] | fromjson | .sequence'
}

inventory_session_matches() {
  local fencing_enabled="$1"
  local fencing_token="$2"

  kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get configmap gpu-platform-inventory -o json | jq -e \
    --argjson fencing_enabled "${fencing_enabled}" \
    --arg fencing_token "${fencing_token}" '
      .data["inventory.json"]
        | fromjson
        | (.agentEpoch | test("^[a-f0-9]{32}$")) and
          (.sequence | type == "number" and . >= 1) and
          .fencingEnabled == $fencing_enabled and
          (.fencingToken // "") == $fencing_token
    ' >/dev/null
}

addon_csr_uids() {
  kubectl --context "${HUB_CONTEXT}" get certificatesigningrequests -o json | jq -r '[.items[] | select(.metadata.labels["open-cluster-management.io/addon-name"] == "gpu-platform-addon") | .metadata.uid] | sort | join(",")'
}

hub_permission_count() {
  local owner_uid="$1"
  kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get roles,rolebindings -o json | jq -er --arg owner_uid "${owner_uid}" '[.items[] | select(any(.metadata.ownerReferences[]?; .uid == $owner_uid))] | length'
}

hub_owned_permissions() {
  local owner_uid="$1"
  kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get roles,rolebindings -o json | jq -r --arg owner_uid "${owner_uid}" '
.items[] | select(any(.metadata.ownerReferences[]?; .uid == $owner_uid)) | [(.kind | ascii_downcase), .metadata.name] | @tsv
'
}

applied_addon_work_count() {
  kubectl --context "${SPOKE_CONTEXT}" get appliedmanifestworks -o json | jq -er '[.items[] | select(.spec.manifestWorkName == "addon-gpu-platform-addon-deploy-0")] | length'
}

inventory_is_available() {
  [[ -n "$(inventory_observed_at 2>/dev/null || true)" ]]
}

inventory_observed_since() {
  local previous="$1"
  local current
  current="$(inventory_observed_at 2>/dev/null || true)"
  [[ -n "${current}" && "${current}" != "${previous}" ]]
}

wait_for_continuity() {
  local label="$1"
  local lease_before
  local inventory_before

  wait_until "${label} initial inventory report" inventory_is_available
  lease_before="$(lease_renew_time "${SPOKE_CONTEXT}" "${ADDON_INSTALL_NAMESPACE}" "${ADDON_NAME}")"
  inventory_before="$(inventory_observed_at)"
  wait_until "${label} Lease renewal" lease_renewed_since "${SPOKE_CONTEXT}" "${ADDON_INSTALL_NAMESPACE}" "${ADDON_NAME}" "${lease_before}"
  wait_until "${label} inventory report" inventory_observed_since "${inventory_before}"
}

manager_image_is() {
  [[ "$(manager_image 2>/dev/null || true)" == "$1" ]]
}

agent_image_is() {
  [[ "$(agent_image 2>/dev/null || true)" == "$1" ]]
}

verify_expected_images() {
  local manager_expected="$1"
  local agent_expected="$2"

  wait_until "expected manager image ${manager_expected}" manager_image_is "${manager_expected}"
  wait_until "expected agent image ${agent_expected}" agent_image_is "${agent_expected}"
  kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" wait --for=condition=Available managedclusteraddon/"${ADDON_NAME}" --timeout="${WAIT_TIMEOUT}"
}

capture_stage() {
  local stage="$1"
  local destination="${LIFECYCLE_DIR}/${stage}.txt"

  {
    echo "stage=${stage}"
    echo "captured_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "manager_image=$(manager_image 2>/dev/null || true)"
    echo "manager_pod_uid=$(manager_pod_uid 2>/dev/null || true)"
    echo "manager_image_id=$(manager_image_id 2>/dev/null || true)"
    echo "agent_image=$(agent_image 2>/dev/null || true)"
    echo "agent_pod_uid=$(agent_pod_uid 2>/dev/null || true)"
    echo "agent_image_id=$(agent_image_id 2>/dev/null || true)"
    echo "managed_cluster_addon_uid=$(managed_cluster_addon_uid 2>/dev/null || true)"
    echo "addon_secret_uid=$(addon_secret_uid 2>/dev/null || true)"
    echo "addon_lease_renew_time=$(lease_renew_time "${SPOKE_CONTEXT}" "${ADDON_INSTALL_NAMESPACE}" "${ADDON_NAME}")"
    echo "inventory_observed_at=$(inventory_observed_at 2>/dev/null || true)"
    echo "inventory_generation=$(inventory_generation 2>/dev/null || true)"
    echo "inventory_owner_uid=$(inventory_owner_uid 2>/dev/null || true)"
    echo "inventory_agent_epoch=$(inventory_agent_epoch 2>/dev/null || true)"
    echo "inventory_sequence=$(inventory_sequence 2>/dev/null || true)"
    echo "addon_csr_uids=$(addon_csr_uids 2>/dev/null || true)"
    echo "applied_addon_work_count=$(applied_addon_work_count 2>/dev/null || true)"
  } >"${destination}"
}

install_images() {
  local manager="$1"
  local agent="$2"
  local supports_uid_env=0
  if [[ "${manager}" == "${ADDON_CURRENT_IMAGE}" ]]; then
    supports_uid_env=1
  fi
  ADDON_MANAGER_IMAGE="${manager}" ADDON_AGENT_IMAGE="${agent}" ADDON_MANAGER_SUPPORTS_UID_ENV="${supports_uid_env}" bash "${SCRIPT_DIR}/install-addon.sh"
}

assert_upgrade_credentials_stable() {
  assert_equal "add-on Secret UID" "${BASELINE_SECRET_UID}" "$(addon_secret_uid)"
  assert_equal "add-on CSR set" "${BASELINE_CSR_UIDS}" "$(addon_csr_uids)"
  assert_equal "inventory generation" "${BASELINE_INVENTORY_GENERATION}" "$(inventory_generation)"
}

permissions_are_removed() {
  [[ "$(hub_permission_count "$1")" == "0" ]]
}

applied_work_is_removed() {
  [[ "$(applied_addon_work_count)" == "0" ]]
}

inventory_is_owned_by() {
  [[ "$(inventory_owner_uid 2>/dev/null || true)" == "$1" ]]
}

delete_addon_and_verify() {
  local addon_uid="$1"
  local stage="$2"
  local permission_resources="$3"

  kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" delete managedclusteraddon "${ADDON_NAME}" --wait=false
  wait_until "${stage} ManagedClusterAddOn cleanup" resource_is_absent --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get managedclusteraddon "${ADDON_NAME}"
  wait_until "${stage} ManifestWork cleanup" resource_is_absent --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get manifestwork "${ADDON_WORK_NAME}"
  wait_until "${stage} agent Deployment cleanup" resource_is_absent --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" get deployment gpu-platform-addon-agent
  wait_until "${stage} agent ServiceAccount cleanup" resource_is_absent --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" get serviceaccount gpu-platform-addon-agent
  wait_until "${stage} agent Role cleanup" resource_is_absent --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" get role gpu-platform-addon-agent
  wait_until "${stage} agent RoleBinding cleanup" resource_is_absent --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" get rolebinding gpu-platform-addon-agent
  wait_until "${stage} agent ClusterRole cleanup" resource_is_absent --context "${SPOKE_CONTEXT}" get clusterrole gpu-platform-addon-agent
  wait_until "${stage} agent ClusterRoleBinding cleanup" resource_is_absent --context "${SPOKE_CONTEXT}" get clusterrolebinding gpu-platform-addon-agent
  wait_until "${stage} add-on Lease cleanup" resource_is_absent --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" get lease "${ADDON_NAME}"
  wait_until "${stage} add-on Secret cleanup" resource_is_absent --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" get secret "${ADDON_HUB_KUBECONFIG_SECRET}"
  wait_until "${stage} inventory cleanup" resource_is_absent --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get configmap gpu-platform-inventory
  wait_until "${stage} Hub permission cleanup" permissions_are_removed "${addon_uid}"
  while IFS=$'\t' read -r permission_kind permission_name; do
    if [[ -n "${permission_kind}" && -n "${permission_name}" ]]; then
      wait_until "${stage} ${permission_kind}/${permission_name} cleanup" resource_is_absent --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" get "${permission_kind}" "${permission_name}"
    fi
  done <<<"${permission_resources}"
  wait_until "${stage} AppliedManifestWork cleanup" applied_work_is_removed
  kubectl --context "${SPOKE_CONTEXT}" get namespace "${ADDON_INSTALL_NAMESPACE}" >/dev/null
  capture_stage "${stage}"
}

verify_chart_uninstalled() {
  resource_is_absent --context "${HUB_CONTEXT}" -n "${ADDON_MANAGER_NAMESPACE}" get deployment "${ADDON_HELM_RELEASE}" || return 1
  resource_is_absent --context "${HUB_CONTEXT}" -n "${ADDON_MANAGER_NAMESPACE}" get serviceaccount "${ADDON_HELM_RELEASE}" || return 1
  resource_is_absent --context "${HUB_CONTEXT}" get clusterrole "${ADDON_HELM_RELEASE}" || return 1
  resource_is_absent --context "${HUB_CONTEXT}" get clusterrolebinding "${ADDON_HELM_RELEASE}" || return 1
  resource_is_absent --context "${HUB_CONTEXT}" get clustermanagementaddon "${ADDON_NAME}"
}

echo '{"status":"running"}' >"${LIFECYCLE_DIR}/assertions.json"

install_images "${ADDON_N_MINUS_ONE_IMAGE}" "${ADDON_N_MINUS_ONE_IMAGE}"
verify_expected_images "${ADDON_N_MINUS_ONE_IMAGE}" "${ADDON_N_MINUS_ONE_IMAGE}"
wait_for_continuity "N-1 baseline"
BASELINE_MANAGER_POD_UID="$(manager_pod_uid)"
BASELINE_MANAGER_IMAGE_ID="$(manager_image_id)"
BASELINE_AGENT_POD_UID="$(agent_pod_uid)"
BASELINE_AGENT_IMAGE_ID="$(agent_image_id)"
BASELINE_MCA_UID="$(managed_cluster_addon_uid)"
BASELINE_SECRET_UID="$(addon_secret_uid)"
BASELINE_CSR_UIDS="$(addon_csr_uids)"
BASELINE_INVENTORY_GENERATION="$(inventory_generation)"
if [[ -z "${BASELINE_CSR_UIDS}" ]]; then
  echo "N-1 baseline has no approved add-on CSR evidence" >&2
  exit 1
fi
capture_stage 00-n-minus-one-baseline

install_images "${ADDON_N_MINUS_ONE_IMAGE}" "${ADDON_N_MINUS_ONE_IMAGE}"
verify_expected_images "${ADDON_N_MINUS_ONE_IMAGE}" "${ADDON_N_MINUS_ONE_IMAGE}"
wait_for_continuity "idempotent reinstall"
assert_equal "idempotent manager Pod UID" "${BASELINE_MANAGER_POD_UID}" "$(manager_pod_uid)"
assert_equal "idempotent agent Pod UID" "${BASELINE_AGENT_POD_UID}" "$(agent_pod_uid)"
assert_equal "idempotent manager image ID" "${BASELINE_MANAGER_IMAGE_ID}" "$(manager_image_id)"
assert_equal "idempotent agent image ID" "${BASELINE_AGENT_IMAGE_ID}" "$(agent_image_id)"
assert_equal "idempotent ManagedClusterAddOn UID" "${BASELINE_MCA_UID}" "$(managed_cluster_addon_uid)"
assert_upgrade_credentials_stable
capture_stage 05-idempotent-reinstall

previous_agent_uid="$(agent_pod_uid)"
install_images "${ADDON_N_MINUS_ONE_IMAGE}" "${ADDON_CURRENT_IMAGE}"
verify_expected_images "${ADDON_N_MINUS_ONE_IMAGE}" "${ADDON_CURRENT_IMAGE}"
wait_for_continuity "N-1 manager with N agent"
assert_not_equal "N agent Pod UID" "${previous_agent_uid}" "$(agent_pod_uid)"
assert_not_equal "N agent image ID" "${BASELINE_AGENT_IMAGE_ID}" "$(agent_image_id)"
assert_upgrade_credentials_stable
assert_equal "N agent ownership with N-1 manager" "" "$(inventory_owner_uid 2>/dev/null || true)"
wait_until "N agent unfenced session with N-1 manager" inventory_session_matches false ""
N_AGENT_EPOCH_WITH_N_MINUS_ONE_MANAGER="$(inventory_agent_epoch)"
capture_stage 10-n-minus-one-manager-n-agent

previous_agent_uid="$(agent_pod_uid)"
install_images "${ADDON_N_MINUS_ONE_IMAGE}" "${ADDON_N_MINUS_ONE_IMAGE}"
verify_expected_images "${ADDON_N_MINUS_ONE_IMAGE}" "${ADDON_N_MINUS_ONE_IMAGE}"
wait_for_continuity "agent rollback to N-1"
assert_not_equal "rolled-back agent Pod UID" "${previous_agent_uid}" "$(agent_pod_uid)"
assert_equal "rolled-back agent image ID" "${BASELINE_AGENT_IMAGE_ID}" "$(agent_image_id)"
assert_upgrade_credentials_stable
capture_stage 15-agent-rollback

previous_manager_uid="$(manager_pod_uid)"
previous_agent_uid="$(agent_pod_uid)"
install_images "${ADDON_CURRENT_IMAGE}" "${ADDON_N_MINUS_ONE_IMAGE}"
verify_expected_images "${ADDON_CURRENT_IMAGE}" "${ADDON_N_MINUS_ONE_IMAGE}"
wait_for_continuity "N manager with N-1 agent"
assert_not_equal "N manager Pod UID" "${previous_manager_uid}" "$(manager_pod_uid)"
assert_not_equal "N-1 agent rollout under N manager" "${previous_agent_uid}" "$(agent_pod_uid)"
assert_not_equal "N manager image ID" "${BASELINE_MANAGER_IMAGE_ID}" "$(manager_image_id)"
assert_equal "N-1 agent image ID under N manager" "${BASELINE_AGENT_IMAGE_ID}" "$(agent_image_id)"
assert_upgrade_credentials_stable
assert_equal "N-1 agent inventory ownership" "" "$(inventory_owner_uid 2>/dev/null || true)"
capture_stage 20-n-manager-n-minus-one-agent

previous_agent_uid="$(agent_pod_uid)"
install_images "${ADDON_CURRENT_IMAGE}" "${ADDON_CURRENT_IMAGE}"
verify_expected_images "${ADDON_CURRENT_IMAGE}" "${ADDON_CURRENT_IMAGE}"
wait_for_continuity "N manager with N agent"
assert_not_equal "N agent final Pod UID" "${previous_agent_uid}" "$(agent_pod_uid)"
assert_not_equal "N agent final image ID" "${BASELINE_AGENT_IMAGE_ID}" "$(agent_image_id)"
assert_upgrade_credentials_stable
wait_until "inventory ManagedClusterAddOn ownership" inventory_is_owned_by "${BASELINE_MCA_UID}"
wait_until "N agent fenced session with N manager" inventory_session_matches true "${BASELINE_MCA_UID}"
assert_not_equal "N agent epoch after rollback and rollout" "${N_AGENT_EPOCH_WITH_N_MINUS_ONE_MANAGER}" "$(inventory_agent_epoch)"
capture_stage 30-n-manager-n-agent

BASELINE_PERMISSION_RESOURCES="$(hub_owned_permissions "${BASELINE_MCA_UID}")"
permission_count="$(hub_permission_count "${BASELINE_MCA_UID}")"
if (( permission_count < 2 )); then
  echo "expected Hub Role and RoleBinding owned by ManagedClusterAddOn, got ${permission_count}" >&2
  exit 1
fi
delete_addon_and_verify "${BASELINE_MCA_UID}" 40-post-delete-cleanup "${BASELINE_PERMISSION_RESOURCES}"

install_images "${ADDON_CURRENT_IMAGE}" "${ADDON_CURRENT_IMAGE}"
verify_expected_images "${ADDON_CURRENT_IMAGE}" "${ADDON_CURRENT_IMAGE}"
wait_for_continuity "re-enabled current add-on"
REENABLED_MCA_UID="$(managed_cluster_addon_uid)"
assert_not_equal "re-enabled ManagedClusterAddOn UID" "${BASELINE_MCA_UID}" "${REENABLED_MCA_UID}"
wait_until "re-enabled inventory ownership" inventory_is_owned_by "${REENABLED_MCA_UID}"
wait_until "re-enabled fenced agent session" inventory_session_matches true "${REENABLED_MCA_UID}"
REENABLED_PERMISSION_RESOURCES="$(hub_owned_permissions "${REENABLED_MCA_UID}")"
if (( $(hub_permission_count "${REENABLED_MCA_UID}") < 2 )); then
  echo "re-enabled Add-on Hub permissions are incomplete" >&2
  exit 1
fi
capture_stage 50-reenabled-current

delete_addon_and_verify "${REENABLED_MCA_UID}" 60-pre-uninstall-cleanup "${REENABLED_PERMISSION_RESOURCES}"
helm uninstall "${ADDON_HELM_RELEASE}" --kube-context "${HUB_CONTEXT}" --namespace "${ADDON_MANAGER_NAMESPACE}" --wait --timeout "${WAIT_TIMEOUT}"
wait_until "Helm chart cleanup" verify_chart_uninstalled
capture_stage 70-post-uninstall-cleanup

install_images "${ADDON_CURRENT_IMAGE}" "${ADDON_CURRENT_IMAGE}"
bash "${SCRIPT_DIR}/verify.sh"
FINAL_MCA_UID="$(managed_cluster_addon_uid)"
assert_not_equal "final reinstall ManagedClusterAddOn UID" "${REENABLED_MCA_UID}" "${FINAL_MCA_UID}"
wait_until "final inventory ownership" inventory_is_owned_by "${FINAL_MCA_UID}"
wait_for_continuity "final reinstall"
capture_stage 80-final-reinstall

jq -n --arg current "${ADDON_CURRENT_IMAGE}" --arg n_minus_one "${ADDON_N_MINUS_ONE_IMAGE}" --arg final_uid "${FINAL_MCA_UID}" '{status:"passed",currentImage:$current,nMinusOneImage:$n_minus_one,finalManagedClusterAddOnUID:$final_uid}' >"${LIFECYCLE_DIR}/assertions.json"

echo "GPU Platform Add-on install, upgrade, N/N-1, cleanup, uninstall, and reinstall checks passed"
