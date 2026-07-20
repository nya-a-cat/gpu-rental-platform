# GPU Observability Helm chart

This chart installs the Phase 0 operational telemetry baseline:

- Prometheus with pinned rules for control-plane availability, recovered panics,
  and OpenTelemetry Collector availability;
- Alertmanager with a local default route that operators can replace during
  environment integration;
- OpenTelemetry Collector with OTLP gRPC/HTTP receivers, control-plane metric
  collection, internal telemetry, and a no-op export boundary;
- an optional monthly audit archive CronJob that uploads deterministic JSONL to
  an S3-compatible bucket with Object Lock retention.

The default profile is single-replica with bounded ephemeral data because this
is the Phase 0 component-validation baseline. Phase 2 owns Prometheus HA,
Thanos, durable object storage, and production notification receivers.

## Audit archive

The CronJob is disabled by default. Enable it after creating the database and S3
credential Secrets and an Object Lock-enabled bucket:

```yaml
auditArchive:
  enabled: true
  image:
    repository: registry.example.com/gpu-cloud-control-plane
    digest: sha256:replace-with-released-image-digest
  database:
    existingSecret: gpu-control-plane-database
    secretKey: DATABASE_URL
  s3:
    endpoint: https://objects.example.com
    bucket: gpu-platform-audit
    existingSecret: gpu-audit-archive-s3
```

The job exports the latest sufficiently aged complete month. Set
`AUDIT_ARCHIVE_MONTH=YYYY-MM` on a one-off Job when replaying a specific month.
Existing objects must match size and archive metadata, which makes retries safe
and exposes conflicting rewrites.
