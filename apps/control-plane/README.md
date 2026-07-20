# GPU Cloud Control Plane

This directory contains the Phase 0 Go modular monolith for the production GPU
Container Cloud control plane. The existing TypeScript API remains available as
the UI and workflow reference while product capabilities move behind `/api/v1`.

## Commands

```powershell
$env:DATABASE_URL = "postgres://gpu_control_plane:password@localhost:5432/gpu_control_plane?sslmode=disable"
go run ./cmd/migrate up
go run ./cmd/control-plane
```

The migration command accepts either no argument or `up`. Run the test suite
with `go test ./...`. PostgreSQL repository tests run when
`TEST_DATABASE_URL` points to a disposable PostgreSQL database; otherwise they
are skipped.

## Kubernetes delivery

`charts/gpu-control-plane` installs the service as a three-replica Kubernetes
Deployment. It references `DATABASE_URL` from an existing Secret, runs the
migration binary as a blocking `pre-install,pre-upgrade` Helm hook, exposes a
ClusterIP Service and creates a PodDisruptionBudget with two replicas available
by default.

The Deployment uses `maxUnavailable: 0`, `maxSurge: 1`, live and readiness
HTTP probes, a 30-second termination grace period and the distroless nonroot
security profile. OCM integration is disabled by default. Enabling it mounts the
ServiceAccount token and creates a least-privilege ClusterRole for ManifestWork
read and server-side apply. PostgreSQL and its Secret remain owned by the vendor
deployment environment.

The GitHub Actions HA gate verifies a three-replica install, external Secret
rotation, a failed migration upgrade, distinct baseline/candidate image
replacement, zero-grace single-Pod recovery and Helm release cleanup.

## Runtime contract

| Variable                      |               Default | Purpose                                                          |
| ----------------------------- | --------------------: | ---------------------------------------------------------------- |
| `DATABASE_URL`                |              required | PostgreSQL connection string                                     |
| `HTTP_ADDR`                   |               `:8080` | HTTP listen address                                              |
| `CONTROL_PLANE_VERSION`       |                 `dev` | Build or release version                                         |
| `CONTROL_PLANE_COMMIT`        |             `unknown` | Source revision                                                  |
| `BREAK_GLASS_ADMIN_TOKEN`     |              disabled | Opaque bearer token for the single local emergency administrator |
| `BREAK_GLASS_ADMIN_SUBJECT`   |   `break-glass-admin` | Stable audit subject for the emergency administrator             |
| `SHUTDOWN_TIMEOUT`            |                 `15s` | Graceful HTTP shutdown deadline                                  |
| `READINESS_TIMEOUT`           |                  `2s` | PostgreSQL readiness deadline                                    |
| `AGENT_HEARTBEAT_INTERVAL`    |                 `15s` | Expected managed-cluster Agent report interval                   |
| `AGENT_DEGRADED_AFTER`        |                 `45s` | Age that changes connectivity from connected to degraded         |
| `AGENT_OFFLINE_AFTER`         |                 `90s` | Age that changes connectivity from degraded to offline           |
| `DB_MAX_OPEN_CONNS`           |                  `20` | Maximum open PostgreSQL connections                              |
| `DB_MAX_IDLE_CONNS`           |                   `5` | Maximum idle PostgreSQL connections                              |
| `DB_CONN_MAX_LIFETIME`        |                 `30m` | PostgreSQL connection lifetime                                   |
| `MIGRATION_TIMEOUT`           |                  `5m` | Total migration process deadline, including database startup     |
| `MIGRATION_LOCK_TIMEOUT`      |                 `30s` | PostgreSQL lock wait limit during migrations                     |
| `MIGRATION_STATEMENT_TIMEOUT` |                  `2m` | PostgreSQL per-statement limit during migrations                 |
| `OCM_ENABLED`                 |               `false` | Enable shared-project ManifestWork reconciliation                |
| `OCM_HUB_URL`                 |    Kubernetes service | HTTPS OCM Hub API origin                                         |
| `OCM_DEFAULT_CLUSTER_ID`      |              required | Phase 1 managed cluster used for shared projects                 |
| `OCM_CA_FILE`                 |    service-account CA | Hub TLS trust bundle                                             |
| `OCM_TOKEN_FILE`              | service-account token | Bearer token re-read for every Hub request                       |
| `OCM_CLIENT_CERT_FILE`        |                 empty | Optional client certificate used with `OCM_CLIENT_KEY_FILE`      |
| `OCM_CLIENT_KEY_FILE`         |                 empty | Optional client key used with `OCM_CLIENT_CERT_FILE`             |
| `OCM_RECONCILE_TIMEOUT`       |                  `2m` | Per-event apply and availability deadline                        |
| `OCM_POLL_INTERVAL`           |                  `2s` | ManifestWork condition polling and Outbox scan interval          |
| `OCM_MAX_ATTEMPTS`            |                   `8` | Attempts before an event enters the dead-letter state            |

Available endpoints are:

- `GET /health/live`, `GET /health/ready`, `GET /metrics`
- `GET /api/v1/system/info`
- `GET /api/v1/operations/{operationID}`
- `POST /api/v1/tenants`, `GET /api/v1/tenants/{tenantID}`
- `POST /api/v1/projects`, `GET /api/v1/projects/{projectID}`
- `POST /api/v1/role-bindings`, `GET /api/v1/role-bindings/{bindingID}`
- `GET` and `PUT /api/v1/projects/{projectID}/quotas/{resourceClass}`
- `GET /api/v1/resource-classes/{resourceClass}`
- `POST /api/v1/clusters`, `GET /api/v1/clusters/{clusterID}`
- `GET` and `PUT /api/v1/clusters/{clusterID}/inventory`
- `POST /api/v1/accelerator-profiles`, `GET /api/v1/accelerator-profiles/{profileID}`
- `POST /api/v1/capacity-pools`, `GET /api/v1/capacity-pools/{poolID}`

Tenancy mutations require an authenticated bearer principal and an
`Idempotency-Key` containing 8 to 255 characters. Accepted mutations return
HTTP 202, `Location`, `Operation-Location`, a resource ID and an Operation ID.
The break-glass token is optional at process startup; authenticated tenancy
routes return HTTP 503 until a token is supplied. PostgreSQL RoleBinding
authorization is active for future OIDC principals, and the break-glass
principal has system-administrator scope.

Project creation accepts the `shared` isolation class and records a stable
namespace name plus independent desired, observed and provisioning states. With
`OCM_ENABLED=true`, the Outbox reconciler applies an idempotent ManifestWork to
the configured managed cluster and waits for `Applied` and `Available` before it
marks the Project and Operation succeeded. The work contains Namespace, read-only
Add-on RBAC, GPU ResourceQuota, default-deny and required allow NetworkPolicies,
and Restricted Pod Security labels. Quota changes reapply the same ManifestWork.
Quota reservation, commit and release use row locks and update reserved and
allocated quantities atomically.
Vendor resource catalog routes require the system-administrator principal. Cluster
inventory replacement atomically persists the Cluster, NodePool, Node, opaque GPU
Device, Trait and Inventory hierarchy. `expectedGeneration` prevents lost updates;
Agent epoch and report sequence reject stale snapshots. Reports with an unchanged
source Generation refresh connectivity without rewriting resource facts or advancing
the inventory Generation. Real Alpha accepts the
`gpu.nvidia.full` whole-GPU resource class. Capacity pools bind one NodePool to one
AcceleratorProfile and one scheduler profile. Physical identifiers remain confined
to the vendor-only inventory response.

The control-plane module runs an OCM Hub inventory consumer when OCM is enabled. It reads
only `gpu-platform-inventory/inventory.json` from each registered ManagedCluster
namespace, enforces the v1alpha1 schema and cluster identity, accepts N-1 aggregate
reports as heartbeat-only observations, and retries one optimistic Generation
conflict. The Helm Chart grants the control-plane ServiceAccount one `get` permission
for that fixed ConfigMap name.

API failures use `application/problem+json`. The migration process is separate
from API startup so deployment tooling controls schema rollout order. Migration
SQL is normalized to LF before checksumming, and the migration session enforces
bounded lock and statement waits.
