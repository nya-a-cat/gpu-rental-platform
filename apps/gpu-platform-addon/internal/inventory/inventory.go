package inventory

import (
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
)

type Resource struct {
	Name                   string `json:"name"`
	Allocatable            int64  `json:"allocatable"`
	SchedulableAllocatable int64  `json:"schedulableAllocatable"`
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
	NodeCount            int        `json:"nodeCount"`
	SchedulableNodeCount int        `json:"schedulableNodeCount"`
	Resources            []Resource `json:"resources"`
}

func Aggregate(clusterName string, nodes []corev1.Node, observedAt time.Time) Snapshot {
	type totals struct {
		allocatable int64
		schedulable int64
	}

	byResource := map[string]totals{}
	schedulableNodes := 0
	for i := range nodes {
		node := &nodes[i]
		isSchedulable := !node.Spec.Unschedulable
		if isSchedulable {
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
			if isSchedulable {
				current.schedulable += value
			}
			byResource[string(resourceName)] = current
		}
	}

	names := make([]string, 0, len(byResource))
	for name := range byResource {
		names = append(names, name)
	}
	sort.Strings(names)

	resources := make([]Resource, 0, len(names))
	for _, name := range names {
		resourceTotals := byResource[name]
		resources = append(resources, Resource{
			Name:                   name,
			Allocatable:            resourceTotals.allocatable,
			SchedulableAllocatable: resourceTotals.schedulable,
		})
	}

	snapshot := Snapshot{
		SchemaVersion:        SchemaVersion,
		ClusterName:          clusterName,
		ObservedAt:           observedAt.UTC(),
		NodeCount:            len(nodes),
		SchedulableNodeCount: schedulableNodes,
		Resources:            resources,
	}
	snapshot.Generation = calculateGeneration(snapshot)
	return snapshot
}

func Marshal(snapshot Snapshot) ([]byte, error) {
	return json.Marshal(snapshot)
}

func isNVIDIAAccelerator(name corev1.ResourceName) bool {
	value := string(name)
	return value == "nvidia.com/gpu" || strings.HasPrefix(value, "nvidia.com/mig-")
}

func calculateGeneration(snapshot Snapshot) string {
	var canonical strings.Builder
	canonical.WriteString(snapshot.ClusterName)
	canonical.WriteByte(0)
	canonical.WriteString(strconv.Itoa(snapshot.NodeCount))
	canonical.WriteByte(0)
	canonical.WriteString(strconv.Itoa(snapshot.SchedulableNodeCount))
	canonical.WriteByte(0)
	for _, resource := range snapshot.Resources {
		canonical.WriteString(resource.Name)
		canonical.WriteByte(0)
		canonical.WriteString(strconv.FormatInt(resource.Allocatable, 10))
		canonical.WriteByte(0)
		canonical.WriteString(strconv.FormatInt(resource.SchedulableAllocatable, 10))
		canonical.WriteByte(0)
	}

	digest := sha256.Sum256([]byte(canonical.String()))
	return hex.EncodeToString(digest[:])
}
