# Deployment

## Toolchain

Local development uses:

- Node.js 24 and pnpm 10.34.5 for the React/NestJS workspace
- Go 1.25 for `apps/control-plane` and `apps/gpu-platform-addon` (GitHub Actions uses Go 1.25.1)
- Docker Engine with Docker Compose v2
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

GitHub Actions is the authoritative delivery gate. Local work in this delivery workflow is limited to code and configuration edits; formatting, lint, tests, builds, database integration, Compose validation and runtime smoke checks execute in `.github/workflows/pipeline.yml` after code is pushed. The startup commands above remain operator references.

The v2 gate validates `docker-compose.v2.yml`, builds its images, starts the isolated project with `up --wait`, checks live, ready, metrics and system-information endpoints, emits container logs on failure, and always removes its containers and test volume.

## GitHub Actions

Pull requests and pushes to `main` run the repository quality gate. It covers:

- frozen pnpm installation, formatting, lint, type checks, unit tests and production build;
- NestJS end-to-end tests against authenticated MongoDB and Redis;
- Go formatting, unit tests and builds for the control plane and GPU Platform Add-on, plus PostgreSQL-backed migration/integration checks;
- certification-version consistency, pinned kubectl/Helm installation, Helm rendering and shell syntax checks for the OCM delivery assets;
- default Compose validation plus dedicated v2 Compose validation, image build and runtime smoke checks;
- a separate two-cluster OCM conformance job covering registration, CSR certificates, Lease renewal, ManifestWork, Add-on deployment and inventory reporting, with object-snapshot evidence artifacts;
- a separate Add-on lifecycle job covering immutable current/N-1 images, bidirectional version skew, idempotent install, stale inventory cleanup, per-cluster re-enablement, Helm uninstall and final reinstall;
- simulated API/web, Go control-plane and GPU Platform Add-on container builds.

The simulated reservation suite includes concurrent attempts to reserve one GPU and verifies a single active order. The v2 checks verify migrations and the current Operation/Outbox foundation. Hardware-backed GPU acceptance remains assigned to a self-hosted runner.

Pages deployment depends on the successful `quality`, `ocm-conformance` and `ocm-addon-lifecycle` jobs and runs from `main` on a push or manual workflow dispatch:

```bash
gh workflow run pipeline.yml --ref main
```

The default Pages release is published at `/gpu-rental-platform/`; the frozen `ui-v1.0.0` build remains under `/gpu-rental-platform/classic/`. Pages serves static assets and does not run Go, NestJS, PostgreSQL, MongoDB or Redis.

## Data and rollback

The v2 foundation uses PostgreSQL and the simulated baseline uses MongoDB. Current development contains no production user data and uses parallel replacement, so startup does not migrate MongoDB records into PostgreSQL.

Database migrations run explicitly before the v2 API starts. A schema or data migration requiring destructive rollback must provide a reviewed backup, compatibility window and dedicated rollback procedure. Application rollback should target a compatible signed image and database schema.

## Production considerations

Both Compose stacks are local reference topologies. A vendor deployment requires TLS, secret management, managed or highly available PostgreSQL, backup and restore testing, object storage, OIDC, network policy, audit retention, monitoring, capacity planning and completed phase acceptance tests. Bind public endpoints only through the approved ingress and keep database services private.
