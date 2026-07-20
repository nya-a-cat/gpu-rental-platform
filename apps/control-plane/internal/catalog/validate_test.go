package catalog

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func validInventoryParams() ReplaceInventoryParams {
	return ReplaceInventoryParams{
		ExpectedGeneration: 0,
		SourceGeneration:   strings.Repeat("a", 64),
		AgentEpoch:         "epoch-0001",
		ReportSequence:     1,
		FencingToken:       "fence-1",
		FencingEnabled:     true,
		ExecutionHealthy:   true,
		ObservedAt:         time.Now().UTC(),
		NodePools: []NodePoolSnapshot{{
			Name: "pool-a", ManagementState: ManagementEnabled,
			Nodes: []NodeSnapshot{{
				OpaqueKey: "node-opaque", ManagementState: ManagementEnabled, HealthState: HealthHealthy, Schedulable: true,
				Traits: map[string]string{"gpu.nvidia.com/product": "A100"},
				GPUDevices: []GPUDeviceSnapshot{{
					OpaqueKey: "gpu-opaque", ResourceClass: WholeGPUResourceClass, Model: "NVIDIA A100",
					MemoryMiB: 40960, AcceleratorMode: AcceleratorWhole, HealthState: HealthHealthy, Allocatable: true,
				}},
			}},
		}},
	}
}

func TestValidateInventoryAcceptsWholeGPU(t *testing.T) {
	if err := ValidateInventory(validInventoryParams()); err != nil {
		t.Fatalf("ValidateInventory() error = %v", err)
	}
}

func TestValidateInventoryRejectsUppercaseGeneration(t *testing.T) {
	params := validInventoryParams()
	params.SourceGeneration = strings.Repeat("A", 64)
	if err := ValidateInventory(params); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ValidateInventory() error = %v, want ErrInvalid", err)
	}
}

func TestValidateInventoryRejectsDuplicateOpaqueNode(t *testing.T) {
	params := validInventoryParams()
	params.NodePools = append(params.NodePools, NodePoolSnapshot{
		Name: "pool-b", ManagementState: ManagementEnabled,
		Nodes: []NodeSnapshot{{OpaqueKey: "node-opaque", ManagementState: ManagementEnabled, HealthState: HealthHealthy}},
	})
	if err := ValidateInventory(params); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ValidateInventory() error = %v, want ErrInvalid", err)
	}
}

func TestValidateAcceleratorProfileRealAlphaBoundary(t *testing.T) {
	params := CreateAcceleratorProfileParams{
		Name: "One A100", Slug: "one-a100", AcceleratorMode: AcceleratorWhole,
		ResourceClass: WholeGPUResourceClass, GPUCount: 1,
	}
	if err := ValidateAcceleratorProfile(params); err != nil {
		t.Fatalf("ValidateAcceleratorProfile() error = %v", err)
	}
	params.AcceleratorMode = AcceleratorMIG
	if err := ValidateAcceleratorProfile(params); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ValidateAcceleratorProfile() error = %v, want ErrInvalid", err)
	}
}
