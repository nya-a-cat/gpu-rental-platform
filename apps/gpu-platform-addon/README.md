# GPU Platform Add-on

This module contains the minimal Open Cluster Management add-on used by the
GPU Container Cloud control plane.

The image exposes one binary with manager and agent execution roles:

```text
gpu-platform-addon manager --agent-image=<image>
gpu-platform-addon controller --agent-image=<image>  # manager alias
gpu-platform-addon agent \
  --hub-kubeconfig=/var/run/hub/kubeconfig \
  --cluster-name=<managed-cluster> \
  --addon-namespace=open-cluster-management-agent-addon
```

The manager uses the OCM add-on framework template factory. It creates the
agent `ManifestWork`, manages CSR registration, grants a namespace-scoped hub
role, and reports add-on health from the agent Lease.

The agent lists local Nodes, aggregates `nvidia.com/gpu` and
`nvidia.com/mig-*` allocatable resources, and writes a JSON snapshot to the
`gpu-platform-inventory` ConfigMap in the managed-cluster namespace on the hub.
GPU device identifiers are never collected. A stable Phase 0 capacity
fingerprint is derived from aggregate capacity and excludes the observation
timestamp. Placement and Allocation concurrency will use a later monotonic
ResourceProvider generation with Agent Epoch and fencing data. The agent also
refreshes the `gpu-platform-addon` Lease in its installation namespace. OCM uses
that Lease for `ManagedClusterAddOn` connectivity health; the current manager
includes the Lease in ManifestWork with `ServerSideApply`. OCM owns the Lease
metadata and cleanup while the agent owns its renewal fields. Inventory
freshness is evaluated separately from the snapshot timestamp. In this schema,
`schedulableAllocatable` means capacity on Nodes without
`spec.unschedulable=true`; Ready state, taints, allocations and fault domains enter
the ResourceProvider model in later phases.

The manager passes the current `ManagedClusterAddOn` UID to current agents through
`GPU_PLATFORM_ADDON_UID`. The variable is optional so a current agent continues to run
with the pinned N-1 manager. When the UID is present, the inventory ConfigMap carries
a controller OwnerReference to the `ManagedClusterAddOn`; OCM deletion then removes stale
inventory together with the per-cluster Add-on resources. An N-1 agent ignores the extra
environment variable, which provides the reciprocal current-manager/N-1-agent compatibility path.
