# Deployment

## Local Docker deployment

Prerequisites:

- Docker Engine with Docker Compose v2
- A free local TCP port `8080`

Create the local environment file before starting the stack:

```bash
cp .env.example .env
```

Replace both passwords. Because the Compose connection strings embed credentials in URLs, use long values containing only letters, digits, `.`, `_`, `~` and `-`.

Start and verify the services:

```bash
docker compose up --build -d
docker compose ps
curl --fail http://localhost:8080/api/health/ready
```

Open `http://localhost:8080`. Nginx serves the web application and proxies `/api` to NestJS. MongoDB, Redis and the API do not publish host ports; the web port is bound to `127.0.0.1`.

Inspect logs or stop the deployment with:

```bash
docker compose logs --tail=200 api web
docker compose down
```

`docker compose down` keeps named database volumes. To remove local application data intentionally, use `docker compose down --volumes`.

## Local workspace build

The non-container toolchain uses Node.js 24 and pnpm 10:

```bash
corepack enable
pnpm install --frozen-lockfile
pnpm format:check
pnpm lint
pnpm typecheck
pnpm test
pnpm build
```

## GitHub Actions

Pull requests and pushes to `main` or `ui/interactive-console-v2` run the frozen install, formatting, lint, type checks, unit tests, workspace build, Compose validation and image build. The quality job also starts authenticated MongoDB 8 and an isolated Redis 8 service, then runs the API end-to-end suite against those real data stores. This suite includes 20 concurrent attempts to reserve one GPU and verifies that exactly one active order is created. Dependencies are installed from the committed lockfile.

On either version branch, the Pages job builds two independent web bundles. The Classic bundle always checks out the frozen `ui-v1.0.0` tag and uses `/gpu-rental-platform/classic/`; the Next bundle checks out `ui/interactive-console-v2` and uses `/gpu-rental-platform/next/`. The job assembles both bundles with a root version selector before publishing one Pages artifact. Backend quality checks remain visible and are not skipped, but a backend-only failure does not replace an already working static product walkthrough with a 404 page. The deployment token has only the `pages: write` and `id-token: write` permissions required by GitHub Pages.

The API E2E step builds the NestJS application first and imports the compiled `dist` modules. This preserves TypeScript decorator metadata and exercises the same JavaScript artifacts used by the production container.

To enable the public demo, set the repository's Pages source to **GitHub Actions**. The expected project URL is:

```text
https://<github-account>.github.io/gpu-rental-platform/
```

The version selector links to both public builds:

```text
https://<github-account>.github.io/gpu-rental-platform/classic/
https://<github-account>.github.io/gpu-rental-platform/next/
```

Classic and Next use separate browser-local demo-state namespaces. Orders, sessions and inventory changes created in one preview do not modify the other preview.

GitHub Pages hosts static assets only. It does not run the NestJS API, MongoDB or Redis; use the Docker profile to exercise the real backend.

## Production considerations

The Compose file is a reference local and portfolio deployment, not a public multi-tenant production topology. Before an internet-facing deployment, provide managed data stores, TLS termination, secret management, backups, monitoring and a restrictive production origin. Set `COOKIE_SECURE=true` behind HTTPS and do not publish database ports.
