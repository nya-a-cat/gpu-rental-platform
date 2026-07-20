# GPU Control Plane Helm Chart

This Helm v3 chart installs the Go GPU Container Cloud control plane on
Kubernetes 1.34. It deploys three replicas by default, a `ClusterIP` Service, a
dedicated ServiceAccount, and a `policy/v1` PodDisruptionBudget. OCM integration
is disabled by default. When enabled, the chart mounts the ServiceAccount token
and grants only `get`, `create`, `update` and `patch` on ManifestWork resources.

## Prerequisites

- Kubernetes 1.34.x
- Helm 3
- A reachable PostgreSQL database
- A namespace-local Secret containing the complete PostgreSQL connection URL
- A published control-plane image built from `infra/control-plane.Dockerfile`

The database URL is only read through `secretKeyRef`. It is never accepted as a
plain Helm value.
The chart does not install PostgreSQL or create the database Secret. The cloud
provider or platform operator owns both resources and their lifecycle.

```bash
kubectl create namespace gpu-control-plane-system
kubectl --namespace gpu-control-plane-system create secret generic gpu-control-plane-database \
  --from-literal=DATABASE_URL='postgres://user:password@postgres.example:5432/gpu_cloud?sslmode=require'

kubectl --namespace gpu-control-plane-system create secret generic gpu-control-plane-break-glass \
  --from-literal=BREAK_GLASS_ADMIN_TOKEN='replace-with-at-least-32-random-characters'

helm upgrade --install gpu-control-plane charts/gpu-control-plane \
  --namespace gpu-control-plane-system \
  --set image.repository=registry.example/gpu-cloud-control-plane \
  --set image.tag=0.1.0 \
  --set auth.breakGlass.existingSecret=gpu-control-plane-break-glass \
  --atomic \
  --timeout 11m
```

The Secret must exist before installation because the migration Job runs as a
`pre-install,pre-upgrade` Helm hook. The Job executes
`/usr/local/bin/control-plane-migrate up`. `migration.hookDeletePolicy` and
`migration.ttlSecondsAfterFinished` control hook cleanup and Kubernetes TTL
cleanup respectively.

The `11m` timeout leaves one minute beyond the default ten-minute migration Job
deadline for hook scheduling and the control-plane rollout. `--atomic` waits for
all resources and removes a failed release automatically.

Database migrations must remain compatible with the current and N-1
control-plane images. Helm rollback restores Kubernetes resources and image
configuration; it does not reverse an already committed database migration.

## Runtime configuration

The chart maps the runtime contract from `apps/control-plane/README.md`:

| Value                                            | Environment variable          | Default                                       |
| ------------------------------------------------ | ----------------------------- | --------------------------------------------- |
| `database.existingSecret` / `database.secretKey` | `DATABASE_URL`                | `gpu-control-plane-database` / `DATABASE_URL` |
| `database.secretRevision`                        | Pod rollout annotation        | Empty string                                  |
| `auth.breakGlass.existingSecret`                 | `BREAK_GLASS_ADMIN_TOKEN`     | Empty; authenticated tenancy API disabled     |
| `auth.breakGlass.tokenKey`                       | Secret key                    | `BREAK_GLASS_ADMIN_TOKEN`                     |
| `auth.breakGlass.subject`                        | `BREAK_GLASS_ADMIN_SUBJECT`   | `break-glass-admin`                           |
| `auth.breakGlass.secretRevision`                 | Pod rollout annotation        | Empty string                                  |
| `config.version`                                 | `CONTROL_PLANE_VERSION`       | Chart app version                             |
| `config.commit`                                  | `CONTROL_PLANE_COMMIT`        | `unknown`                                     |
| `config.shutdownTimeout`                         | `SHUTDOWN_TIMEOUT`            | `15s`                                         |
| `config.readinessTimeout`                        | `READINESS_TIMEOUT`           | `2s`                                          |
| `config.agentHeartbeatInterval`                  | `AGENT_HEARTBEAT_INTERVAL`    | `15s`                                         |
| `config.agentDegradedAfter`                      | `AGENT_DEGRADED_AFTER`        | `45s`                                         |
| `config.agentOfflineAfter`                       | `AGENT_OFFLINE_AFTER`         | `90s`                                         |
| `config.dbMaxOpenConns`                          | `DB_MAX_OPEN_CONNS`           | `20`                                          |
| `config.dbMaxIdleConns`                          | `DB_MAX_IDLE_CONNS`           | `5`                                           |
| `config.dbConnMaxLifetime`                       | `DB_CONN_MAX_LIFETIME`        | `30m`                                         |
| `config.migrationTimeout`                        | `MIGRATION_TIMEOUT`           | `5m`                                          |
| `config.migrationLockTimeout`                    | `MIGRATION_LOCK_TIMEOUT`      | `30s`                                         |
| `config.migrationStatementTimeout`               | `MIGRATION_STATEMENT_TIMEOUT` | `2m`                                          |
| `ocm.enabled`                                    | `OCM_ENABLED`                 | `false`                                       |
| `ocm.hubURL`                                     | `OCM_HUB_URL`                 | `https://kubernetes.default.svc`              |
| `ocm.defaultClusterID`                           | `OCM_DEFAULT_CLUSTER_ID`      | Empty; required when enabled                  |
| `ocm.addonInstallNamespace`                      | `OCM_ADDON_INSTALL_NAMESPACE` | `open-cluster-management-agent-addon`         |
| `ocm.addonServiceAccount`                        | `OCM_ADDON_SERVICE_ACCOUNT`   | `gpu-platform-addon-agent`                    |
| `ocm.reconcileTimeout`                           | `OCM_RECONCILE_TIMEOUT`       | `2m`                                          |
| `ocm.pollInterval`                               | `OCM_POLL_INTERVAL`           | `2s`                                          |
| `ocm.maxAttempts`                                | `OCM_MAX_ATTEMPTS`            | `8`                                           |

The chart fixes `HTTP_ADDR=:8080` so the process, Service and probes share one
port contract.

Kubernetes does not restart Pods when an externally managed Secret changes.
After rotating `database.existingSecret`, set `database.secretRevision` to a new
operator-controlled value in the same Helm upgrade. The pod-template annotation
then triggers a rolling replacement of all control-plane Pods.

After rotating `auth.breakGlass.existingSecret`, update
`auth.breakGlass.secretRevision` in the same Helm upgrade. Keep the emergency
token in a namespace-local Secret and restrict Secret access to vendor
operators.

## Availability and scheduling

- The Deployment uses `RollingUpdate` with `maxUnavailable: 0` and
  `maxSurge: 1`.
- The default PodDisruptionBudget requires two available replicas.
- Startup and liveness use `/health/live`; readiness uses `/health/ready`.
- The default topology spread constraint distributes replicas across
  `kubernetes.io/hostname` when capacity permits.
- `resources`, `nodeSelector`, `tolerations`, `affinity`, and
  `topologySpreadConstraints` are configurable in `values.yaml`.

The production schema requires at least three replicas. Adjust
`podDisruptionBudget.minAvailable` when running a larger topology.

The default security contexts match the distroless nonroot image: UID/GID
`65532`, runtime-default seccomp, no privilege escalation, all Linux
capabilities dropped, and a read-only root filesystem. The ServiceAccount token
is mounted only when `ocm.enabled=true`.
