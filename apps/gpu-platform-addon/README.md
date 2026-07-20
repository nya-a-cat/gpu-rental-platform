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

The agent lists local Nodes and writes a JSON snapshot to the
`gpu-platform-inventory` ConfigMap in the managed-cluster namespace on the hub.
The additive v1alpha1 payload retains aggregate `nvidia.com/gpu` and
`nvidia.com/mig-*` totals for N/N-1 compatibility and adds the Phase 1
NodePool, Node, whole-GPU logical-device and Trait hierarchy consumed by the
vendor resource catalog.

Raw Node names, provider IDs, machine IDs and physical GPU IDs are excluded.
Node and logical GPU slot keys use an HMAC derived from the current
`ManagedClusterAddOn` UID, remain stable across agent Pod restarts, and rotate
when the Add-on is recreated. Logical device records are emitted only when GPU
Feature Discovery supplies product and memory labels. Each snapshot includes a
random 128-bit Agent Epoch, a monotonically increasing report sequence, and the
current Add-on UID as a Fencing Token when the manager supplies it.
`fencingEnabled` states whether downstream fencing validation is active. The
SHA-256 source Generation excludes observation time and Agent session metadata
and includes detailed health, traits and capacity facts.

Hub inventory access uses a namespace RoleBinding containing only the
cluster-specific kube-client registration user. The shared Add-on group is
excluded from that binding, preserving authorization boundaries between managed
cluster namespaces.

The agent also refreshes the `gpu-platform-addon` Lease in its installation
namespace. OCM uses that Lease for `ManagedClusterAddOn` connectivity health;
the current manager includes the Lease in ManifestWork with
`ServerSideApply`. OCM owns Lease metadata and cleanup while the agent owns its
renewal fields. The control-plane domain evaluator calculates connectivity and
inventory freshness with independently configurable 15/45/90 second thresholds.
A Node contributes schedulable whole-GPU capacity only while Ready, healthy and
not cordoned. Pressure and network conditions degrade health. The optional
`gpu.platform.nyaacat.dev/node-pool` label selects a DNS-compatible NodePool;
other Nodes enter the `default` pool. Per-device DCGM health and physical-device
mapping remain outside this Kubernetes Node snapshot boundary.

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
