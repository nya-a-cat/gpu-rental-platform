# Deployment

## Toolchain

Local development uses:

- Node.js 24 and pnpm 10.34.5 for the React/NestJS workspace
- Go 1.25 for `apps/control-plane` and `apps/gpu-platform-addon` (GitHub Actions uses Go 1.25.1)
- Docker Engine with Docker Compose v2
- Kubernetes 1.34.x for the production control-plane Chart
- Helm 3 for Kubernetes install, upgrade and rollback operations
- free loopback ports `8080` for the simulated baseline and `8081` for v2

Create a local environment file and replace all example passwords:

```bash
cp .env.example .env
```

Use long password values containing letters, digits, `.`, `_`, `~` and `-`. This avoids URL-encoding ambiguity in the local connection strings. Commit no `.env` file.

## v2 control-plane foundation

The dedicated `docker-compose.v2.yml` project runs PostgreSQL and the Go control plane independently from the default simulated stack. It uses the fixed project name `gpu-cloud-control-plane-v2` and its own PostgreSQL volume. PostgreSQL and migrations attach only to the internal backend network; the Go API attaches to backend and edge networks. The default `docker-compose.yml` never evaluates `POSTGRES_PASSWORD`, so existing simulation-only `.env` files remain valid.

Start the v2 services and wait for PostgreSQL, the migration job and the API health check:

```bash
docker compose -f docker-compose.v2.yml up --build --wait postgres control-plane
docker compose -f docker-compose.v2.yml ps
```

Verify process, database readiness, metrics and build information:

```bash
curl --fail http://localhost:8081/health/live
curl --fail http://localhost:8081/health/ready
curl --fail http://localhost:8081/metrics
curl --fail http://localhost:8081/api/v1/system/info
```

Inspect logs with:

```bash
docker compose -f docker-compose.v2.yml logs --tail=200 postgres control-plane
```

The container listens on `HTTP_ADDR=:8080`; Compose publishes its edge-network side as `127.0.0.1:8081`. PostgreSQL remains reachable only through the dedicated internal backend network, and the migration service has no edge-network attachment. The migration service executes `/usr/local/bin/control-plane-migrate up` and exits before the API starts. Migration execution defaults to `MIGRATION_TIMEOUT=5m`, `MIGRATION_LOCK_TIMEOUT=30s` and `MIGRATION_STATEMENT_TIMEOUT=2m`; deployments can override them through the v2 environment. The initial migration creates audit partitions for the current and following calendar months only. A later Phase 0 operations task must create future partitions before each month boundary and monitor the default partition; automatic partition maintenance is not implemented yet. Stop this stack with `docker compose -f docker-compose.v2.yml down`; add `--volumes` only when intentionally deleting the isolated v2 PostgreSQL data.

### Kubernetes Helm delivery

`charts/gpu-control-plane` is the vendor Kubernetes delivery profile. It
deploys three control-plane replicas, a ClusterIP Service, a dedicated
ServiceAccount and a `policy/v1` PodDisruptionBudget. PostgreSQL remains an
external dependency and the Chart reads its complete connection URL from an
existing namespace-local Secret.

```bash
kubectl create namespace gpu-control-plane-system
kubectl --namespace gpu-control-plane-system create secret generic gpu-control-plane-database \
  --from-literal=DATABASE_URL='postgres://user:password@postgres.example:5432/gpu_cloud?sslmode=require'

helm upgrade --install gpu-control-plane charts/gpu-control-plane \
  --namespace gpu-control-plane-system \
  --set image.repository=registry.example/gpu-cloud-control-plane \
  --set image.tag=0.1.0 \
  --atomic \
  --timeout 11m
```

The Secret must exist before Helm runs. A blocking `pre-install,pre-upgrade`
Job executes `/usr/local/bin/control-plane-migrate up`; a migration failure
stops the release before the Deployment template changes. The Deployment uses
`maxUnavailable: 0`, `maxSurge: 1`, a 30-second termination grace period,
startup/liveness checks on `/health/live` and PostgreSQL readiness on
`/health/ready`. The default disruption budget keeps two replicas available.

The control-plane Pods use UID/GID `65532`, runtime-default seccomp, a
read-only root filesystem, no privilege escalation, no Linux capabilities and
no ServiceAccount token mount. The Chart creates no RBAC binding, database,
database user or database Secret. Change `database.secretRevision` whenever
the external Secret data rotates so the Deployment replaces Pods. See
`charts/gpu-control-plane/README.md` for the complete value contract.

Every migration must preserve compatibility with the current and N-1
control-plane images. Helm application rollback does not reverse a committed
database migration, so destructive schema contraction requires a later,
separately reviewed release after the compatibility window closes.

### Direct Go workflow

Provide a reachable PostgreSQL `DATABASE_URL`, then run:

```bash
pnpm control-plane:migrate
pnpm control-plane:test
pnpm control-plane:run
```

The scripts resolve to:

```bash
go -C apps/control-plane run ./cmd/migrate up
go -C apps/control-plane test ./...
go -C apps/control-plane run ./cmd/control-plane
```

`DATABASE_URL` is mandatory. Runtime configuration also supports `HTTP_ADDR`, `CONTROL_PLANE_VERSION`, `CONTROL_PLANE_COMMIT`, `SHUTDOWN_TIMEOUT`, `READINESS_TIMEOUT`, `DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS` and `DB_CONN_MAX_LIFETIME`. Compose uses `CONTROL_PLANE_STOP_GRACE_PERIOD=20s`; this value must remain greater than `SHUTDOWN_TIMEOUT`, whose default is `15s`, so the process can finish graceful shutdown before the container is terminated.

Phase 0 currently exposes foundational endpoints and includes the first OCM fleet/Add-on implementation. Its certification status remains pending until the Actions conformance run succeeds. Real GPU scheduling, tenant APIs and billing execution require later phase components.

## OCM fleet and GPU Platform Add-on

The first fleet integration fixes OCM and `clusteradm` at `v1.3.1`, OCM API and Addon Framework at `v1.3.0`, and the GitHub-hosted kind environment at Kubernetes `v1.34.8`. The production target remains Kubernetes `v1.34.9`; the two patch versions retain separate evidence in the [certification matrix](certification/kubernetes-1.34-matrix.md).

The `ocm-conformance` Actions job builds `gpu-platform-addon:ci` and invokes `bash deploy/ocm/scripts/ci-smoke.sh`. The script downloads pinned kind, clusteradm, kubectl and Helm release assets, verifies their SHA-256 digests, creates separate hub and managed kind clusters, and cleans both clusters at the end. The independent `ocm-addon-lifecycle` job builds current `0.2.0` and pinned N-1 `0.1.0` images from full Git history, then verifies bidirectional version skew, cleanup, re-enablement, uninstall and reinstall.

The conformance path verifies ManagedCluster acceptance, signed CSR certificates, cluster Lease renewal, ManifestWork delivery, Add-on CSR registration, generated hub kubeconfig, managed-cluster Add-on Deployment readiness, Add-on Lease renewal and the sanitized inventory ConfigMap with a Phase 0 capacity fingerprint and `ManagedClusterAddOn` ownership. Evidence uploads use explicit field whitelists and a pre-upload policy scan; node addresses and machine identifiers, Secret data, CSR bodies, kubeconfig content, private keys, full Docker inspect output and runner storage paths are excluded. Failure diagnostics remain in the Actions step log.

The GitHub-hosted runner has no NVIDIA GPU. Driver, Device Plugin, `nvidia-smi`, DCGM, MIG and GPU workload tests remain assigned to the self-hosted GPU certification gate.

## GPUStack Phase 0 comparison baseline

The `gpustack-baseline` Actions job starts the official GPUStack v2.2.1 Server
wheel against the PostgreSQL service preinstalled on the Ubuntu 24.04 runner.
The release URL, release checksum, upstream commit, exported dependency lock,
Python version and uv version are fixed in `deploy/gpustack/versions.env`.

The baseline disables the embedded gateway, built-in observability, update
checks and embedded worker. It verifies public probes, real administrator
login, selected cluster, GPU Instance, SSH key, persistent-volume and usage
API surfaces, authenticated collection reads and persistence across a server
restart. It does not provision a Kubernetes worker, physical GPU, instance,
PVC or tunnel.

The official linux/amd64 container image has a compressed layer larger than
100 MiB. This profile uses the 16.23 MiB official wheel and the exact runtime
dependencies exported from the upstream lock. The job fails when its uv cache
contains any individual file larger than 100 MiB. Successful runs retain
policy-checked summaries and checksums for seven days; raw response bodies,
cookie jars, credentials and database URLs remain in runner-temporary storage.
See `deploy/gpustack/README.md` and the
[Phase 0 comparison matrix](research/gpustack-v2.2.1-phase0-benchmark.md) for
the acceptance boundary.

## Simulated baseline

Start and verify the existing React/NestJS product baseline:

```bash
docker compose up --build -d
docker compose ps
curl --fail http://localhost:8080/api/health/ready
```

Open `http://localhost:8080`. Nginx serves React and proxies `/api` to NestJS. MongoDB, Redis and NestJS stay on private container networks.

Inspect logs or stop the stack with:

```bash
docker compose logs --tail=200 api web
docker compose down
```

`docker compose down` keeps named database volumes. `docker compose down --volumes` intentionally removes only the default simulation project's MongoDB and Redis data. The v2 PostgreSQL volume belongs to the separate `gpu-cloud-control-plane-v2` project.

## Verification policy

GitHub Actions is the authoritative delivery gate. Local work in this delivery workflow is limited to code and configuration edits. The repository uses two verification levels:

- `.github/workflows/pipeline.yml` is the fast gate for pull requests and pushes to `main`. Three jobs run in parallel: frontend/API formatting, lint, type checks, unit tests and build; Go module, formatting, vet, unit tests and command builds; Helm, shell, version and Compose static validation.
- `.github/workflows/certification.yml` is the full runtime certification gate. It runs at 18:17 UTC each day, through manual dispatch and for `v*` release tags. It starts MongoDB, Redis and PostgreSQL; runs service-backed tests and migrations; builds container images; executes v2 Compose smoke, three-replica HA, two-cluster OCM, Add-on N/N-1, GPUStack and observability evidence workflows.

A green fast gate certifies source-level feedback for the commit. Release readiness requires a current green Certification run. The interval before the scheduled full run carries database, container and multi-cluster integration risk, so release tags also invoke the complete suite.

The v2 certification validates `docker-compose.v2.yml`, builds its images, starts the isolated project with `up --wait`, checks live, ready, metrics and system-information endpoints, exercises tenant, project, RoleBinding, idempotency and quota APIs, emits container logs on failure, and removes its containers and test volume. The independent `control-plane-ha` job installs the Helm Chart into a fixed Kubernetes 1.34 kind cluster and exercises migration ordering, external Secret rotation, three-replica availability, shared PostgreSQL Operation reads, a rejected migration upgrade, baseline-to-candidate image replacement, zero-grace single-Pod failure recovery and release cleanup. Successful runs upload full assertion evidence. Failed HA runs upload the available logs and object snapshots after the same credential and path scan passes.

The independent `gpustack-baseline` job installs the checksum-pinned GPUStack v2.2.1 wheel and lock-derived dependencies, starts its real API process twice against the same PostgreSQL database and uploads evidence that passes the credential, path, file-type and package-size policy.

The first successful baseline is Pipeline `29713314184`, job `88261048162`, commit `aed41cb339af1965d5787db10d5a227df331ab8d` and Artifact `8449510409`. Its full evidence policy checked 12 files with zero violations. The run used GPUStack v2.2.1 commit `9e9f841`, Python 3.12.13, uv 0.9.6 and PostgreSQL 16.14; it installed 135 packages and measured a largest cached file of 63,816,496 bytes under the 100 MiB boundary.

## GitHub Actions

The fast gate covers:

- frozen pnpm installation, formatting, lint, type checks, unit tests and production build;
- Go formatting, module consistency, vet, unit tests and command builds for the control plane and GPU Platform Add-on;
- certification-version consistency, pinned kubectl/Helm installation and Chart lint for the delivery assets;
- shell and Python syntax checks plus default and v2 Compose configuration validation.

The full Certification workflow adds:

- NestJS end-to-end tests against authenticated MongoDB and Redis;
- PostgreSQL-backed migrations and integration tests;
- exhaustive Helm rendering and evidence-policy tests;
- simulated API/web, Go control-plane and GPU Platform Add-on container builds;
- v2 Compose runtime smoke;
- control-plane HA, two-cluster OCM conformance, Add-on lifecycle, GPUStack and observability runtime evidence.

The simulated reservation suite includes concurrent attempts to reserve one GPU and verifies a single active order. Hardware-backed GPU acceptance remains assigned to a self-hosted runner.

Pages deployment depends on the three successful fast-gate jobs and runs from `main` on a push or manual fast-gate dispatch:

```bash
gh workflow run pipeline.yml --ref main
```

Run the complete certification suite on demand with:

```bash
gh workflow run certification.yml --ref main
```

The default Pages release is published at `/gpu-rental-platform/`; the frozen `ui-v1.0.0` build remains under `/gpu-rental-platform/classic/`. Pages serves static assets and does not run Go, NestJS, PostgreSQL, MongoDB or Redis.

## Data and rollback

The v2 foundation uses PostgreSQL and the simulated baseline uses MongoDB. Current development contains no production user data and uses parallel replacement, so startup does not migrate MongoDB records into PostgreSQL.

Database migrations run explicitly before the v2 API starts. A schema or data migration requiring destructive rollback must provide a reviewed backup, compatibility window and dedicated rollback procedure. Application rollback should target a compatible signed image and database schema.

## Production considerations

Both Compose stacks are local reference topologies. A vendor deployment requires TLS, secret management, managed or highly available PostgreSQL, backup and restore testing, object storage, OIDC, network policy, audit retention, monitoring, capacity planning and completed phase acceptance tests. Bind public endpoints only through the approved ingress and keep database services private.
