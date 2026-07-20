# GPUStack v2.2.1 Actions baseline

This directory defines the Phase 0 runtime comparison profile for GPUStack
v2.2.1. The profile starts the real GPUStack Server from the signed release
wheel on a GitHub-hosted Ubuntu runner and uses the runner's preinstalled
PostgreSQL 16 service.

The upstream linux/amd64 container image contains a compressed layer larger
than 100 MiB. The repository's dependency-size policy therefore selects the
official 16.23 MiB wheel and the dependency versions exported from the
upstream `uv.lock` at commit
`9e9f841a7ad8170f24d3a0f1c1bbfd307d6ebdaf`. The runtime profile disables the
embedded gateway, built-in observability, update checks and the embedded
worker. This keeps the baseline focused on the server control surface and
avoids downloading inference runtimes or GPU images.

## Acceptance scope

The `gpustack-baseline` Actions job verifies:

- GS-00 runtime version, release checksum, dependency lock and environment
  provenance;
- public health, readiness and version probes;
- admin login through the real `/auth/login` flow;
- GS-04 cluster management API surface and collection access;
- GS-07 GPU Instance create, query, update, stop, start and delete API surface;
- GS-08 SSH public-key and cluster registration API surface;
- GS-09 persistent-volume API surface and collection access;
- GS-10 usage metadata API access;
- server restart with the same PostgreSQL database and stable admin identity;
- evidence redaction, JSON validity and SHA-256 manifests.

This server-only profile does not create a GPU worker, Kubernetes cluster, GPU
Instance, PVC or Worker Tunnel. Those runtime behaviors remain assigned to the
Real Alpha and GPU self-hosted gates.

## Evidence

Successful runs upload `gpustack-baseline-<commit>` for seven days. Full
evidence contains only selected API method maps, collection counts and field
names, package versions, probe results and restart assertions. Cookie jars,
database URLs, passwords, raw OpenAPI documents and response bodies stay in
the runner temporary directory.

Failed runtime attempts may upload a policy-checked partial artifact containing
the sanitized baseline log. The evidence policy rejects credential-like file
names, authentication headers, JWT-shaped values, database URLs, private keys,
runner paths and packages exceeding the configured 100 MiB limit.
