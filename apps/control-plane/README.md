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

## Runtime contract

| Variable                      |   Default | Purpose                                                      |
| ----------------------------- | --------: | ------------------------------------------------------------ |
| `DATABASE_URL`                |  required | PostgreSQL connection string                                 |
| `HTTP_ADDR`                   |   `:8080` | HTTP listen address                                          |
| `CONTROL_PLANE_VERSION`       |     `dev` | Build or release version                                     |
| `CONTROL_PLANE_COMMIT`        | `unknown` | Source revision                                              |
| `SHUTDOWN_TIMEOUT`            |     `15s` | Graceful HTTP shutdown deadline                              |
| `READINESS_TIMEOUT`           |      `2s` | PostgreSQL readiness deadline                                |
| `DB_MAX_OPEN_CONNS`           |      `20` | Maximum open PostgreSQL connections                          |
| `DB_MAX_IDLE_CONNS`           |       `5` | Maximum idle PostgreSQL connections                          |
| `DB_CONN_MAX_LIFETIME`        |     `30m` | PostgreSQL connection lifetime                               |
| `MIGRATION_TIMEOUT`           |      `5m` | Total migration process deadline, including database startup |
| `MIGRATION_LOCK_TIMEOUT`      |     `30s` | PostgreSQL lock wait limit during migrations                 |
| `MIGRATION_STATEMENT_TIMEOUT` |      `2m` | PostgreSQL per-statement limit during migrations             |

Available foundation endpoints are:

- `GET /health/live`
- `GET /health/ready`
- `GET /metrics`
- `GET /api/v1/system/info`
- `GET /api/v1/operations/{operationID}`

API failures use `application/problem+json`. The migration process is separate
from API startup so deployment tooling controls schema rollout order. Migration
SQL is normalized to LF before checksumming, and the migration session enforces
bounded lock and statement waits.
