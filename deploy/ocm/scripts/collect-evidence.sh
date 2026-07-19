#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_command docker
require_command jq
require_command kubectl

mkdir -p "${ARTIFACT_DIR}"

capture_kube_json() {
  local file_name="$1"
  local context="$2"
  local filter="$3"
  shift 3

  if ! kubectl --context "${context}" "$@" -o json 2>/dev/null | jq "${filter}" >"${ARTIFACT_DIR}/${file_name}"; then
    echo '{"captureStatus":"unavailable"}' >"${ARTIFACT_DIR}/${file_name}"
  fi
}

capture_secret_metadata() {
  local file_name="$1"
  local namespace="$2"
  local secret_name="$3"

  if ! kubectl --context "${SPOKE_CONTEXT}" -n "${namespace}" get secret "${secret_name}" -o json 2>/dev/null | jq '{apiVersion,kind,metadata:{name:.metadata.name,namespace:.metadata.namespace,uid:.metadata.uid,resourceVersion:.metadata.resourceVersion,creationTimestamp:.metadata.creationTimestamp,labels:.metadata.labels},type,dataKeys:((.data // {}) | keys)}' >"${ARTIFACT_DIR}/${file_name}"; then
    echo '{"captureStatus":"unavailable"}' >"${ARTIFACT_DIR}/${file_name}"
  fi
}

capture_log() {
  local file_name="$1"
  shift
  "$@" >"${ARTIFACT_DIR}/${file_name}" 2>&1 || true
}

NODE_FILTER='{items:[.items[]|{metadata:{name:.metadata.name,uid:.metadata.uid,labels:((.metadata.labels // {})|with_entries(select(.key|test("^(kubernetes.io/(arch|os|hostname)|node.kubernetes.io/instance-type|nvidia.com/(gpu.present|gpu.count|gpu.product|gpu.memory|mig.capable|mig.strategy))$"))))},spec:{unschedulable:(.spec.unschedulable // false),taints:(.spec.taints // [])},status:{capacity:.status.capacity,allocatable:.status.allocatable,nodeInfo:{architecture:.status.nodeInfo.architecture,containerRuntimeVersion:.status.nodeInfo.containerRuntimeVersion,kernelVersion:.status.nodeInfo.kernelVersion,kubeletVersion:.status.nodeInfo.kubeletVersion,operatingSystem:.status.nodeInfo.operatingSystem,osImage:.status.nodeInfo.osImage},conditions:.status.conditions}}]}'
POD_FILTER='{items:[.items[]|{metadata:{name:.metadata.name,namespace:.metadata.namespace,uid:.metadata.uid,labels:.metadata.labels},spec:{nodeName:.spec.nodeName,containers:[.spec.containers[]|{name,image}]},status:{phase:.status.phase,conditions:.status.conditions,containerStatuses:[.status.containerStatuses[]?|{name,ready,restartCount,image,imageID,state}]}}]}'
CONDITION_FILTER='{apiVersion,kind,metadata:{name:.metadata.name,namespace:.metadata.namespace,uid:.metadata.uid,generation:.metadata.generation,resourceVersion:.metadata.resourceVersion,creationTimestamp:.metadata.creationTimestamp,labels:.metadata.labels,ownerReferences:.metadata.ownerReferences},status:{conditions:.status.conditions}}'
MANIFESTWORK_FILTER='{items:[.items[]|{metadata:{name:.metadata.name,namespace:.metadata.namespace,uid:.metadata.uid,generation:.metadata.generation,resourceVersion:.metadata.resourceVersion,creationTimestamp:.metadata.creationTimestamp,ownerReferences:.metadata.ownerReferences},status:{conditions:.status.conditions}}]}'
LEASE_FILTER='{items:[.items[]|{metadata:{name:.metadata.name,namespace:.metadata.namespace,uid:.metadata.uid,resourceVersion:.metadata.resourceVersion,creationTimestamp:.metadata.creationTimestamp},spec:{holderIdentity:.spec.holderIdentity,leaseDurationSeconds:.spec.leaseDurationSeconds,acquireTime:.spec.acquireTime,renewTime:.spec.renewTime,leaseTransitions:.spec.leaseTransitions}}]}'
INVENTORY_FILTER='{apiVersion,kind,metadata:{name:.metadata.name,namespace:.metadata.namespace,uid:.metadata.uid,resourceVersion:.metadata.resourceVersion,creationTimestamp:.metadata.creationTimestamp,labels:.metadata.labels,ownerReferences:.metadata.ownerReferences},inventory:(.data["inventory.json"]|fromjson)}'
DEPLOYMENT_FILTER='{apiVersion,kind,metadata:{name:.metadata.name,namespace:.metadata.namespace,uid:.metadata.uid,generation:.metadata.generation,resourceVersion:.metadata.resourceVersion,creationTimestamp:.metadata.creationTimestamp,labels:.metadata.labels},spec:{replicas:.spec.replicas,strategy:.spec.strategy,template:{metadata:{labels:.spec.template.metadata.labels},spec:{serviceAccountName:.spec.template.spec.serviceAccountName,containers:[.spec.template.spec.containers[]|{name,image,imagePullPolicy,args,env}]}}},status:{observedGeneration:.status.observedGeneration,replicas:.status.replicas,readyReplicas:.status.readyReplicas,updatedReplicas:.status.updatedReplicas,availableReplicas:.status.availableReplicas,conditions:.status.conditions}}'

kubectl --context "${HUB_CONTEXT}" -n kube-system get pods -l component=kube-controller-manager -o json 2>/dev/null | jq '{items:[.items[]|{metadata:{name:.metadata.name,uid:.metadata.uid},containers:[.spec.containers[]|{name,image,signingArguments:[((.command // []) + (.args // []))[]?|select(startswith("--cluster-signing-duration="))]}]}]}' >"${ARTIFACT_DIR}/hub-kube-controller-manager.json" || echo '{"captureStatus":"unavailable"}' >"${ARTIFACT_DIR}/hub-kube-controller-manager.json"

{
  echo "collected_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "ocm_version=${OCM_VERSION}"
  echo "kubernetes_version=${KUBERNETES_VERSION}"
  echo "hub_cluster_signing_duration=${EFFECTIVE_HUB_CLUSTER_SIGNING_DURATION}"
  "${KIND_BIN}" version
  "${CLUSTERADM_BIN}" version
  "${KUBECTL_BIN}" version --client --output=json
  "${HELM_BIN}" version --short
  docker version --format 'client={{.Client.Version}} server={{.Server.Version}}'
} >"${ARTIFACT_DIR}/tool-versions.txt" 2>&1 || true

declare -A seen_images=()
images=()
for image in "${ADDON_IMAGE:-}" "${ADDON_MANAGER_IMAGE:-}" "${ADDON_AGENT_IMAGE:-}" "${ADDON_CURRENT_IMAGE:-}" "${ADDON_N_MINUS_ONE_IMAGE:-}"; do
  if [[ -n "${image}" && -z "${seen_images[$image]+x}" ]] && docker image inspect "${image}" >/dev/null 2>&1; then
    seen_images["${image}"]=1
    images+=("${image}")
  fi
done
if (( ${#images[@]} > 0 )); then
  docker image inspect "${images[@]}" | jq '[.[]|{id:.Id,repoTags:.RepoTags,created:.Created,size:.Size,os:.Os,architecture:.Architecture,labels:{source:.Config.Labels["org.opencontainers.image.source"],version:.Config.Labels["org.opencontainers.image.version"],revision:.Config.Labels["org.opencontainers.image.revision"]}}]' >"${ARTIFACT_DIR}/addon-images.json"
else
  echo '[]' >"${ARTIFACT_DIR}/addon-images.json"
fi

capture_kube_json hub-nodes.json "${HUB_CONTEXT}" "${NODE_FILTER}" get nodes
capture_kube_json hub-pods.json "${HUB_CONTEXT}" "${POD_FILTER}" get pods -A
capture_kube_json managed-cluster.json "${HUB_CONTEXT}" "${CONDITION_FILTER}" get managedcluster "${MANAGED_CLUSTER_NAME}"
kubectl --context "${HUB_CONTEXT}" get certificatesigningrequests -o json 2>/dev/null | jq '{apiVersion,kind,items:[.items[]|{metadata:{name:.metadata.name,uid:.metadata.uid,resourceVersion:.metadata.resourceVersion,creationTimestamp:.metadata.creationTimestamp,labels:.metadata.labels},spec:{signerName:.spec.signerName,expirationSeconds:.spec.expirationSeconds,usages:.spec.usages},status:{conditions:.status.conditions}}]}' >"${ARTIFACT_DIR}/certificate-signing-requests.json" || echo '{"captureStatus":"unavailable"}' >"${ARTIFACT_DIR}/certificate-signing-requests.json"
capture_kube_json cluster-management-addon.json "${HUB_CONTEXT}" "${CONDITION_FILTER}" get clustermanagementaddon "${ADDON_NAME}"
capture_kube_json managed-cluster-addon.json "${HUB_CONTEXT}" "${CONDITION_FILTER}" -n "${MANAGED_CLUSTER_NAME}" get managedclusteraddon "${ADDON_NAME}"
capture_kube_json manifestworks.json "${HUB_CONTEXT}" "${MANIFESTWORK_FILTER}" -n "${MANAGED_CLUSTER_NAME}" get manifestworks
capture_kube_json hub-leases.json "${HUB_CONTEXT}" "${LEASE_FILTER}" -n "${MANAGED_CLUSTER_NAME}" get leases
capture_kube_json inventory-configmap.json "${HUB_CONTEXT}" "${INVENTORY_FILTER}" -n "${MANAGED_CLUSTER_NAME}" get configmap gpu-platform-inventory
capture_kube_json spoke-nodes.json "${SPOKE_CONTEXT}" "${NODE_FILTER}" get nodes
capture_kube_json spoke-pods.json "${SPOKE_CONTEXT}" "${POD_FILTER}" get pods -A
capture_kube_json addon-deployment.json "${SPOKE_CONTEXT}" "${DEPLOYMENT_FILTER}" -n "${ADDON_INSTALL_NAMESPACE}" get deployment gpu-platform-addon-agent
capture_kube_json addon-lease.json "${SPOKE_CONTEXT}" '{apiVersion,kind,metadata:{name:.metadata.name,namespace:.metadata.namespace,uid:.metadata.uid,resourceVersion:.metadata.resourceVersion,creationTimestamp:.metadata.creationTimestamp},spec:{holderIdentity:.spec.holderIdentity,leaseDurationSeconds:.spec.leaseDurationSeconds,acquireTime:.spec.acquireTime,renewTime:.spec.renewTime,leaseTransitions:.spec.leaseTransitions}}' -n "${ADDON_INSTALL_NAMESPACE}" get lease "${ADDON_NAME}"
capture_secret_metadata managed-cluster-hub-secret-metadata.json "${MANAGED_CLUSTER_AGENT_NAMESPACE}" "${MANAGED_CLUSTER_HUB_KUBECONFIG_SECRET}"
capture_secret_metadata addon-hub-secret-metadata.json "${ADDON_INSTALL_NAMESPACE}" "${ADDON_HUB_KUBECONFIG_SECRET}"
capture_log addon-manager.log kubectl --context "${HUB_CONTEXT}" -n "${ADDON_MANAGER_NAMESPACE}" logs deployment/"${ADDON_HELM_RELEASE}" --all-containers --tail=500
capture_log addon-agent.log kubectl --context "${SPOKE_CONTEXT}" -n "${ADDON_INSTALL_NAMESPACE}" logs deployment/gpu-platform-addon-agent --all-containers --tail=500
capture_kube_json manifestwork-smoke-configmap.json "${SPOKE_CONTEXT}" '{apiVersion,kind,metadata:{name:.metadata.name,namespace:.metadata.namespace,uid:.metadata.uid,resourceVersion:.metadata.resourceVersion,creationTimestamp:.metadata.creationTimestamp},data}' -n default get configmap gpu-platform-ocm-smoke
