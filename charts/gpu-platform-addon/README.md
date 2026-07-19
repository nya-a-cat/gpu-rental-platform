# GPU Platform Add-on Helm Chart

This chart installs the hub-side GPU Platform Add-on manager, its
`ClusterManagementAddOn`, and the RBAC required by OCM Add-on Framework.

The managed-cluster agent is distributed by the manager through
`ManifestWork`. `image.repository` and `image.tag` select the hub manager image.
`agent.image.repository` and `agent.image.tag` select an independently versioned agent image; empty agent values inherit the manager image. Inventory reporting defaults to 15 seconds and is configured with `agent.reportInterval`.

```bash
helm upgrade --install gpu-platform-addon charts/gpu-platform-addon \
  --kube-context kind-hub \
  --namespace gpu-platform-system \
  --create-namespace \
  --wait
```

Create a `ManagedClusterAddOn` in each accepted managed-cluster namespace to
enable the agent. The Phase 0 example is in
`deploy/ocm/manifests/managed-cluster-addon.yaml`.

The manager intentionally has no Service or HTTP probe in Phase 0. The manager
Deployment rollout verifies process startup, while the agent Lease drives OCM
`ManagedClusterAddOn` connectivity health. The supported Phase 0 manager
topology is one replica with a `Recreate` deployment strategy until leader election is implemented. This gives manager upgrades one active writer for ManifestWork reconciliation.
