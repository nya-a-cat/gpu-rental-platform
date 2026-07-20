# GPU Container Cloud

[![Pipeline](https://github.com/nya-a-cat/gpu-rental-platform/actions/workflows/pipeline.yml/badge.svg)](https://github.com/nya-a-cat/gpu-rental-platform/actions/workflows/pipeline.yml)

GPU Container Cloud 是面向云服务器厂商、渠道商和企业租户建设的 GPU 容器云控制面。仓库采用双轨演进：`apps/control-plane` 是 Go、PostgreSQL 与 OCM 方向的生产轨，`apps/api` 与现有 React 控制台保留为可运行的模拟业务基准。

[Live GitHub Pages demo](https://nya-a-cat.github.io/gpu-rental-platform/) · [v2 architecture](docs/control-plane-v2.md) · [Kubernetes 1.34 matrix](docs/certification/kubernetes-1.34-matrix.md) · [GPUStack benchmark](docs/research/gpustack-v2.2.1-phase0-benchmark.md) · [Deployment](docs/deployment.md) · [Roadmap](ROADMAP.md)

## Repository tracks

### Production v2 foundation

- Go 1.25 product control plane with a stable `/api/v1` contract.
- PostgreSQL-backed Operation, idempotency, Outbox and audit foundations.
- Internal `BillingEngine`, `AuthorizationEngine`, `JobEngine` and OCM-facing fleet boundaries.
- OCM 1.3.1 fleet registration assets and a minimal Addon Framework 1.3.0 GPU inventory agent.
- A three-replica Helm delivery profile with migration hooks, disruption protection and an Actions-only HA gate.
- A digest-pinned Prometheus, Alertmanager and OpenTelemetry profile with an Object Lock audit archive command and Actions runtime gate.
- A pinned GPUStack v2.2.1 server baseline for Phase 0 product-surface comparison in GitHub Actions.
- Health, readiness, Prometheus metrics, request correlation, ManagedCluster Lease and Add-on Lease paths.
- A versioned OpenAPI 3.1 contract for generated clients and vendor integration.

Phase 0 now contains the control-plane foundation, the first Kubernetes 1.34 certification matrix, an Actions-only OCM conformance harness and a verified GPUStack v2.2.1 server comparison baseline. Real GPU scheduling, tenant isolation and commercial billing remain staged roadmap work. See [GPU Cloud Control Plane v2](docs/control-plane-v2.md) for the complete target and delivery gates.

### Simulated product baseline

- React console and NestJS API backed by MongoDB and Redis.
- Simulated GPU inventory, orders, instance lifecycle and projected usage cost.
- Wallet, SSH/API keys, firewall rules, persistent volumes, snapshots, teams and projects.
- Role-protected administration and a transparent browser-only Pages demo.

This track remains the UI and workflow regression baseline while production domains move to `/api/v1` one at a time.

## Reference local operation

GitHub Actions is the authoritative verification gate. The commands below document operator startup and manual inspection; repository changes are tested and built after push.

Requirements:

- Node.js 24 and pnpm 10 for the React/NestJS workspace
- Go 1.25 for direct control-plane development
- Docker Engine with Docker Compose v2
- Kubernetes 1.34.x for the control-plane chart
- Helm 3 for Kubernetes installation and upgrades

Create local credentials first:

```bash
cp .env.example .env
```

### Run the simulated baseline

```bash
pnpm install --frozen-lockfile
docker compose up --build -d
```

Open `http://localhost:8080`. Seed simulated inventory and create an administrator with:

```bash
pnpm cli demo:seed
pnpm cli admin:create --username admin
```

### Run the v2 foundation

The dedicated `docker-compose.v2.yml` project keeps PostgreSQL and migrations on an internal backend network, connects the Go API to both backend and edge networks, and publishes the edge side only on `127.0.0.1:8081`; PostgreSQL uses its own named volume. The default `docker-compose.yml` continues to accept legacy `.env` files that contain only MongoDB and Redis settings; it does not evaluate `POSTGRES_PASSWORD` or other v2 variables:

```bash
docker compose -f docker-compose.v2.yml up --build --wait postgres control-plane
curl --fail http://localhost:8081/health/ready
curl --fail http://localhost:8081/api/v1/system/info
```

For direct Go development against a configured `DATABASE_URL`:

```bash
pnpm control-plane:migrate
pnpm control-plane:run
pnpm control-plane:test
```

For a Kubernetes deployment, create the external PostgreSQL connection Secret
and install `charts/gpu-control-plane`. The chart defaults to three replicas,
runs migrations before install and upgrade, and keeps PostgreSQL outside the
release lifecycle:

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

See [deployment.md](docs/deployment.md) for environment variables, verification commands and CI behavior.
See [observability.md](docs/observability.md) for telemetry, alert, audit archive and evidence contracts.

## Current product boundary

The v2 service currently provides production-oriented control-plane foundations and a sanitized OCM inventory path; it does not yet provision physical GPUs. The simulated baseline uses real MongoDB, Redis sessions and concurrency controls while its GPU delivery, wallet settlement and infrastructure operations remain explicitly simulated. SSH, terminal and notebook addresses use reserved `.invalid` domains.

GitHub Pages contains static assets and a labelled browser-local adapter. Backend, database and cluster services are unavailable on Pages.

## Visual assets

The interface uses code-native controls, original mechanical illustrations and optional public-domain archive images. See [asset credits](docs/asset-credits.md).

## License

[MIT](LICENSE)
