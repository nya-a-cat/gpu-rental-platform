# GPU Rental Platform

[![Pipeline](https://github.com/nya-a-cat/gpu-rental-platform/actions/workflows/pipeline.yml/badge.svg)](https://github.com/nya-a-cat/gpu-rental-platform/actions/workflows/pipeline.yml)

A portfolio-grade GPU resource rental control plane built with React, NestJS, TypeScript, MongoDB, Redis and Docker Compose.

[Live GitHub Pages demo](https://nya-a-cat.github.io/gpu-rental-platform/) · [Architecture](docs/architecture.md) · [Deployment](docs/deployment.md)

## Product workflow

- Filter simulated GPU inventory by model, region, memory, availability and price.
- Reserve a resource, follow explicit order states and return it with one action.
- Manage listings and orders through role-protected administrator routes.
- Switch between Chinese and English across desktop and mobile layouts.
- Explore a transparent browser-only Pages demo without pretending to provision hardware.

## Engineering highlights

- Redis server-side sessions in HttpOnly cookies support logout, logout-all and immediate revocation after a password change.
- Reservation uses a token-owned `SET NX EX` resource lock with Lua compare-and-delete unlock.
- A partial MongoDB unique index remains the durable last line of defense against duplicate active allocation.
- CI runs type checks, unit tests, real MongoDB/Redis E2E tests, production builds and Docker image builds.
- Docker Compose exposes only the Nginx edge on `127.0.0.1:8080`; MongoDB, Redis and the API remain private.

## Run locally

Requirements: Node.js 24, pnpm 10 and Docker Compose v2.

```bash
cp .env.example .env
pnpm install --frozen-lockfile
docker compose up --build -d
```

Open `http://localhost:8080`. Seed simulated inventory and create an administrator with the consolidated CLI:

```bash
pnpm cli demo:seed
pnpm cli admin:create --username admin
```

See [deployment.md](docs/deployment.md) for verification and security notes.

## Honest demo boundary

Only GPU inventory is simulated. The Docker profile uses the real NestJS API, MongoDB documents, Redis sessions and Redis reservation lock. The project does not claim physical provisioning, payment, SSH, notebook access or live GPU telemetry.

GitHub Pages runs a clearly labelled local-browser demo adapter because static hosting cannot run the backend. Backend concurrency and session behavior are tested against real MongoDB and Redis in CI.

## Visual assets

The interface is composed from code-native controls, two original small mechanical illustrations and optional public-domain archive images. See [asset credits](docs/asset-credits.md).

## License

[MIT](LICENSE)
