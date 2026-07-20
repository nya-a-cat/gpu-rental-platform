#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

stage="${1:-snapshot}"
ensure_artifact_dir

write_unavailable_json() {
  local destination="$1"
  local source_name="$2"
  jq -n --arg source "${source_name}" '{captureStatus:"unavailable", source:$source}' >"${destination}"
}

capture_json() {
  local destination="$1"
  local source_name="$2"
  local filter="$3"
  shift 3

  local temporary="${destination}.tmp"
  if "$@" 2>/dev/null | jq "${filter}" >"${temporary}"; then
    mv "${temporary}" "${destination}"
  else
    rm -f "${temporary}"
    write_unavailable_json "${destination}" "${source_name}"
  fi
}

capture_text() {
  local destination="$1"
  local source_name="$2"
  shift 2

  local temporary="${destination}.tmp"
  if "$@" >"${temporary}" 2>&1; then
    mv "${temporary}" "${destination}"
  else
    {
      echo "capture unavailable: ${source_name}"
      cat "${temporary}" 2>/dev/null || true
    } >"${destination}"
    rm -f "${temporary}"
  fi
}

cat >"${ARTIFACT_DIR}/tool-versions.txt" <<EOF
kind=${KIND_VERSION}
kubernetes=${KUBERNETES_VERSION}
kind-node-image=${KIND_NODE_IMAGE}
kubectl=${KUBECTL_VERSION}
helm=${HELM_VERSION}
control-plane-baseline-image=${CONTROL_PLANE_BASELINE_IMAGE}
control-plane-candidate-image=${CONTROL_PLANE_CANDIDATE_IMAGE}
EOF

jq -n \
  --arg stage "${stage}" \
  --arg capturedAt "$(utc_now)" \
  --arg cluster "${HA_CLUSTER_NAME}" \
  --arg namespace "${HA_NAMESPACE}" \
  --arg release "${HA_RELEASE}" \
  --arg kubernetesVersion "${KUBERNETES_VERSION}" \
  --arg kindVersion "${KIND_VERSION}" \
  --arg kubectlVersion "${KUBECTL_VERSION}" \
  --arg helmVersion "${HELM_VERSION}" \
  '{stage:$stage,capturedAt:$capturedAt,cluster:$cluster,namespace:$namespace,release:$release,versions:{kubernetes:$kubernetesVersion,kind:$kindVersion,kubectl:$kubectlVersion,helm:$helmVersion}}' \
  >"${ARTIFACT_DIR}/collection.json"

capture_json "${ARTIFACT_DIR}/nodes.json" nodes '
  {
    apiVersion,
    kind,
    items: [
      .items[] | {
        metadata: {
          name: .metadata.name,
          labels: (.metadata.labels | with_entries(select(.key == "kubernetes.io/arch" or .key == "kubernetes.io/os" or .key == "node-role.kubernetes.io/control-plane")))
        },
        status: {
          capacity: {cpu: .status.capacity.cpu, memory: .status.capacity.memory, pods: .status.capacity.pods},
          allocatable: {cpu: .status.allocatable.cpu, memory: .status.allocatable.memory, pods: .status.allocatable.pods},
          conditions: [.status.conditions[] | {type,status,reason}],
          runtime: {
            architecture: .status.nodeInfo.architecture,
            operatingSystem: .status.nodeInfo.operatingSystem,
            kubeletVersion: .status.nodeInfo.kubeletVersion,
            containerRuntimeVersion: .status.nodeInfo.containerRuntimeVersion
          }
        }
      }
    ]
  }
' kubectl_ha get nodes -o json

capture_json "${ARTIFACT_DIR}/deployment.json" deployment '
  {
    apiVersion,
    kind,
    metadata: {name:.metadata.name,uid:.metadata.uid,generation:.metadata.generation,labels:.metadata.labels},
    spec: {
      replicas:.spec.replicas,
      strategy:.spec.strategy,
      selector:.spec.selector,
      template: {
        metadata:{labels:.spec.template.metadata.labels},
        spec:{
          serviceAccountName:.spec.template.spec.serviceAccountName,
          automountServiceAccountToken:.spec.template.spec.automountServiceAccountToken,
          securityContext:.spec.template.spec.securityContext,
          terminationGracePeriodSeconds:.spec.template.spec.terminationGracePeriodSeconds,
          containers:[.spec.template.spec.containers[] | {
            name,image,imagePullPolicy,ports,resources,securityContext,readinessProbe,startupProbe,livenessProbe,
            env:[.env[]? | if .valueFrom.secretKeyRef then {name,valueFrom:{secretKeyRef:{name:.valueFrom.secretKeyRef.name,key:.valueFrom.secretKeyRef.key}}} else {name,value} end]
          }]
        }
      }
    },
    status:{replicas:.status.replicas,readyReplicas:.status.readyReplicas,availableReplicas:.status.availableReplicas,updatedReplicas:.status.updatedReplicas,conditions:[.status.conditions[]? | {type,status,reason}]}
  }
' kubectl_ha -n "${HA_NAMESPACE}" get deployment "${CONTROL_PLANE_NAME}" -o json

capture_json "${ARTIFACT_DIR}/service.json" service '
  {
    apiVersion,
    kind,
    metadata:{name:.metadata.name,uid:.metadata.uid,labels:.metadata.labels},
    spec:{type:.spec.type,selector:.spec.selector,ports:.spec.ports},
    status:.status
  }
' kubectl_ha -n "${HA_NAMESPACE}" get service "${CONTROL_PLANE_NAME}" -o json

capture_json "${ARTIFACT_DIR}/service-account.json" service-account '
  {
    apiVersion,
    kind,
    metadata:{name:.metadata.name,uid:.metadata.uid,labels:.metadata.labels},
    automountServiceAccountToken:.automountServiceAccountToken
  }
' kubectl_ha -n "${HA_NAMESPACE}" get serviceaccount "${CONTROL_PLANE_NAME}" -o json

capture_json "${ARTIFACT_DIR}/pdb.json" pdb '
  {
    apiVersion,
    kind,
    metadata:{name:.metadata.name,uid:.metadata.uid,generation:.metadata.generation,labels:.metadata.labels},
    spec:{minAvailable:.spec.minAvailable,selector:.spec.selector},
    status:{currentHealthy:.status.currentHealthy,desiredHealthy:.status.desiredHealthy,disruptionsAllowed:.status.disruptionsAllowed,expectedPods:.status.expectedPods,observedGeneration:.status.observedGeneration,conditions:[.status.conditions[]? | {type,status,reason}]}
  }
' kubectl_ha -n "${HA_NAMESPACE}" get poddisruptionbudget "${CONTROL_PLANE_NAME}" -o json

capture_json "${ARTIFACT_DIR}/pods.json" pods '
  {
    apiVersion,
    kind,
    items:[.items[] | {
      metadata:{name:.metadata.name,uid:.metadata.uid,labels:.metadata.labels},
      status:{phase:.status.phase,conditions:[.status.conditions[]? | {type,status,reason}],containerStatuses:[.status.containerStatuses[]? | {name,ready,restartCount,image,imageID}]}
    }]
  }
' kubectl_ha -n "${HA_NAMESPACE}" get pods -l "${CONTROL_PLANE_SELECTOR}" -o json

capture_json "${ARTIFACT_DIR}/postgres.json" postgres '
  {
    apiVersion,
    kind,
    metadata:{name:.metadata.name,uid:.metadata.uid,generation:.metadata.generation,labels:.metadata.labels},
    spec:{replicas:.spec.replicas,serviceName:.spec.serviceName,containers:[.spec.template.spec.containers[] | {name,image,imagePullPolicy,ports,resources,readinessProbe,livenessProbe}]},
    status:{replicas:.status.replicas,readyReplicas:.status.readyReplicas,currentReplicas:.status.currentReplicas,updatedReplicas:.status.updatedReplicas,currentRevision:.status.currentRevision,updateRevision:.status.updateRevision}
  }
' kubectl_ha -n "${HA_NAMESPACE}" get statefulset "${POSTGRES_NAME}" -o json

capture_json "${ARTIFACT_DIR}/database-secret.json" database-secret '
  {apiVersion,kind,metadata:{name:.metadata.name,uid:.metadata.uid,labels:.metadata.labels},type,dataKeys:((.data // {}) | keys)}
' kubectl_ha -n "${HA_NAMESPACE}" get secret "${DATABASE_SECRET_NAME}" -o json

capture_json "${ARTIFACT_DIR}/bad-database-secret.json" bad-database-secret '
  {apiVersion,kind,metadata:{name:.metadata.name,uid:.metadata.uid,labels:.metadata.labels},type,dataKeys:((.data // {}) | keys)}
' kubectl_ha -n "${HA_NAMESPACE}" get secret "${BAD_DATABASE_SECRET_NAME}" -o json

capture_json "${ARTIFACT_DIR}/migration-job.json" migration-job '
  {
    apiVersion,
    kind,
    metadata:{name:.metadata.name,uid:.metadata.uid,labels:.metadata.labels,annotations:{hook:.metadata.annotations["helm.sh/hook"],hookDeletePolicy:.metadata.annotations["helm.sh/hook-delete-policy"]}},
    spec:{backoffLimit:.spec.backoffLimit,activeDeadlineSeconds:.spec.activeDeadlineSeconds,ttlSecondsAfterFinished:.spec.ttlSecondsAfterFinished,template:{spec:{serviceAccountName:.spec.template.spec.serviceAccountName,automountServiceAccountToken:.spec.template.spec.automountServiceAccountToken,securityContext:.spec.template.spec.securityContext,containers:[.spec.template.spec.containers[] | {name,image,imagePullPolicy,command,args,resources,securityContext,env:[.env[]? | if .valueFrom.secretKeyRef then {name,valueFrom:{secretKeyRef:{name:.valueFrom.secretKeyRef.name,key:.valueFrom.secretKeyRef.key}}} else {name,value} end]}]}}},
    status:{startTime:.status.startTime,completionTime:.status.completionTime,succeeded:.status.succeeded,failed:.status.failed,conditions:[.status.conditions[]? | {type,status,reason}]}
  }
' kubectl_ha -n "${HA_NAMESPACE}" get job "${MIGRATION_JOB_NAME}" -o json

capture_json "${ARTIFACT_DIR}/endpoint-slices.json" endpoint-slices '
  {
    apiVersion,
    kind,
    items:[.items[] | {
      metadata:{name:.metadata.name,uid:.metadata.uid,labels:.metadata.labels},
      addressType,
      ports,
      endpoints:[.endpoints[]? | {conditions,targetRef:(.targetRef | {kind,name,uid})}]
    }]
  }
' kubectl_ha -n "${HA_NAMESPACE}" get endpointslices -l "kubernetes.io/service-name=${CONTROL_PLANE_NAME}" -o json

capture_json "${ARTIFACT_DIR}/helm-release.json" helm-release '
  (map(select(.name == "gpu-control-plane" and .namespace == "gpu-control-plane-system")) | first) as $item |
  if $item == null then {captureStatus:"unavailable",source:"helm-release"}
  else {name:$item.name,namespace:$item.namespace,revision:$item.revision,updated:$item.updated,status:$item.status,chart:$item.chart,appVersion:$item.app_version}
  end
' helm_ha list --namespace "${HA_NAMESPACE}" --all --output json

capture_json "${ARTIFACT_DIR}/events.json" events '
  {
    apiVersion,
    kind,
    items:[.items[] | {
      metadata:{name:.metadata.name,namespace:.metadata.namespace},
      eventTime,
      firstTimestamp,
      lastTimestamp,
      type,
      reason,
      message,
      count,
      regarding:(.regarding // .involvedObject | {kind,name,namespace})
    }]
  }
' kubectl_ha -n "${HA_NAMESPACE}" get events -o json

capture_text "${ARTIFACT_DIR}/control-plane.log" control-plane-logs \
  kubectl_ha -n "${HA_NAMESPACE}" logs deployment/"${CONTROL_PLANE_NAME}" --all-pods=true --prefix=true --tail=500
capture_text "${ARTIFACT_DIR}/migration-job.log" migration-job-logs \
  kubectl_ha -n "${HA_NAMESPACE}" logs job/"${MIGRATION_JOB_NAME}" --prefix=true --tail=500
capture_text "${ARTIFACT_DIR}/postgres.log" postgres-logs \
  kubectl_ha -n "${HA_NAMESPACE}" logs statefulset/"${POSTGRES_NAME}" --prefix=true --tail=500

capture_image() {
  local image_name="$1"
  local destination="$2"
  local source_name="$3"
  local temporary="${destination}.tmp"

  if docker image inspect "${image_name}" 2>/dev/null | jq '
    .[0] | {
      id:.Id,
      repoTags:.RepoTags,
      created:.Created,
      size:.Size,
      os:.Os,
      architecture:.Architecture,
      labels:((.Config.Labels // {}) | with_entries(select(.key | startswith("org.opencontainers.image."))))
    }
  ' >"${temporary}"; then
    mv "${temporary}" "${destination}"
  else
    rm -f "${temporary}"
    write_unavailable_json "${destination}" "${source_name}"
  fi
}

capture_image "${CONTROL_PLANE_BASELINE_IMAGE}" "${ARTIFACT_DIR}/baseline-image.json" baseline-image
capture_image "${CONTROL_PLANE_CANDIDATE_IMAGE}" "${ARTIFACT_DIR}/image.json" candidate-image
