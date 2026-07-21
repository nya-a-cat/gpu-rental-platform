package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/ports"
)

func TestFixedEngineRatesByResourceSecondsAndCeils(t *testing.T) {
	engine, err := NewFixedEngine([]FixedRate{{ResourceClass: "gpu.nvidia.full", PriceBookID: "gpu-standard", PriceVersion: 3, MinorPerUnit: 7, Currency: "cny"}})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Unix(100, 0).UTC()
	rated, err := engine.RateUsage(context.Background(), ports.UsageFact{
		ID: "usage-1", TenantID: "tenant-1", ProjectID: "project-1", ResourceClass: "gpu.nvidia.full", Quantity: "2",
		AllocationFrom: start, AllocationTo: start.Add(1500 * time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rated.AmountMinor != 21 || rated.Currency != "CNY" || rated.PriceVersion != 3 {
		t.Fatalf("rated usage = %#v", rated)
	}
}

func TestFixedEngineRejectsUnknownRateAndInvalidInterval(t *testing.T) {
	engine, err := NewFixedEngine([]FixedRate{{ResourceClass: "gpu.nvidia.full", PriceBookID: "gpu-standard", PriceVersion: 1, MinorPerUnit: 1, Currency: "CNY"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.RateUsage(context.Background(), ports.UsageFact{ID: "usage-1", TenantID: "tenant-1", ProjectID: "project-1", ResourceClass: "cpu", Quantity: "1", AllocationFrom: time.Unix(1, 0), AllocationTo: time.Unix(2, 0)}); !errors.Is(err, ErrUnknownRate) {
		t.Fatalf("unknown rate error = %v", err)
	}
	if _, err := engine.RateUsage(context.Background(), ports.UsageFact{ID: "usage-1", TenantID: "tenant-1", ProjectID: "project-1", ResourceClass: "gpu.nvidia.full", Quantity: "1", AllocationFrom: time.Unix(2, 0), AllocationTo: time.Unix(1, 0)}); !errors.Is(err, ErrInvalidUsage) {
		t.Fatalf("invalid interval error = %v", err)
	}
}
