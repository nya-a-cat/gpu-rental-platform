# Phase 0 observability and audit archive

## Delivery scope

The Phase 0 profile establishes a deployable operational telemetry path for the
GPU Container Cloud control plane:

- Prometheus scrapes the Go control plane and the OpenTelemetry Collector;
- Alertmanager receives the first availability and recovered-panic rules;
- the OpenTelemetry Collector accepts OTLP over gRPC and HTTP and scrapes the
  control-plane metric endpoint;
- a monthly CronJob can export immutable PostgreSQL audit events to an
  S3-compatible Object Lock bucket;
- GitHub Actions verifies the integrated runtime and publishes sanitized evidence.

This profile uses one replica for each telemetry component, 24 hours of
Prometheus retention, and bounded ephemeral storage. Prometheus HA, Thanos,
long-term metric storage, and production alert receivers remain Phase 2 work.

## Helm deployment

The chart is located at `charts/gpu-observability`. Its certified Phase 0 image
set is pinned by multi-architecture manifest digest:

| Component | Version | Image |
| --- | --- | --- |
| Prometheus | v3.13.1 | `prom/prometheus` |
| Alertmanager | v0.33.1 | `prom/alertmanager` |
| OpenTelemetry Collector K8s distribution | 0.156.0 | `otel/opentelemetry-collector-k8s` |

The default control-plane metric target is
`gpu-control-plane.gpu-control-plane-system.svc.cluster.local:8080`. Set
`controlPlaneMetricsTarget` when the service name or namespace differs.

```bash
helm upgrade --install gpu-observability charts/gpu-observability \
  --namespace gpu-observability-system \
  --create-namespace
```

The default alert rules are:

- `GPUControlPlaneMetricsUnavailable`;
- `GPUControlPlaneRecoveredPanic`;
- `GPUPlatformOTelCollectorUnavailable`.

The default Alertmanager receiver is intentionally local and has no external
notification endpoint. A provider deployment must supply its approved routing
and receiver configuration before treating alert delivery as production-ready.

The Collector exports to a no-op boundary in Phase 0. This validates OTLP
ingestion and processor behavior without sending tenant payloads to an
unconfigured destination. A central exporter is introduced together with the
provider's Thanos and telemetry retention design.

## Audit archive contract

The control-plane image contains `/usr/local/bin/control-plane-audit-archive`.
The command:

1. reads one complete UTC month from `audit_events` in `occurred_at, id` order;
2. writes deterministic JSON Lines and computes its SHA-256 digest;
3. uploads to
   `<prefix>/year=YYYY/month=MM/audit-events-YYYY-MM.jsonl`;
4. sets period, row-count, schema, and digest metadata;
5. applies S3 Object Lock retention in `GOVERNANCE` or `COMPLIANCE` mode;
6. treats a matching existing object as a successful idempotent retry;
7. fails when an existing object conflicts with the current export.

The command never deletes PostgreSQL audit records. Retention and database
partition maintenance remain separate operator responsibilities.

Required runtime variables:

| Variable | Purpose |
| --- | --- |
| `DATABASE_URL` | Read access to the control-plane PostgreSQL database |
| `AUDIT_ARCHIVE_S3_ENDPOINT` | S3-compatible HTTP or HTTPS origin |
| `AUDIT_ARCHIVE_S3_ACCESS_KEY` | Object-store access key from a Secret |
| `AUDIT_ARCHIVE_S3_SECRET_KEY` | Object-store secret key from a Secret |
| `AUDIT_ARCHIVE_S3_BUCKET` | Object Lock-enabled archive bucket |

Optional variables include `AUDIT_ARCHIVE_S3_REGION`, `AUDIT_ARCHIVE_PREFIX`,
`AUDIT_ARCHIVE_MONTH`, `AUDIT_ARCHIVE_RETENTION_MODE`,
`AUDIT_ARCHIVE_RETENTION_DAYS`, `AUDIT_ARCHIVE_MIN_AGE`,
`AUDIT_ARCHIVE_TIMEOUT`, and `TMPDIR`.

The Helm CronJob is disabled by default. Its default schedule runs on the eighth
day of each month, after the seven-day minimum-age boundary. Enabling it requires
an Object Lock-enabled bucket, a database Secret, an object-store Secret, and a
released control-plane image reference.

## Runtime verification

The `observability-baseline` GitHub Actions job performs these checks:

- exports a real PostgreSQL audit event and verifies upload plus idempotent replay;
- verifies SigV4, Object Lock headers, immutable metadata, JSONL content, and SHA-256;
- deploys the control plane and observability chart to Kubernetes 1.34 kind;
- verifies both Prometheus targets are healthy and all three alert rules load;
- posts an OTLP metric payload and checks the Collector health and telemetry endpoint;
- checks Pod security contexts, disabled ServiceAccount token mounts, and ready replicas;
- deletes the temporary cluster and publishes only evidence accepted by the
  credential and host-identity policy.

GitHub Actions artifacts are the authoritative runtime record for this profile.
Local checks are limited to source formatting, syntax, schema parsing, and
version consistency.
