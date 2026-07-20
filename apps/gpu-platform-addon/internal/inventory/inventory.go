package inventory

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
)

const (
	SchemaVersion    = "gpu.platform.nyaacat.dev/v1alpha1"
	ConfigMapName    = "gpu-platform-inventory"
	ConfigMapDataKey = "inventory.json"

	WholeGPUResourceClass = "gpu.nvidia.full"
	DefaultNodePool       = "default"
)

const (
	labelNodePool       = "gpu.platform.nyaacat.dev/node-pool"
	labelGPUProduct     = "nvidia.com/gpu.product"
	labelGPUMemory      = "nvidia.com/gpu.memory"
	labelCUDAMajor      = "nvidia.com/cuda.runtime.major"
	labelCUDAMinor      = "nvidia.com/cuda.runtime.minor"
	labelComputeMajor   = "nvidia.com/gpu.compute.major"
	labelComputeMinor   = "nvidia.com/gpu.compute.minor"
	labelMIGCapable     = "nvidia.com/mig.capable"
	labelInstanceType   = "node.kubernetes.io/instance-type"
	labelTopologyZone   = "topology.kubernetes.io/zone"
	labelTopologyRegion = "topology.kubernetes.io/region"
)

type ManagementState string

const (
	ManagementEnabled ManagementState = "enabled"
)

type HealthState string

const (
	HealthHealthy     HealthState = "healthy"
	HealthDegraded    HealthState = "degraded"
	HealthUnreachable HealthState = "unreachable"
	HealthFailed      HealthState = "failed"
	HealthUnknown     HealthState = "unknown"
)

type Resource struct {
	Name                   string `json:"name"`
	Allocatable            int64  `json:"allocatable"`
	SchedulableAllocatable int64  `json:"schedulableAllocatable"`
}

type GPUDevice struct {
	OpaqueKey       string            `json:"opaqueKey"`
	ResourceClass   string            `json:"resourceClass"`
	Model           string            `json:"model"`
	MemoryMiB       int64             `json:"memoryMiB"`
	AcceleratorMode string            `json:"acceleratorMode"`
	HealthState     HealthState       `json:"healthState"`
	Allocatable     bool              `json:"allocatable"`
	Traits          map[string]string `json:"traits"`
}

type Node struct {
	OpaqueKey       string            `json:"opaqueKey"`
	ManagementState ManagementState   `json:"managementState"`
	HealthState     HealthState       `json:"healthState"`
	Schedulable     bool              `json:"schedulable"`
	Traits          map[string]string `json:"traits"`
	GPUDevices      []GPUDevice       `json:"gpuDevices"`
}

type NodePool struct {
	Name            string          `json:"name"`
	ManagementState ManagementState `json:"managementState"`
	Nodes           []Node          `json:"nodes"`
}

type Snapshot struct {
	SchemaVersion        string     `json:"schemaVersion"`
	ClusterName          string     `json:"clusterName"`
	AgentEpoch           string     `json:"agentEpoch"`
	Sequence             uint64     `json:"sequence"`
	FencingToken         string     `json:"fencingToken,omitempty"`
	FencingEnabled       bool       `json:"fencingEnabled"`
	Generation           string     `json:"generation"`
	ObservedAt           time.Time  `json:"observedAt"`
	ExecutionHealthy     bool       `json:"executionHealthy"`
	Fenced               bool       `json:"fenced"`
	NodeCount            int        `json:"nodeCount"`
	SchedulableNodeCount int        `json:"schedulableNodeCount"`
	Resources            []Resource `json:"resources"`
	NodePools            []NodePool `json:"nodePools"`
}

func Aggregate(clusterName string, nodes []corev1.Node, observedAt time.Time) Snapshot {
	return Build(clusterName, clusterName, nodes, observedAt)
}

func Build(clusterName, identitySeed string, nodes []corev1.Node, observedAt time.Time) Snapshot {
	if strings.TrimSpace(identitySeed) == "" {
		identitySeed = clusterName
	}
	type totals struct {
		allocatable int64
		schedulable int64
	}

	byResource := map[string]totals{}
	byPool := map[string][]Node{}
	schedulableNodes := 0
	for index := range nodes {
		node := &nodes[index]
		health := nodeHealth(node)
		schedulable := !node.Spec.Unschedulable && health == HealthHealthy
		if schedulable {
			schedulableNodes++
		}

		for resourceName, quantity := range node.Status.Allocatable {
			if !isNVIDIAAccelerator(resourceName) {
				continue
			}
			value := quantity.Value()
			if value <= 0 {
				continue
			}
			current := byResource[string(resourceName)]
			current.allocatable += value
			if schedulable {
				current.schedulable += value
			}
			byResource[string(resourceName)] = current
		}

		poolName := node.Labels[labelNodePool]
		if !validDNSLabel(poolName) {
			poolName = DefaultNodePool
		}
		detailed := detailedNode(identitySeed, node, health, schedulable)
		byPool[poolName] = append(byPool[poolName], detailed)
	}

	names := make([]string, 0, len(byResource))
	for name := range byResource {
		names = append(names, name)
	}
	sort.Strings(names)
	resources := make([]Resource, 0, len(names))
	for _, name := range names {
		resourceTotals := byResource[name]
		resources = append(resources, Resource{Name: name, Allocatable: resourceTotals.allocatable, SchedulableAllocatable: resourceTotals.schedulable})
	}

	poolNames := make([]string, 0, len(byPool))
	for name := range byPool {
		poolNames = append(poolNames, name)
	}
	sort.Strings(poolNames)
	pools := make([]NodePool, 0, len(poolNames))
	for _, name := range poolNames {
		poolNodes := byPool[name]
		sort.Slice(poolNodes, func(i, j int) bool { return poolNodes[i].OpaqueKey < poolNodes[j].OpaqueKey })
		pools = append(pools, NodePool{Name: name, ManagementState: ManagementEnabled, Nodes: poolNodes})
	}

	snapshot := Snapshot{
		SchemaVersion: SchemaVersion, ClusterName: clusterName, ObservedAt: observedAt.UTC(),
		ExecutionHealthy: true, NodeCount: len(nodes), SchedulableNodeCount: schedulableNodes,
		Resources: resources, NodePools: pools,
	}
	snapshot.Generation = calculateGeneration(snapshot)
	return snapshot
}

func Marshal(snapshot Snapshot) ([]byte, error) {
	return json.Marshal(snapshot)
}

func detailedNode(identitySeed string, node *corev1.Node, health HealthState, schedulable bool) Node {
	opaqueNode := opaqueKey(identitySeed, "node", node.Name)
	result := Node{
		OpaqueKey: opaqueNode, ManagementState: ManagementEnabled, HealthState: health,
		Schedulable: schedulable, Traits: nodeTraits(node.Labels), GPUDevices: []GPUDevice{},
	}
	count := node.Status.Allocatable[corev1.ResourceName("nvidia.com/gpu")].Value()
	model := strings.TrimSpace(node.Labels[labelGPUProduct])
	memoryMiB, memoryOK := parsePositiveInt64(node.Labels[labelGPUMemory])
	if count <= 0 || model == "" || !memoryOK {
		return result
	}
	deviceHealth := health
	for index := int64(0); index < count; index++ {
		result.GPUDevices = append(result.GPUDevices, GPUDevice{
			OpaqueKey:     opaqueKey(identitySeed, "gpu", node.Name+"\x00"+strconv.FormatInt(index, 10)),
			ResourceClass: WholeGPUResourceClass, Model: model, MemoryMiB: memoryMiB,
			AcceleratorMode: "whole", HealthState: deviceHealth, Allocatable: schedulable,
			Traits: gpuTraits(node.Labels, model, memoryMiB),
		})
	}
	return result
}

func nodeHealth(node *corev1.Node) HealthState {
	ready := HealthUnknown
	degraded := false
	for _, condition := range node.Status.Conditions {
		switch condition.Type {
		case corev1.NodeReady:
			switch condition.Status {
			case corev1.ConditionTrue:
				ready = HealthHealthy
			case corev1.ConditionFalse:
				ready = HealthFailed
			case corev1.ConditionUnknown:
				ready = HealthUnreachable
			}
		case corev1.NodeMemoryPressure, corev1.NodeDiskPressure, corev1.NodePIDPressure, corev1.NodeNetworkUnavailable:
			if condition.Status == corev1.ConditionTrue {
				degraded = true
			}
		}
	}
	if ready == HealthHealthy && degraded {
		return HealthDegraded
	}
	return ready
}

func nodeTraits(labels map[string]string) map[string]string {
	result := map[string]string{}
	copyTrait(result, labels, labelTopologyRegion, "topology.kubernetes.io/region")
	copyTrait(result, labels, labelTopologyZone, "topology.kubernetes.io/zone")
	copyTrait(result, labels, labelInstanceType, "node.kubernetes.io/instance-type")
	copyTrait(result, labels, labelCUDAMajor, "gpu.nvidia.com/cuda-runtime-major")
	copyTrait(result, labels, labelCUDAMinor, "gpu.nvidia.com/cuda-runtime-minor")
	copyTrait(result, labels, labelComputeMajor, "gpu.nvidia.com/compute-major")
	copyTrait(result, labels, labelComputeMinor, "gpu.nvidia.com/compute-minor")
	return result
}

func gpuTraits(labels map[string]string, model string, memoryMiB int64) map[string]string {
	result := map[string]string{
		"gpu.nvidia.com/model":      model,
		"gpu.nvidia.com/memory-mib": strconv.FormatInt(memoryMiB, 10),
	}
	if major, minor := strings.TrimSpace(labels[labelCUDAMajor]), strings.TrimSpace(labels[labelCUDAMinor]); major != "" && minor != "" {
		result["gpu.nvidia.com/cuda-runtime"] = major + "." + minor
	}
	if major, minor := strings.TrimSpace(labels[labelComputeMajor]), strings.TrimSpace(labels[labelComputeMinor]); major != "" && minor != "" {
		result["gpu.nvidia.com/compute-capability"] = major + "." + minor
	}
	if value := strings.TrimSpace(labels[labelMIGCapable]); value != "" {
		result["gpu.nvidia.com/mig-capable"] = value
	}
	return result
}

func copyTrait(target, source map[string]string, sourceKey, targetKey string) {
	if value := strings.TrimSpace(source[sourceKey]); value != "" {
		target[targetKey] = value
	}
}

func isNVIDIAAccelerator(name corev1.ResourceName) bool {
	value := string(name)
	return value == "nvidia.com/gpu" || strings.HasPrefix(value, "nvidia.com/mig-")
}

func calculateGeneration(snapshot Snapshot) string {
	content := struct {
		ClusterName          string     `json:"clusterName"`
		NodeCount            int        `json:"nodeCount"`
		SchedulableNodeCount int        `json:"schedulableNodeCount"`
		Resources            []Resource `json:"resources"`
		NodePools            []NodePool `json:"nodePools"`
	}{snapshot.ClusterName, snapshot.NodeCount, snapshot.SchedulableNodeCount, snapshot.Resources, snapshot.NodePools}
	canonical, _ := json.Marshal(content)
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:])
}

func opaqueKey(seed, kind, value string) string {
	mac := hmac.New(sha256.New, []byte(seed))
	mac.Write([]byte(kind))
	mac.Write([]byte{0})
	mac.Write([]byte(value))
	return kind + "-" + hex.EncodeToString(mac.Sum(nil)[:16])
}

func parsePositiveInt64(value string) (int64, bool) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed, err == nil && parsed > 0
}

func validDNSLabel(value string) bool {
	if len(value) == 0 || len(value) > 63 || !isLowerAlnum(value[0]) || !isLowerAlnum(value[len(value)-1]) {
		return false
	}
	for index := range value {
		if !isLowerAlnum(value[index]) && value[index] != '-' {
			return false
		}
	}
	return true
}

func isLowerAlnum(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}
