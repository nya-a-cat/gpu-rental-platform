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
GPU device identifiers are never collected. Each snapshot includes a random
128-bit Agent Epoch created at process start, a monotonically increasing report
sequence, and the current `ManagedClusterAddOn` UID as a Fencing Token when the
manager supplies it. `fencingEnabled` states whether the manager supplied that token for downstream
fencing validation. The stable Phase 0 capacity fingerprint excludes observation time
and Agent session metadata, so restart and N/N-1 transitions do not create false
capacity generations.

Hub inventory access uses a namespace RoleBinding containing only the
cluster-specific kube-client registration user. The shared Add-on group is
excluded from that binding, preserving authorization boundaries between managed
cluster namespaces.

The agent also refreshes the `gpu-platform-addon` Lease in its installation
namespace. OCM uses that Lease for `ManagedClusterAddOn` connectivity health;
the current manager includes the Lease in ManifestWork with
`ServerSideApply`. OCM owns Lease metadata and cleanup while the agent owns its
renewal fields. The control-plane domain evaluator calculates connectivity and
inventory freshness with independently configurable 15/45/90 second thresholds. In this schema,
`schedulableAllocatable` means capacity on Nodes without
`spec.unschedulable=true`; Ready state, taints, allocations and fault domains
enter the ResourceProvider model in later phases.

The manager passes the current `ManagedClusterAddOn` UID to current agents
through `GPU_PLATFORM_ADDON_UID`. The variable is optional so a current agent
continues to run with the pinned N-1 manager. Reports produced on that path set
`fencingEnabled=false` and omit the token. When the UID is present, the
inventory ConfigMap carries a controller OwnerReference to the
`ManagedClusterAddOn`; OCM deletion removes stale inventory together with the
per-cluster Add-on resources. An N-1 agent ignores the extra environment
variable, which provides the reciprocal current-manager/N-1-agent compatibility
path. Command admission sequencing will be added with the first workload command
transport; the current `sequence` field orders Agent inventory reports.
