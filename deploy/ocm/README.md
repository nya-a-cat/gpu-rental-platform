# OCM Phase 0 deployment assets

This directory contains the reproducible GitHub Actions smoke environment for
the first candidate stack:

- Open Cluster Management `1.3.1`
- `clusteradm` `v1.3.1`
- kind `v0.32.0`
- Kubernetes and kubectl `v1.34.8`
- Helm `v3.19.0`

The ephemeral kind Hub limits client certificates to `7m` so the suite can exercise OCM's native rotation path within one Actions job. Production certificate lifetimes remain a vendor security-policy setting.

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
- available `ManagedClusterAddOn`, renewed Add-on Lease, and sanitized inventory capacity fingerprint;
- native ManagedCluster and Add-on client certificate rotation with new automatically approved CSRs;
- stable Secret and agent Pod identities, plus continued Lease renewal and inventory reporting after the original certificates expire and the Hub API connection is reset.

Credentials are materialized only in a mode-`0600` temporary directory for post-expiry API checks and are removed when the script exits. Uploaded evidence contains sanitized CSR and Secret metadata, certificate fingerprints and validity timestamps, controller arguments, object snapshots, and logs. It excludes CSR request bodies, issued certificate bodies, Secret data, kubeconfig content, and private keys. Failed runs also print hub and managed-cluster objects, events, and Add-on logs.
Clusters are deleted at the end. Set `KEEP_CLUSTERS=1` only during an
interactive Actions debugging session.

The scripts are intentionally invoked with `bash`, so repository executable
bits are not required on Windows checkouts.
