#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${DEPLOY_ROOT}/../.." && pwd)"
# shellcheck source=../versions.env
source "${DEPLOY_ROOT}/versions.env"
# shellcheck source=../../ocm/versions.env
source "${REPO_ROOT}/deploy/ocm/versions.env"

ARTIFACT_DIR="${ARTIFACT_DIR:-${REPO_ROOT}/artifacts/observability-baseline}"
CLUSTER_NAME="gpu-observability"
CONTROL_NAMESPACE="gpu-control-plane-system"
OBSERVABILITY_NAMESPACE="gpu-observability-system"
CONTROL_PLANE_IMAGE="gpu-control-plane:observability-ci"
AUDIT_BIN="${RUNNER_TEMP}/control-plane-audit-archive"
PORT_FILE="${RUNNER_TEMP}/audit-s3-port"
FIXTURE_PID=""
PROMETHEUS_FORWARD_PID=""
ALERTMANAGER_FORWARD_PID=""
OTEL_FORWARD_PID=""
CLEANUP_COMPLETE=0

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "required command is unavailable: $1" >&2
    exit 1
  fi
}

stop_pid() {
  local pid="$1"
  if [[ -n "${pid}" ]] && kill -0 "${pid}" >/dev/null 2>&1; then
    kill "${pid}" >/dev/null 2>&1 || true
    wait "${pid}" >/dev/null 2>&1 || true
  fi
}

cleanup() {
  local exit_code="$1"
  set +e
  stop_pid "${PROMETHEUS_FORWARD_PID}"
  stop_pid "${ALERTMANAGER_FORWARD_PID}"
  stop_pid "${OTEL_FORWARD_PID}"
  stop_pid "${FIXTURE_PID}"
  if [[ "${CLEANUP_COMPLETE}" -eq 0 ]] && kind get clusters 2>/dev/null | grep -Fxq "${CLUSTER_NAME}"; then
    kind delete cluster --name "${CLUSTER_NAME}" >/dev/null 2>&1 || true
  fi
  exit "${exit_code}"
}
trap 'cleanup $?' EXIT

wait_for_file() {
  local path="$1"
  for _ in $(seq 1 60); do
    if [[ -s "${path}" ]]; then
      return 0
    fi
    sleep 0.25
  done
  echo "timed out waiting for file: ${path}" >&2
  return 1
}

wait_for_http() {
  local url="$1"
  for _ in $(seq 1 120); do
    if curl --fail --silent --show-error --max-time 2 "${url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "timed out waiting for HTTP endpoint: ${url}" >&2
  return 1
}

wait_for_prometheus_targets() {
  for _ in $(seq 1 120); do
    if curl --fail --silent --show-error http://127.0.0.1:19090/api/v1/targets | jq -e '
      [.data.activeTargets[] | select(.labels.job == "gpu-control-plane" or .labels.job == "otel-collector")] as $targets |
      ($targets | length) == 2 and all($targets[]; .health == "up")
    ' >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "timed out waiting for Prometheus targets" >&2
  return 1
}

for command_name in curl docker go helm jq kind kubectl psql python3 sha256sum; do
  require_command "${command_name}"
done

mkdir -p "${ARTIFACT_DIR}"
rm -f "${PORT_FILE}"

echo "building control-plane audit command and container"
(
  cd "${REPO_ROOT}/apps/control-plane"
  go build -o "${AUDIT_BIN}" ./cmd/audit-archive
  go run ./cmd/migrate up
)
docker build --file "${REPO_ROOT}/infra/control-plane.Dockerfile" --tag "${CONTROL_PLANE_IMAGE}" "${REPO_ROOT}"

psql --set ON_ERROR_STOP=1 "${DATABASE_URL}" <<'SQL'
DELETE FROM audit_events WHERE actor_id = 'observability-ci';
INSERT INTO audit_events (
  id, occurred_at, actor_type, actor_id, scope_type, action,
  resource_type, resource_id, request_id, source_ip, user_agent, outcome, details
) VALUES (
  '00000000-0000-4000-8000-0000000000a1',
  '2026-01-15T12:00:00Z',
  'service-account',
  'observability-ci',
  'system',
  'audit.archive.verify',
  'audit-event',
  '00000000-0000-4000-8000-0000000000a1',
  'observability-ci-request',
  '192.0.2.10',
  'github-actions',
  'succeeded',
  '{"fixture":"observability-baseline"}'::jsonb
);
SQL

python3 "${SCRIPT_DIR}/s3-fixture.py" \
  --report "${ARTIFACT_DIR}/audit-fixture.json" \
  --body "${ARTIFACT_DIR}/audit-object.jsonl" \
  --port-file "${PORT_FILE}" &
FIXTURE_PID=$!
wait_for_file "${PORT_FILE}"
fixture_port="$(<"${PORT_FILE}")"

run_archive() {
  local output_file="$1"
  env \
    DATABASE_URL="${DATABASE_URL}" \
    AUDIT_ARCHIVE_S3_ENDPOINT="http://127.0.0.1:${fixture_port}" \
    AUDIT_ARCHIVE_S3_ACCESS_KEY="${AUDIT_ARCHIVE_S3_ACCESS_KEY}" \
    AUDIT_ARCHIVE_S3_SECRET_KEY="${AUDIT_ARCHIVE_S3_SECRET_KEY}" \
    AUDIT_ARCHIVE_S3_BUCKET="audit" \
    AUDIT_ARCHIVE_S3_REGION="us-east-1" \
    AUDIT_ARCHIVE_PREFIX="platform" \
    AUDIT_ARCHIVE_MONTH="2026-01" \
    AUDIT_ARCHIVE_RETENTION_MODE="GOVERNANCE" \
    AUDIT_ARCHIVE_RETENTION_DAYS="365" \
    AUDIT_ARCHIVE_TIMEOUT="2m" \
    TMPDIR="${RUNNER_TEMP}" \
    "${AUDIT_BIN}" | jq -s 'last' >"${output_file}"
}

run_archive "${ARTIFACT_DIR}/audit-first.json"
wait_for_file "${ARTIFACT_DIR}/audit-fixture.json"
run_archive "${ARTIFACT_DIR}/audit-second.json"

jq -s -e 'length == 1 and .[0].actorId == "observability-ci" and .[0].action == "audit.archive.verify"' \
  "${ARTIFACT_DIR}/audit-object.jsonl" >/dev/null
jq -n \
  --slurpfile first "${ARTIFACT_DIR}/audit-first.json" \
  --slurpfile second "${ARTIFACT_DIR}/audit-second.json" \
  --slurpfile fixture "${ARTIFACT_DIR}/audit-fixture.json" '
    {
      firstStatus:$first[0].status,
      secondStatus:$second[0].status,
      rows:$first[0].rows,
      objectKey:$first[0].objectKey,
      sha256Matches:($first[0].sha256 == $fixture[0].sha256),
      objectLock:$fixture[0].retentionMode,
      putCount:$fixture[0].putCount,
      authorizationV4:$fixture[0].authorizationV4,
      streamingSigV4:$fixture[0].streamingSigV4
    }
    | .status = (if .firstStatus == "uploaded" and .secondStatus == "existing" and .rows == 1 and .sha256Matches and .objectLock == "GOVERNANCE" and .putCount == 1 and .authorizationV4 and .streamingSigV4 then "passed" else "failed" end)
  ' >"${ARTIFACT_DIR}/audit-validation.json"
jq -e '.status == "passed"' "${ARTIFACT_DIR}/audit-validation.json" >/dev/null

echo "creating Kubernetes observability baseline"
kind create cluster --name "${CLUSTER_NAME}" --image "${KIND_NODE_IMAGE}" --wait 120s
kind load docker-image --name "${CLUSTER_NAME}" "${CONTROL_PLANE_IMAGE}"
gateway_ip="$(docker inspect "${CLUSTER_NAME}-control-plane" --format '{{(index .NetworkSettings.Networks "kind").Gateway}}')"
if [[ ! "${gateway_ip}" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
  echo "kind host gateway is not a valid IPv4 address: ${gateway_ip}" >&2
  exit 1
fi

kubectl create namespace "${CONTROL_NAMESPACE}"
kubectl -n "${CONTROL_NAMESPACE}" apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: runner-postgres
spec:
  ports:
    - name: postgres
      port: 5432
      targetPort: 5432
---
apiVersion: discovery.k8s.io/v1
kind: EndpointSlice
metadata:
  name: runner-postgres
  labels:
    kubernetes.io/service-name: runner-postgres
addressType: IPv4
ports:
  - name: postgres
    port: 5432
    protocol: TCP
endpoints:
  - addresses:
      - ${gateway_ip}
EOF
kubectl -n "${CONTROL_NAMESPACE}" create secret generic gpu-control-plane-database \
  --from-literal="DATABASE_URL=postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@runner-postgres:5432/${POSTGRES_DATABASE}?sslmode=disable"

helm upgrade --install gpu-control-plane "${REPO_ROOT}/charts/gpu-control-plane" \
  --namespace "${CONTROL_NAMESPACE}" \
  --set-string image.repository=gpu-control-plane \
  --set-string image.tag=observability-ci \
  --set-string image.pullPolicy=Never \
  --set-string database.existingSecret=gpu-control-plane-database \
  --set-string database.secretKey=DATABASE_URL \
  --set-string database.secretRevision=observability-ci \
  --set-string config.version=observability-ci \
  --set-string config.commit="${GITHUB_SHA}"

kubectl create namespace "${OBSERVABILITY_NAMESPACE}"
helm upgrade --install gpu-observability "${REPO_ROOT}/charts/gpu-observability" \
  --namespace "${OBSERVABILITY_NAMESPACE}"

kubectl -n "${CONTROL_NAMESPACE}" rollout status deployment/gpu-control-plane --timeout=180s
kubectl -n "${OBSERVABILITY_NAMESPACE}" rollout status deployment/gpu-observability-prometheus --timeout=180s
kubectl -n "${OBSERVABILITY_NAMESPACE}" rollout status deployment/gpu-observability-alertmanager --timeout=180s
kubectl -n "${OBSERVABILITY_NAMESPACE}" rollout status deployment/gpu-observability-otel-collector --timeout=180s

kubectl -n "${OBSERVABILITY_NAMESPACE}" port-forward service/gpu-observability-prometheus 19090:9090 >"${RUNNER_TEMP}/prometheus-forward.log" 2>&1 &
PROMETHEUS_FORWARD_PID=$!
kubectl -n "${OBSERVABILITY_NAMESPACE}" port-forward service/gpu-observability-alertmanager 19093:9093 >"${RUNNER_TEMP}/alertmanager-forward.log" 2>&1 &
ALERTMANAGER_FORWARD_PID=$!
kubectl -n "${OBSERVABILITY_NAMESPACE}" port-forward service/gpu-observability-otel-collector 14318:4318 18888:8888 13133:13133 >"${RUNNER_TEMP}/otel-forward.log" 2>&1 &
OTEL_FORWARD_PID=$!

wait_for_http http://127.0.0.1:19090/-/ready
wait_for_http http://127.0.0.1:19093/-/ready
wait_for_http http://127.0.0.1:13133/
wait_for_http http://127.0.0.1:18888/metrics
wait_for_prometheus_targets

prometheus_ready="$(curl --fail --silent --show-error http://127.0.0.1:19090/-/ready)"
jq -n --arg body "${prometheus_ready}" '{status:"passed",body:$body}' >"${ARTIFACT_DIR}/prometheus-ready.json"
curl --fail --silent --show-error http://127.0.0.1:19090/api/v1/targets | jq '
  {
    status:.status,
    activeTargets:[.data.activeTargets[] | select(.labels.job == "gpu-control-plane" or .labels.job == "otel-collector") | {job:.labels.job,health,lastError}]
  }
' >"${ARTIFACT_DIR}/prometheus-targets.json"
curl --fail --silent --show-error http://127.0.0.1:19090/api/v1/rules | jq '
  {
    status:.status,
    groups:[.data.groups[] | {name,rules:[.rules[] | {name,type,health}]}]
  }
' >"${ARTIFACT_DIR}/prometheus-rules.json"
curl --fail --silent --show-error http://127.0.0.1:19093/api/v2/status | jq '
  {status:"passed",versionInfo,clusterStatus:{status:.clusterStatus.status,peerCount:(.clusterStatus.peers | length)}}
' >"${ARTIFACT_DIR}/alertmanager-status.json"
otel_health="$(curl --fail --silent --show-error http://127.0.0.1:13133/)"
jq -n --arg body "${otel_health}" '{status:"passed",body:$body}' >"${ARTIFACT_DIR}/otel-health.json"

otlp_response="${RUNNER_TEMP}/otel-otlp-response.json"
otlp_http_code="$(curl --silent --show-error --output "${otlp_response}" --write-out '%{http_code}' \
  --header 'Content-Type: application/json' \
  --data-binary '{"resourceMetrics":[{"scopeMetrics":[{"scope":{"name":"gpu-observability-ci"},"metrics":[{"name":"gpu_observability_ci_metric","gauge":{"dataPoints":[{"asInt":"1","timeUnixNano":"1784516400000000000"}]}}]}]}]}' \
  http://127.0.0.1:14318/v1/metrics)"
jq -n --arg httpCode "${otlp_http_code}" --rawfile response "${otlp_response}" \
  '{status:(if $httpCode == "200" then "passed" else "failed" end),httpCode:$httpCode,response:$response}' \
  >"${ARTIFACT_DIR}/otel-otlp.json"
jq -e '.status == "passed"' "${ARTIFACT_DIR}/otel-otlp.json" >/dev/null
curl --fail --silent --show-error http://127.0.0.1:18888/metrics >"${ARTIFACT_DIR}/otel-metrics.txt"
grep -Eq '^otelcol_' "${ARTIFACT_DIR}/otel-metrics.txt"

kubectl get pods -A -o json | jq \
  --arg controlNamespace "${CONTROL_NAMESPACE}" \
  --arg observabilityNamespace "${OBSERVABILITY_NAMESPACE}" '
    {
      status:"passed",
      pods:[
        .items[]
        | select(.metadata.namespace == $controlNamespace or .metadata.namespace == $observabilityNamespace)
        | {
            namespace:.metadata.namespace,
            name:.metadata.name,
            component:.metadata.labels["app.kubernetes.io/component"],
            phase:.status.phase,
            ready:any(.status.conditions[]?; .type == "Ready" and .status == "True"),
            images:[.spec.containers[].image],
            serviceAccountTokenDisabled:(.spec.automountServiceAccountToken == false),
            runAsNonRoot:(.spec.securityContext.runAsNonRoot == true),
            secureContainers:all(.spec.containers[]; .securityContext.allowPrivilegeEscalation == false and .securityContext.readOnlyRootFilesystem == true)
          }
      ]
    }
  ' >"${ARTIFACT_DIR}/pods.json"
kubectl get deployments -A -o json | jq \
  --arg controlNamespace "${CONTROL_NAMESPACE}" \
  --arg observabilityNamespace "${OBSERVABILITY_NAMESPACE}" '
    {
      status:"passed",
      deployments:[
        .items[]
        | select(.metadata.namespace == $controlNamespace or .metadata.namespace == $observabilityNamespace)
        | {namespace:.metadata.namespace,name:.metadata.name,desired:.spec.replicas,ready:(.status.readyReplicas // 0),available:(.status.availableReplicas // 0)}
      ]
    }
  ' >"${ARTIFACT_DIR}/deployments.json"

{
  printf 'kind='; kind version
  printf 'kubectl='; kubectl version --client -o json | jq -c '{gitVersion:.clientVersion.gitVersion}'
  printf 'helm='; helm version --short
  printf 'go='; go version
  printf 'prometheus=%s@%s\n' "${PROMETHEUS_VERSION}" "${PROMETHEUS_DIGEST}"
  printf 'alertmanager=%s@%s\n' "${ALERTMANAGER_VERSION}" "${ALERTMANAGER_DIGEST}"
  printf 'otel-collector=%s@%s\n' "${OTEL_COLLECTOR_VERSION}" "${OTEL_COLLECTOR_DIGEST}"
} >"${ARTIFACT_DIR}/tool-versions.txt"

jq -n \
  --arg kubernetes "${KUBERNETES_VERSION}" \
  --arg cluster "${CLUSTER_NAME}" \
  --arg controlNamespace "${CONTROL_NAMESPACE}" \
  --arg observabilityNamespace "${OBSERVABILITY_NAMESPACE}" \
  --arg collectedAt "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg prometheus "${PROMETHEUS_VERSION}" \
  --arg alertmanager "${ALERTMANAGER_VERSION}" \
  --arg otelCollector "${OTEL_COLLECTOR_VERSION}" \
  '{stage:"runtime-verified",cluster:$cluster,kubernetes:$kubernetes,namespaces:[$controlNamespace,$observabilityNamespace],versions:{prometheus:$prometheus,alertmanager:$alertmanager,otelCollector:$otelCollector},collectedAt:$collectedAt}' \
  >"${ARTIFACT_DIR}/collection.json"

jq -n \
  --slurpfile audit "${ARTIFACT_DIR}/audit-validation.json" \
  --slurpfile targets "${ARTIFACT_DIR}/prometheus-targets.json" \
  --slurpfile rules "${ARTIFACT_DIR}/prometheus-rules.json" \
  --slurpfile alertmanager "${ARTIFACT_DIR}/alertmanager-status.json" \
  --slurpfile otlp "${ARTIFACT_DIR}/otel-otlp.json" \
  --slurpfile pods "${ARTIFACT_DIR}/pods.json" \
  --slurpfile deployments "${ARTIFACT_DIR}/deployments.json" '
    {
      checks:{
        auditArchive:$audit[0].status,
        prometheusTargets:(if $targets[0].status == "success" and ($targets[0].activeTargets | length) == 2 and all($targets[0].activeTargets[]; .health == "up" and .lastError == "") then "passed" else "failed" end),
        alertRules:(if ([ $rules[0].groups[].rules[].name ] | sort) == (["GPUControlPlaneMetricsUnavailable","GPUControlPlaneRecoveredPanic","GPUPlatformOTelCollectorUnavailable"] | sort) and all($rules[0].groups[].rules[]; .health == "ok") then "passed" else "failed" end),
        alertmanager:$alertmanager[0].status,
        otlp:$otlp[0].status,
        podSecurity:(if ($pods[0].pods | length) >= 6 and all($pods[0].pods[]; .phase == "Running" and .ready and .serviceAccountTokenDisabled and .runAsNonRoot and .secureContainers) then "passed" else "failed" end),
        deployments:(if ($deployments[0].deployments | length) == 4 and all($deployments[0].deployments[]; .ready == .desired and .available == .desired) then "passed" else "failed" end)
      }
    }
    | .status = (if all(.checks[]; . == "passed") then "passed" else "failed" end)
  ' >"${ARTIFACT_DIR}/baseline-assertions.json"
jq -e '.status == "passed"' "${ARTIFACT_DIR}/baseline-assertions.json" >/dev/null

stop_pid "${PROMETHEUS_FORWARD_PID}"
PROMETHEUS_FORWARD_PID=""
stop_pid "${ALERTMANAGER_FORWARD_PID}"
ALERTMANAGER_FORWARD_PID=""
stop_pid "${OTEL_FORWARD_PID}"
OTEL_FORWARD_PID=""
stop_pid "${FIXTURE_PID}"
FIXTURE_PID=""
kind delete cluster --name "${CLUSTER_NAME}"
cluster_removed=true
if kind get clusters 2>/dev/null | grep -Fxq "${CLUSTER_NAME}"; then
  cluster_removed=false
fi
jq -n --arg completedAt "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --argjson clusterRemoved "${cluster_removed}" \
  '{status:(if $clusterRemoved then "passed" else "failed" end),completedAt:$completedAt,clusterRemoved:$clusterRemoved}' \
  >"${ARTIFACT_DIR}/cleanup-summary.json"
jq -e '.status == "passed"' "${ARTIFACT_DIR}/cleanup-summary.json" >/dev/null

CLEANUP_COMPLETE=1
echo "observability and audit archive baseline passed"
