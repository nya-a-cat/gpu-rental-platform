package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/ports"
)

var (
	ErrInvalidUsage = errors.New("invalid usage fact")
	ErrUnknownRate  = errors.New("billing rate is not configured")
)

// FixedRate is a versioned price per resource unit-second in minor currency
// units. Rates are kept in memory by the initial engine; the persisted price
// book adapter can implement the same ports.BillingEngine contract later.
type FixedRate struct {
	ResourceClass string
	PriceBookID   string
	PriceVersion  int
	MinorPerUnit  int64
	Currency      string
}

type FixedEngine struct {
	rates map[string]FixedRate
}

func NewFixedEngine(rates []FixedRate) (*FixedEngine, error) {
	configured := make(map[string]FixedRate, len(rates))
	for _, rate := range rates {
		resourceClass := strings.TrimSpace(rate.ResourceClass)
		currency := strings.ToUpper(strings.TrimSpace(rate.Currency))
		if resourceClass == "" || strings.TrimSpace(rate.PriceBookID) == "" || rate.PriceVersion < 1 || rate.MinorPerUnit <= 0 || len(currency) != 3 {
			return nil, fmt.Errorf("invalid fixed billing rate: %w", ErrInvalidUsage)
		}
		if currency < "AAA" || currency > "ZZZ" {
			return nil, fmt.Errorf("invalid fixed billing currency: %w", ErrInvalidUsage)
		}
		if _, exists := configured[resourceClass]; exists {
			return nil, fmt.Errorf("duplicate fixed billing rate for %s: %w", resourceClass, ErrInvalidUsage)
		}
		rate.ResourceClass = resourceClass
		rate.Currency = currency
		configured[resourceClass] = rate
	}
	return &FixedEngine{rates: configured}, nil
}

func (engine *FixedEngine) RateUsage(_ context.Context, fact ports.UsageFact) (ports.RatedUsage, error) {
	if engine == nil {
		return ports.RatedUsage{}, fmt.Errorf("billing engine is nil: %w", ErrInvalidUsage)
	}
	rate, ok := engine.rates[strings.TrimSpace(fact.ResourceClass)]
	if !ok {
		return ports.RatedUsage{}, fmt.Errorf("%s: %w", fact.ResourceClass, ErrUnknownRate)
	}
	if strings.TrimSpace(fact.ID) == "" || strings.TrimSpace(fact.ResourceClass) == "" || fact.TenantID == "" || fact.ProjectID == "" || fact.AllocationFrom.IsZero() || fact.AllocationTo.IsZero() || !fact.AllocationTo.After(fact.AllocationFrom) {
		return ports.RatedUsage{}, fmt.Errorf("usage interval and ownership are required: %w", ErrInvalidUsage)
	}
	quantity, ok := new(big.Rat).SetString(strings.TrimSpace(fact.Quantity))
	if !ok || quantity.Sign() <= 0 {
		return ports.RatedUsage{}, fmt.Errorf("usage quantity must be positive: %w", ErrInvalidUsage)
	}
	duration := new(big.Rat).SetInt64(fact.AllocationTo.Sub(fact.AllocationFrom).Nanoseconds())
	duration.Quo(duration, big.NewRat(int64(time.Second), 1))
	amount := new(big.Rat).Mul(quantity, duration)
	amount.Mul(amount, new(big.Rat).SetInt64(rate.MinorPerUnit))
	minor, err := ceilRatInt64(amount)
	if err != nil {
		return ports.RatedUsage{}, err
	}
	calculation, err := json.Marshal(map[string]any{
		"model":              "fixed-per-unit-second",
		"resourceClass":      rate.ResourceClass,
		"quantity":           fact.Quantity,
		"durationSeconds":    duration.FloatString(9),
		"minorPerUnitSecond": rate.MinorPerUnit,
		"rounding":           "ceiling",
	})
	if err != nil {
		return ports.RatedUsage{}, fmt.Errorf("encode billing calculation: %w", err)
	}
	return ports.RatedUsage{
		UsageFactID:  fact.ID,
		PriceBookID:  rate.PriceBookID,
		PriceVersion: rate.PriceVersion,
		AmountMinor:  minor,
		Currency:     rate.Currency,
		Calculation:  calculation,
		CalculatedAt: time.Now().UTC(),
	}, nil
}

func ceilRatInt64(value *big.Rat) (int64, error) {
	if value.Sign() < 0 || !value.IsInt() && value.Num().BitLen() > 62 {
		return 0, fmt.Errorf("billing amount is outside int64 range: %w", ErrInvalidUsage)
	}
	numerator := new(big.Int).Set(value.Num())
	denominator := value.Denom()
	if denominator.Sign() <= 0 {
		return 0, fmt.Errorf("billing amount denominator is invalid: %w", ErrInvalidUsage)
	}
	quotient, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	if remainder.Sign() != 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() {
		return 0, fmt.Errorf("billing amount is outside int64 range: %w", ErrInvalidUsage)
	}
	return quotient.Int64(), nil
}
