# GPU Container Cloud

[![Pipeline](https://github.com/nya-a-cat/gpu-rental-platform/actions/workflows/pipeline.yml/badge.svg)](https://github.com/nya-a-cat/gpu-rental-platform/actions/workflows/pipeline.yml)

GPU Container Cloud 是面向云服务器厂商、渠道商和企业租户建设的 GPU 容器云控制面。仓库采用双轨演进：`apps/control-plane` 是 Go、PostgreSQL 与 OCM 方向的生产轨，`apps/api` 与现有 React 控制台保留为可运行的模拟业务基准。

[Live GitHub Pages demo](https://nya-a-cat.github.io/gpu-rental-platform/) · [v2 architecture](docs/control-plane-v2.md) · [Architecture](docs/architecture.md) · [Deployment](docs/deployment.md) · [Roadmap](ROADMAP.md)

## Repository tracks

### Production v2 foundation

- Go 1.25 product control plane with a stable `/api/v1` contract.
- PostgreSQL-backed Operation, idempotency, Outbox and audit foundations.
- Internal `BillingEngine`, `AuthorizationEngine`, `JobEngine` and OCM-facing fleet boundaries.
- Health, readiness, Prometheus metrics and request-correlation endpoints.
- A versioned OpenAPI 3.1 contract for generated clients and vendor integration.

Phase 0 currently establishes the control-plane foundation. ManagedCluster registration, real GPU scheduling, tenant isolation and commercial billing remain staged roadmap work. See [GPU Cloud Control Plane v2](docs/control-plane-v2.md) for the complete target and delivery gates.

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

The dedicated `docker-compose.v2.yml` project starts PostgreSQL and the Go control plane in its own internal network and named volume. The default `docker-compose.yml` continues to accept legacy `.env` files that contain only MongoDB and Redis settings; it does not evaluate `POSTGRES_PASSWORD` or other v2 variables:

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

See [deployment.md](docs/deployment.md) for environment variables, verification commands and CI behavior.

## Current product boundary

The v2 service currently provides production-oriented control-plane foundations and does not yet provision physical GPUs. The simulated baseline uses real MongoDB, Redis sessions and concurrency controls while its GPU delivery, wallet settlement and infrastructure operations remain explicitly simulated. SSH, terminal and notebook addresses use reserved `.invalid` domains.

GitHub Pages contains static assets and a labelled browser-local adapter. Backend, database and cluster services are unavailable on Pages.

## Visual assets

The interface uses code-native controls, original mechanical illustrations and optional public-domain archive images. See [asset credits](docs/asset-credits.md).

## License

[MIT](LICENSE)
