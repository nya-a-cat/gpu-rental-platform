package catalog

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

var (
	dnsSubdomainPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`)
	dnsLabelPattern     = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	slugPattern         = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)
	traitKeyPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]{0,127}$`)
)

func ValidateInventory(params ReplaceInventoryParams) error {
	if params.ExpectedGeneration < 0 {
		return invalid("expected generation must be non-negative")
	}
	if len(params.AgentEpoch) < 8 || len(params.AgentEpoch) > 128 {
		return invalid("agent epoch must contain between 8 and 128 characters")
	}
	if params.ReportSequence == 0 {
		return invalid("report sequence must be greater than zero")
	}
	if len(params.FencingToken) > 255 {
		return invalid("fencing token must contain at most 255 characters")
	}
	if params.FencingEnabled && strings.TrimSpace(params.FencingToken) == "" {
		return invalid("fencing token is required when fencing is enabled")
	}
	if len(params.SourceGeneration) != 64 {
		return invalid("source generation must be a SHA-256 hex digest")
	}
	if params.SourceGeneration != strings.ToLower(params.SourceGeneration) {
		return invalid("source generation must use lowercase SHA-256 hex")
	}
	if _, err := hex.DecodeString(params.SourceGeneration); err != nil {
		return invalid("source generation must be a SHA-256 hex digest")
	}
	if params.ObservedAt.IsZero() {
		return invalid("inventory observation time is required")
	}

	poolNames := map[string]struct{}{}
	nodeKeys := map[string]struct{}{}
	for _, pool := range params.NodePools {
		if !validDNSLabel(pool.Name) {
			return invalid("node pool name must be a DNS label")
		}
		if !validManagementState(pool.ManagementState) {
			return invalid("node pool management state is invalid")
		}
		if _, exists := poolNames[pool.Name]; exists {
			return invalid("node pool names must be unique within a snapshot")
		}
		poolNames[pool.Name] = struct{}{}
		for _, node := range pool.Nodes {
			if err := validateOpaqueKey("node", node.OpaqueKey); err != nil {
				return err
			}
			if _, exists := nodeKeys[node.OpaqueKey]; exists {
				return invalid("node opaque keys must be unique within a cluster snapshot")
			}
			nodeKeys[node.OpaqueKey] = struct{}{}
			if !validManagementState(node.ManagementState) || !validHealthState(node.HealthState) {
				return invalid("node management or health state is invalid")
			}
			if err := validateTraits(node.Traits); err != nil {
				return err
			}
			deviceKeys := map[string]struct{}{}
			for _, device := range node.GPUDevices {
				if err := validateOpaqueKey("GPU device", device.OpaqueKey); err != nil {
					return err
				}
				if _, exists := deviceKeys[device.OpaqueKey]; exists {
					return invalid("GPU device opaque keys must be unique within a node snapshot")
				}
				deviceKeys[device.OpaqueKey] = struct{}{}
				if device.ResourceClass != WholeGPUResourceClass || device.AcceleratorMode != AcceleratorWhole {
					return invalid("Real Alpha inventory accepts whole NVIDIA GPU devices only")
				}
				if strings.TrimSpace(device.Model) == "" || len(device.Model) > 120 || device.MemoryMiB <= 0 {
					return invalid("GPU device model and memory are required")
				}
				if !validHealthState(device.HealthState) {
					return invalid("GPU device health state is invalid")
				}
				if err := validateTraits(device.Traits); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func ValidateCluster(managedClusterName, displayName string) error {
	if len(managedClusterName) == 0 || len(managedClusterName) > 253 || !dnsSubdomainPattern.MatchString(managedClusterName) {
		return invalid("managed cluster name must be a DNS-compatible resource name")
	}
	if strings.TrimSpace(displayName) == "" || len(displayName) > 120 {
		return invalid("cluster display name must contain between 1 and 120 characters")
	}
	return nil
}

func ValidateAcceleratorProfile(params CreateAcceleratorProfileParams) error {
	if strings.TrimSpace(params.Name) == "" || len(params.Name) > 120 || !slugPattern.MatchString(params.Slug) {
		return invalid("accelerator profile name or slug is invalid")
	}
	if params.AcceleratorMode != AcceleratorWhole || params.ResourceClass != WholeGPUResourceClass {
		return invalid("Real Alpha accelerator profiles support whole NVIDIA GPUs only")
	}
	if params.GPUCount <= 0 || params.GPUCount > 64 {
		return invalid("accelerator profile GPU count must be between 1 and 64")
	}
	if params.MemoryMiB != nil && *params.MemoryMiB <= 0 {
		return invalid("accelerator profile memory must be greater than zero")
	}
	return validateTraits(params.Traits)
}

func ValidateCapacityPool(params CreateCapacityPoolParams) error {
	if strings.TrimSpace(params.Name) == "" || len(params.Name) > 120 {
		return invalid("capacity pool name must contain between 1 and 120 characters")
	}
	if params.SchedulerProfile != SchedulerNone && params.SchedulerProfile != SchedulerVolcano && params.SchedulerProfile != SchedulerKueue {
		return invalid("capacity pool scheduler profile is invalid")
	}
	return nil
}

func validManagementState(value ManagementState) bool {
	switch value {
	case ManagementEnabled, ManagementDisabled, ManagementDraining, ManagementMaintenance, ManagementQuarantined:
		return true
	default:
		return false
	}
}

func validHealthState(value HealthState) bool {
	switch value {
	case HealthHealthy, HealthDegraded, HealthUnreachable, HealthFailed, HealthUnknown:
		return true
	default:
		return false
	}
}

func validDNSLabel(value string) bool {
	return len(value) > 0 && len(value) <= 63 && dnsLabelPattern.MatchString(value)
}

func validateOpaqueKey(kind, value string) error {
	if strings.TrimSpace(value) == "" || len(value) > 128 {
		return invalid(fmt.Sprintf("%s opaque key must contain between 1 and 128 characters", kind))
	}
	return nil
}

func validateTraits(traits map[string]string) error {
	if len(traits) > 64 {
		return invalid("a resource may contain at most 64 traits")
	}
	for key, value := range traits {
		if !traitKeyPattern.MatchString(key) || strings.TrimSpace(value) == "" || len(value) > 255 {
			return invalid("resource trait key or value is invalid")
		}
	}
	return nil
}

func invalid(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalid, message)
}
