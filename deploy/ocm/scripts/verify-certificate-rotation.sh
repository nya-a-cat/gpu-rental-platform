#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_command base64
require_command cut
require_command date
require_command docker
require_command jq
require_command kubectl
require_command mktemp
require_command openssl
require_command sed
require_command sha256sum

readonly ROTATION_TRIGGER_REMAINING_SECONDS=120
readonly MINIMUM_CERTIFICATE_OVERLAP_SECONDS=60
readonly POST_EXPIRY_GRACE_SECONDS=30
readonly ROTATION_ANNOTATION="gpu-cloud.nyaacat.dev/rotation-check"
readonly ROTATION_APPROVAL_REASON="AutoApprovedByHubCSRApprovingController"
readonly ROTATION_SUMMARY="${ARTIFACT_DIR}/certificate-rotation-summary.txt"

mkdir -p "${ARTIFACT_DIR}" "${TOOLS_ROOT}"
umask 077
credential_root="$(mktemp -d "${TOOLS_ROOT}/certificate-rotation.XXXXXX")"
trap 'rm -rf -- "${credential_root}"' EXIT

assert_equal() {
  local description="$1"
  local expected="$2"
  local actual="$3"

  if [[ "${actual}" != "${expected}" ]]; then
    echo "${description}: expected ${expected}, got ${actual}" >&2
    return 1
  fi
}

secret_has_client_material() {
  local namespace="$1"
  local secret_name="$2"

  kubectl --context "${SPOKE_CONTEXT}" -n "${namespace}" \
    get secret "${secret_name}" -o json | jq -e '
      . as $secret
      | all(["kubeconfig", "tls.crt", "tls.key"][];
          (($secret.data[.] // "") | length) > 0
        )
    ' >/dev/null
}

secret_data() {
  local namespace="$1"
  local secret_name="$2"
  local key="$3"

  kubectl --context "${SPOKE_CONTEXT}" -n "${namespace}" \
    get secret "${secret_name}" -o json | jq -er --arg key "${key}" '.data[$key]'
}

secret_certificate() {
  secret_data "$1" "$2" tls.crt | base64 --decode
}

secret_key_fingerprint() {
  secret_data "$1" "$2" tls.key | base64 --decode | sha256sum | cut -d ' ' -f 1
}

csr_certificate() {
  kubectl --context "${HUB_CONTEXT}" get certificatesigningrequest "$1" \
    -o jsonpath='{.status.certificate}' | base64 --decode
}

certificate_fingerprint() {
  openssl x509 -outform DER | sha256sum | cut -d ' ' -f 1
}

certificate_serial() {
  openssl x509 -noout -serial | cut -d = -f 2-
}

certificate_subject() {
  openssl x509 -noout -subject -nameopt RFC2253 | sed 's/^subject=//'
}

certificate_not_before() {
  openssl x509 -noout -startdate | cut -d = -f 2-
}

certificate_not_after() {
  openssl x509 -noout -enddate | cut -d = -f 2-
}

certificate_not_after_epoch() {
  date -u -d "$1" +%s
}

secret_uid() {
  kubectl --context "${SPOKE_CONTEXT}" -n "$1" get secret "$2" \
    -o jsonpath='{.metadata.uid}'
}

secret_resource_version() {
  kubectl --context "${SPOKE_CONTEXT}" -n "$1" get secret "$2" \
    -o jsonpath='{.metadata.resourceVersion}'
}

latest_issued_csr() {
  local addon_name="$1"

  kubectl --context "${HUB_CONTEXT}" get certificatesigningrequests -o json | jq -er \
    --arg cluster "${MANAGED_CLUSTER_NAME}" \
    --arg addon "${addon_name}" '
      [
        .items[]
        | select(.metadata.labels["open-cluster-management.io/cluster-name"] == $cluster)
        | select(.spec.signerName == "kubernetes.io/kube-apiserver-client")
        | select(
            if $addon == "" then
              ((.metadata.labels["open-cluster-management.io/addon-name"] // "") == "")
            else
              .metadata.labels["open-cluster-management.io/addon-name"] == $addon
            end
          )
        | select(any(.status.conditions[]?; .type == "Approved" and .status == "True"))
        | select(((.status.certificate // "") | length) > 0)
      ]
      | sort_by(.metadata.creationTimestamp, .metadata.name)
      | (last // empty)
      | .metadata.name
    '
}

issued_csr_uids() {
  local addon_name="$1"

  kubectl --context "${HUB_CONTEXT}" get certificatesigningrequests -o json | jq -c \
    --arg cluster "${MANAGED_CLUSTER_NAME}" \
    --arg addon "${addon_name}" '
      [
        .items[]
        | select(.metadata.labels["open-cluster-management.io/cluster-name"] == $cluster)
        | select(.spec.signerName == "kubernetes.io/kube-apiserver-client")
        | select(
            if $addon == "" then
              ((.metadata.labels["open-cluster-management.io/addon-name"] // "") == "")
            else
              .metadata.labels["open-cluster-management.io/addon-name"] == $addon
            end
          )
        | .metadata.uid
      ]
    '
}

latest_rotated_csr() {
  local addon_name="$1"
  local baseline_uids="$2"

  kubectl --context "${HUB_CONTEXT}" get certificatesigningrequests -o json | jq -r \
    --arg cluster "${MANAGED_CLUSTER_NAME}" \
    --arg addon "${addon_name}" \
    --argjson baseline "${baseline_uids}" \
    --arg reason "${ROTATION_APPROVAL_REASON}" '
      [
        .items[]
        | select(.metadata.uid as $uid | ($baseline | index($uid)) == null)
        | select(
            if $addon == "" then
              (.metadata.name | startswith($cluster + "-"))
            else
              (.metadata.name | startswith("addon-" + $cluster + "-" + $addon + "-"))
            end
          )
        | select(.metadata.labels["open-cluster-management.io/cluster-name"] == $cluster)
        | select(.spec.signerName == "kubernetes.io/kube-apiserver-client")
        | select(
            if $addon == "" then
              ((.metadata.labels["open-cluster-management.io/addon-name"] // "") == "")
            else
              .metadata.labels["open-cluster-management.io/addon-name"] == $addon
            end
          )
        | select(any(.status.conditions[]?;
            .type == "Approved" and
            .status == "True" and
            .reason == $reason
          ))
        | select(((.status.certificate // "") | length) > 0)
      ]
      | sort_by(.metadata.creationTimestamp, .metadata.name)
      | (last // empty)
      | .metadata.name // ""
    '
}

rotated_csr_exists() {
  [[ -n "$(latest_rotated_csr "$1" "$2")" ]]
}

epoch_reached() {
  (( $(date -u +%s) >= $1 ))
}

hub_apiserver_container_id() {
  docker exec "${HUB_CLUSTER_NAME}-control-plane" \
    crictl ps --name kube-apiserver -q | sed -n '1p'
}

hub_apiserver_restarted_since() {
  local previous="$1"
  local current

  current="$(hub_apiserver_container_id 2>/dev/null || true)"
  [[ -n "${current}" && "${current}" != "${previous}" ]]
}

hub_api_is_ready() {
  kubectl --context "${HUB_CONTEXT}" --request-timeout=2s \
    get --raw=/readyz >/dev/null 2>&1
}

secret_fingerprint_changed() {
  local namespace="$1"
  local secret_name="$2"
  local previous="$3"
  local current

  current="$(secret_certificate "${namespace}" "${secret_name}" | certificate_fingerprint)"
  [[ -n "${current}" && "${current}" != "${previous}" ]]
}

pod_identity() {
  local namespace="$1"
  local name_prefix="$2"

  kubectl --context "${SPOKE_CONTEXT}" -n "${namespace}" get pods -o json | jq -er \
    --arg prefix "${name_prefix}" '
      [
        .items[]
        | select(.metadata.deletionTimestamp == null)
        | select(.metadata.name | startswith($prefix))
      ] as $pods
      | select(($pods | length) == 1)
      | $pods[0]
      | [
          .metadata.name,
          .metadata.uid,
          ([.status.containerStatuses[]?.restartCount] | add // 0)
        ]
      | @tsv
    '
}

inventory_observed_at() {
  kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" \
    get configmap gpu-platform-inventory -o json | jq -er \
    '.data["inventory.json"] | fromjson | .observedAt'
}

inventory_observed_since() {
  local previous="$1"
  local current

  current="$(inventory_observed_at 2>/dev/null || true)"
  [[ -n "${current}" && "${current}" != "${previous}" ]]
}

managed_cluster_rotation_condition() {
  kubectl --context "${HUB_CONTEXT}" get managedcluster "${MANAGED_CLUSTER_NAME}" \
    -o json | jq -r '
      [.status.conditions[]?
        | select(
            .type == "ClusterCertificateRotated" and
            .status == "True" and
            .reason == "ClientCertificateUpdated"
          )
      ]
      | (last // empty)
      | [.status, .reason, .message, .lastTransitionTime]
      | @tsv
    ' 2>/dev/null || true
}

addon_rotation_condition() {
  kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" \
    get managedclusteraddon "${ADDON_NAME}" -o json | jq -r '
      [.status.conditions[]?
        | select(
            .type == "ClusterCertificateRotated" and
            .status == "True" and
            .reason == "ClientCertificateUpdated"
          )
      ]
      | (last // empty)
      | [.status, .reason, .message, .lastTransitionTime]
      | @tsv
    ' 2>/dev/null || true
}

rotation_condition_is_true() {
  [[ -n "$("$1")" ]]
}

materialize_credential() {
  local namespace="$1"
  local secret_name="$2"
  local destination="$3"
  local key

  mkdir -p "${destination}"
  for key in kubeconfig tls.crt tls.key; do
    secret_data "${namespace}" "${secret_name}" "${key}" | base64 --decode \
      >"${destination}/${key}"
    chmod 600 "${destination}/${key}"
  done
}

ROTATED_CSR_NAME=""
ROTATED_CSR_UID=""
ROTATED_CERTIFICATE_FINGERPRINT=""
ROTATION_OBSERVED_EPOCH=""

rotate_certificate() {
  local label="$1"
  local namespace="$2"
  local secret_name="$3"
  local addon_name="$4"
  local initial_csr="$5"
  local csr_baseline_uids="$6"
  local old_secret_uid="$7"
  local old_resource_version="$8"
  local old_key_fingerprint="$9"
  local old_fingerprint="${10}"
  local old_serial="${11}"
  local old_subject="${12}"
  local old_not_after="${13}"
  local old_not_after_epoch="${14}"
  local trigger_epoch
  local new_csr
  local new_secret_uid
  local new_resource_version
  local new_key_fingerprint
  local new_fingerprint
  local new_serial
  local new_subject
  local new_not_after
  local new_not_after_epoch
  local csr_fingerprint
  local overlap_seconds

  trigger_epoch=$((old_not_after_epoch - ROTATION_TRIGGER_REMAINING_SECONDS))
  wait_until "${label} certificate rotation threshold" epoch_reached "${trigger_epoch}"

  kubectl --context "${SPOKE_CONTEXT}" -n "${namespace}" annotate secret "${secret_name}" \
    "${ROTATION_ANNOTATION}=$(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite >/dev/null

  wait_until "new approved ${label} CSR" rotated_csr_exists "${addon_name}" "${csr_baseline_uids}"
  new_csr="$(latest_rotated_csr "${addon_name}" "${csr_baseline_uids}")"
  wait_until "updated ${label} client certificate" secret_fingerprint_changed \
    "${namespace}" "${secret_name}" "${old_fingerprint}"

  new_secret_uid="$(secret_uid "${namespace}" "${secret_name}")"
  new_resource_version="$(secret_resource_version "${namespace}" "${secret_name}")"
  new_key_fingerprint="$(secret_key_fingerprint "${namespace}" "${secret_name}")"
  new_fingerprint="$(secret_certificate "${namespace}" "${secret_name}" | certificate_fingerprint)"
  new_serial="$(secret_certificate "${namespace}" "${secret_name}" | certificate_serial)"
  new_subject="$(secret_certificate "${namespace}" "${secret_name}" | certificate_subject)"
  new_not_after="$(secret_certificate "${namespace}" "${secret_name}" | certificate_not_after)"
  new_not_after_epoch="$(certificate_not_after_epoch "${new_not_after}")"
  csr_fingerprint="$(csr_certificate "${new_csr}" | certificate_fingerprint)"
  ROTATION_OBSERVED_EPOCH="$(date -u +%s)"
  overlap_seconds=$((old_not_after_epoch - ROTATION_OBSERVED_EPOCH))

  assert_equal "${label} Secret UID changed" "${old_secret_uid}" "${new_secret_uid}"
  assert_equal "${label} certificate subject changed" "${old_subject}" "${new_subject}"
  if [[ "${new_resource_version}" == "${old_resource_version}" ]]; then
    echo "${label} Secret resourceVersion did not change" >&2
    return 1
  fi
  if [[ "${new_key_fingerprint}" == "${old_key_fingerprint}" ]]; then
    echo "${label} private key did not rotate" >&2
    return 1
  fi
  assert_equal "${label} CSR and Secret certificate differ" "${csr_fingerprint}" "${new_fingerprint}"

  if [[ "${new_serial}" == "${old_serial}" ]]; then
    echo "${label} certificate serial did not change" >&2
    return 1
  fi
  if (( new_not_after_epoch <= old_not_after_epoch )); then
    echo "${label} certificate expiry did not advance" >&2
    return 1
  fi
  if (( overlap_seconds < MINIMUM_CERTIFICATE_OVERLAP_SECONDS )); then
    echo "${label} certificate overlap is ${overlap_seconds}s; expected at least ${MINIMUM_CERTIFICATE_OVERLAP_SECONDS}s" >&2
    return 1
  fi

  ROTATED_CSR_NAME="${new_csr}"
  ROTATED_CSR_UID="$(kubectl --context "${HUB_CONTEXT}" get certificatesigningrequest "${new_csr}" -o jsonpath='{.metadata.uid}')"
  ROTATED_CERTIFICATE_FINGERPRINT="${new_fingerprint}"

  {
    echo "[${label}]"
    echo "initial_csr=${initial_csr}"
    echo "rotated_csr=${ROTATED_CSR_NAME}"
    echo "rotated_csr_uid=${ROTATED_CSR_UID}"
    echo "secret_uid=${new_secret_uid}"
    echo "old_secret_resource_version=${old_resource_version}"
    echo "new_secret_resource_version=${new_resource_version}"
    echo "private_key_rotated=true"
    echo "old_certificate_sha256=${old_fingerprint}"
    echo "new_certificate_sha256=${new_fingerprint}"
    echo "old_certificate_serial=${old_serial}"
    echo "new_certificate_serial=${new_serial}"
    echo "old_certificate_not_after=${old_not_after}"
    echo "new_certificate_not_after=${new_not_after}"
    echo "rotation_overlap_seconds=${overlap_seconds}"
    echo
  } >>"${ROTATION_SUMMARY}"
}

wait_until "ManagedCluster Hub credential material" secret_has_client_material \
  "${MANAGED_CLUSTER_AGENT_NAMESPACE}" "${MANAGED_CLUSTER_HUB_KUBECONFIG_SECRET}"
wait_until "Add-on Hub credential material" secret_has_client_material \
  "${ADDON_INSTALL_NAMESPACE}" "${ADDON_HUB_KUBECONFIG_SECRET}"

cluster_initial_csr="$(latest_issued_csr "")"
addon_initial_csr="$(latest_issued_csr "${ADDON_NAME}")"
cluster_csr_baseline_uids="$(issued_csr_uids "")"
addon_csr_baseline_uids="$(issued_csr_uids "${ADDON_NAME}")"
cluster_secret_uid="$(secret_uid "${MANAGED_CLUSTER_AGENT_NAMESPACE}" "${MANAGED_CLUSTER_HUB_KUBECONFIG_SECRET}")"
addon_secret_uid="$(secret_uid "${ADDON_INSTALL_NAMESPACE}" "${ADDON_HUB_KUBECONFIG_SECRET}")"
cluster_secret_resource_version="$(secret_resource_version "${MANAGED_CLUSTER_AGENT_NAMESPACE}" "${MANAGED_CLUSTER_HUB_KUBECONFIG_SECRET}")"
addon_secret_resource_version="$(secret_resource_version "${ADDON_INSTALL_NAMESPACE}" "${ADDON_HUB_KUBECONFIG_SECRET}")"
cluster_old_key_fingerprint="$(secret_key_fingerprint "${MANAGED_CLUSTER_AGENT_NAMESPACE}" "${MANAGED_CLUSTER_HUB_KUBECONFIG_SECRET}")"
addon_old_key_fingerprint="$(secret_key_fingerprint "${ADDON_INSTALL_NAMESPACE}" "${ADDON_HUB_KUBECONFIG_SECRET}")"
cluster_old_fingerprint="$(secret_certificate "${MANAGED_CLUSTER_AGENT_NAMESPACE}" "${MANAGED_CLUSTER_HUB_KUBECONFIG_SECRET}" | certificate_fingerprint)"
addon_old_fingerprint="$(secret_certificate "${ADDON_INSTALL_NAMESPACE}" "${ADDON_HUB_KUBECONFIG_SECRET}" | certificate_fingerprint)"
assert_equal "initial ManagedCluster CSR and Secret certificate differ" \
  "$(csr_certificate "${cluster_initial_csr}" | certificate_fingerprint)" \
  "${cluster_old_fingerprint}"
assert_equal "initial Add-on CSR and Secret certificate differ" \
  "$(csr_certificate "${addon_initial_csr}" | certificate_fingerprint)" \
  "${addon_old_fingerprint}"
cluster_old_serial="$(secret_certificate "${MANAGED_CLUSTER_AGENT_NAMESPACE}" "${MANAGED_CLUSTER_HUB_KUBECONFIG_SECRET}" | certificate_serial)"
addon_old_serial="$(secret_certificate "${ADDON_INSTALL_NAMESPACE}" "${ADDON_HUB_KUBECONFIG_SECRET}" | certificate_serial)"
cluster_old_subject="$(secret_certificate "${MANAGED_CLUSTER_AGENT_NAMESPACE}" "${MANAGED_CLUSTER_HUB_KUBECONFIG_SECRET}" | certificate_subject)"
addon_old_subject="$(secret_certificate "${ADDON_INSTALL_NAMESPACE}" "${ADDON_HUB_KUBECONFIG_SECRET}" | certificate_subject)"
cluster_old_not_before="$(secret_certificate "${MANAGED_CLUSTER_AGENT_NAMESPACE}" "${MANAGED_CLUSTER_HUB_KUBECONFIG_SECRET}" | certificate_not_before)"
addon_old_not_before="$(secret_certificate "${ADDON_INSTALL_NAMESPACE}" "${ADDON_HUB_KUBECONFIG_SECRET}" | certificate_not_before)"
cluster_old_not_after="$(secret_certificate "${MANAGED_CLUSTER_AGENT_NAMESPACE}" "${MANAGED_CLUSTER_HUB_KUBECONFIG_SECRET}" | certificate_not_after)"
addon_old_not_after="$(secret_certificate "${ADDON_INSTALL_NAMESPACE}" "${ADDON_HUB_KUBECONFIG_SECRET}" | certificate_not_after)"
cluster_old_not_after_epoch="$(certificate_not_after_epoch "${cluster_old_not_after}")"
addon_old_not_after_epoch="$(certificate_not_after_epoch "${addon_old_not_after}")"
cluster_pod_before="$(pod_identity "${MANAGED_CLUSTER_AGENT_NAMESPACE}" klusterlet-registration-agent-)"
addon_pod_before="$(pod_identity "${ADDON_INSTALL_NAMESPACE}" gpu-platform-addon-agent-)"

{
  echo "schema_version=1"
  echo "collected_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "hub_cluster_signing_duration=${EFFECTIVE_HUB_CLUSTER_SIGNING_DURATION}"
  echo "managed_cluster_initial_secret_resource_version=${cluster_secret_resource_version}"
  echo "addon_initial_secret_resource_version=${addon_secret_resource_version}"
  echo "managed_cluster_initial_not_before=${cluster_old_not_before}"
  echo "addon_initial_not_before=${addon_old_not_before}"
  echo
} >"${ROTATION_SUMMARY}"

rotate_certificate \
  managed-cluster \
  "${MANAGED_CLUSTER_AGENT_NAMESPACE}" \
  "${MANAGED_CLUSTER_HUB_KUBECONFIG_SECRET}" \
  "" \
  "${cluster_initial_csr}" \
  "${cluster_csr_baseline_uids}" \
  "${cluster_secret_uid}" \
  "${cluster_secret_resource_version}" \
  "${cluster_old_key_fingerprint}" \
  "${cluster_old_fingerprint}" \
  "${cluster_old_serial}" \
  "${cluster_old_subject}" \
  "${cluster_old_not_after}" \
  "${cluster_old_not_after_epoch}"
cluster_rotated_csr="${ROTATED_CSR_NAME}"
cluster_rotated_csr_uid="${ROTATED_CSR_UID}"
cluster_new_fingerprint="${ROTATED_CERTIFICATE_FINGERPRINT}"
cluster_rotation_observed_epoch="${ROTATION_OBSERVED_EPOCH}"

rotate_certificate \
  addon \
  "${ADDON_INSTALL_NAMESPACE}" \
  "${ADDON_HUB_KUBECONFIG_SECRET}" \
  "${ADDON_NAME}" \
  "${addon_initial_csr}" \
  "${addon_csr_baseline_uids}" \
  "${addon_secret_uid}" \
  "${addon_secret_resource_version}" \
  "${addon_old_key_fingerprint}" \
  "${addon_old_fingerprint}" \
  "${addon_old_serial}" \
  "${addon_old_subject}" \
  "${addon_old_not_after}" \
  "${addon_old_not_after_epoch}"
addon_rotated_csr="${ROTATED_CSR_NAME}"
addon_rotated_csr_uid="${ROTATED_CSR_UID}"
addon_new_fingerprint="${ROTATED_CERTIFICATE_FINGERPRINT}"
addon_rotation_observed_epoch="${ROTATION_OBSERVED_EPOCH}"

wait_until "ManagedCluster rotation condition" rotation_condition_is_true \
  managed_cluster_rotation_condition
wait_until "Add-on rotation condition" rotation_condition_is_true \
  addon_rotation_condition

assert_equal "registration agent changed during rotation" \
  "${cluster_pod_before}" \
  "$(pod_identity "${MANAGED_CLUSTER_AGENT_NAMESPACE}" klusterlet-registration-agent-)"
assert_equal "Add-on agent changed during rotation" \
  "${addon_pod_before}" \
  "$(pod_identity "${ADDON_INSTALL_NAMESPACE}" gpu-platform-addon-agent-)"

if (( cluster_old_not_after_epoch > addon_old_not_after_epoch )); then
  post_expiry_epoch=$((cluster_old_not_after_epoch + POST_EXPIRY_GRACE_SECONDS))
else
  post_expiry_epoch=$((addon_old_not_after_epoch + POST_EXPIRY_GRACE_SECONDS))
fi
wait_until "initial client certificates to expire" epoch_reached "${post_expiry_epoch}"

hub_apiserver_before="$(hub_apiserver_container_id)"
if [[ -z "${hub_apiserver_before}" ]]; then
  echo "Hub kube-apiserver container is unavailable" >&2
  exit 1
fi
docker exec "${HUB_CLUSTER_NAME}-control-plane" \
  crictl stop "${hub_apiserver_before}" >/dev/null
wait_until "Hub kube-apiserver container restart" hub_apiserver_restarted_since \
  "${hub_apiserver_before}"
hub_apiserver_after="$(hub_apiserver_container_id)"
wait_until "Hub API recovery" hub_api_is_ready

cluster_lease_reconnect_baseline="$(lease_renew_time "${HUB_CONTEXT}" "${MANAGED_CLUSTER_NAME}" managed-cluster-lease)"
addon_lease_reconnect_baseline="$(lease_renew_time "${SPOKE_CONTEXT}" "${ADDON_INSTALL_NAMESPACE}" "${ADDON_NAME}")"
inventory_reconnect_baseline="$(inventory_observed_at)"

wait_until "ManagedCluster Lease renewal after initial certificate expiry and Hub reconnect" lease_renewed_since \
  "${HUB_CONTEXT}" "${MANAGED_CLUSTER_NAME}" managed-cluster-lease "${cluster_lease_reconnect_baseline}"
wait_until "Add-on Lease renewal after initial certificate expiry and Hub reconnect" lease_renewed_since \
  "${SPOKE_CONTEXT}" "${ADDON_INSTALL_NAMESPACE}" "${ADDON_NAME}" "${addon_lease_reconnect_baseline}"
wait_until "inventory report after initial certificate expiry and Hub reconnect" inventory_observed_since \
  "${inventory_reconnect_baseline}"

cluster_lease_after_reconnect="$(lease_renew_time "${HUB_CONTEXT}" "${MANAGED_CLUSTER_NAME}" managed-cluster-lease)"
addon_lease_after_reconnect="$(lease_renew_time "${SPOKE_CONTEXT}" "${ADDON_INSTALL_NAMESPACE}" "${ADDON_NAME}")"
inventory_after_reconnect="$(inventory_observed_at)"

kubectl --context "${HUB_CONTEXT}" wait \
  --for=condition=ManagedClusterConditionAvailable \
  managedcluster/"${MANAGED_CLUSTER_NAME}" \
  --timeout="${WAIT_TIMEOUT}"
kubectl --context "${HUB_CONTEXT}" -n "${MANAGED_CLUSTER_NAME}" wait \
  --for=condition=Available \
  managedclusteraddon/"${ADDON_NAME}" \
  --timeout="${WAIT_TIMEOUT}"

assert_equal "registration agent changed after initial certificate expiry" \
  "${cluster_pod_before}" \
  "$(pod_identity "${MANAGED_CLUSTER_AGENT_NAMESPACE}" klusterlet-registration-agent-)"
assert_equal "Add-on agent changed after initial certificate expiry" \
  "${addon_pod_before}" \
  "$(pod_identity "${ADDON_INSTALL_NAMESPACE}" gpu-platform-addon-agent-)"

cluster_credential_dir="${credential_root}/managed-cluster"
addon_credential_dir="${credential_root}/addon"
materialize_credential \
  "${MANAGED_CLUSTER_AGENT_NAMESPACE}" \
  "${MANAGED_CLUSTER_HUB_KUBECONFIG_SECRET}" \
  "${cluster_credential_dir}"
materialize_credential \
  "${ADDON_INSTALL_NAMESPACE}" \
  "${ADDON_HUB_KUBECONFIG_SECRET}" \
  "${addon_credential_dir}"

hub_server="$(kubectl --context "${HUB_CONTEXT}" config view --minify -o jsonpath='{.clusters[0].cluster.server}')"
"${KUBECTL_BIN}" --kubeconfig "${cluster_credential_dir}/kubeconfig" --server "${hub_server}" \
  get managedcluster "${MANAGED_CLUSTER_NAME}" >/dev/null
"${KUBECTL_BIN}" --kubeconfig "${addon_credential_dir}/kubeconfig" --server "${hub_server}" \
  -n "${MANAGED_CLUSTER_NAME}" get configmap gpu-platform-inventory >/dev/null

{
  echo "managed_cluster_rotated_csr=${cluster_rotated_csr}"
  echo "managed_cluster_rotated_csr_uid=${cluster_rotated_csr_uid}"
  echo "managed_cluster_new_certificate_sha256=${cluster_new_fingerprint}"
  echo "managed_cluster_rotation_observed_epoch=${cluster_rotation_observed_epoch}"
  echo "addon_rotated_csr=${addon_rotated_csr}"
  echo "addon_rotated_csr_uid=${addon_rotated_csr_uid}"
  echo "addon_new_certificate_sha256=${addon_new_fingerprint}"
  echo "addon_rotation_observed_epoch=${addon_rotation_observed_epoch}"
  echo "managed_cluster_rotation_condition=$(managed_cluster_rotation_condition)"
  echo "addon_rotation_condition=$(addon_rotation_condition)"
  echo "initial_certificates_post_expiry_epoch=${post_expiry_epoch}"
  echo "hub_apiserver_container_before=${hub_apiserver_before}"
  echo "hub_apiserver_container_after=${hub_apiserver_after}"
  echo "managed_cluster_lease_reconnect_baseline=${cluster_lease_reconnect_baseline}"
  echo "managed_cluster_lease_after_reconnect=${cluster_lease_after_reconnect}"
  echo "addon_lease_reconnect_baseline=${addon_lease_reconnect_baseline}"
  echo "addon_lease_after_reconnect=${addon_lease_after_reconnect}"
  echo "inventory_reconnect_baseline=${inventory_reconnect_baseline}"
  echo "inventory_after_reconnect=${inventory_after_reconnect}"
  echo "registration_agent_identity=${cluster_pod_before}"
  echo "addon_agent_identity=${addon_pod_before}"
  echo "post_expiry_verified_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "hub_connection_reset=passed"
  echo "credential_api_checks=passed"
} >>"${ROTATION_SUMMARY}"

echo "ManagedCluster and GPU Platform Add-on client certificate rotation checks passed"