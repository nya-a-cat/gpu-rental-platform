# OCM Phase 0 deployment assets

This directory contains the reproducible GitHub Actions smoke environment for
the first candidate stack:

- Open Cluster Management `1.3.1`
- `clusteradm` `v1.3.1`
- kind `v0.32.0`
- Kubernetes and kubectl `v1.34.8`
- Helm `v3.19.0`

Every downloaded executable and archive has a fixed HTTPS release URL and SHA-256 in
`versions.env`. The scripts download archives as data, verify them, and then
execute the extracted binaries. They do not execute network-delivered shell
scripts.

## CI entry point

Build `gpu-platform-addon:ci` first, then run:

```bash
bash deploy/ocm/scripts/ci-smoke.sh
```

The suite creates `kind-hub` and `kind-cluster1`, installs OCM, registers and
accepts `cluster1`, installs the hub manager Helm chart, enables the managed
cluster add-on, and verifies:

- accepted and available `ManagedCluster` conditions;
- an approved managed-cluster CSR and a fresh cluster Lease;
- ManifestWork delivery of the smoke ConfigMap;
- an approved Add-on CSR and generated Hub kubeconfig;
- available Add-on manager and agent Deployments;
- available `ManagedClusterAddOn`, renewed Add-on Lease, and sanitized inventory capacity fingerprint.

Every run uploads pinned tool versions, the Add-on image description, and key hub/spoke object snapshots. Failed runs also print hub and managed-cluster objects, events, and Add-on logs.
Clusters are deleted at the end. Set `KEEP_CLUSTERS=1` only during an
interactive Actions debugging session.

The scripts are intentionally invoked with `bash`, so repository executable
bits are not required on Windows checkouts.
