package metering

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/ports"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

var (
	ErrInvalid  = errors.New("invalid usage fact")
	ErrNotFound = errors.New("usage fact not found")
)

type UsageFact struct {
	ID             string          `json:"id"`
	TenantID       string          `json:"tenantId"`
	ProjectID      string          `json:"projectId"`
	ResourceClass  string          `json:"resourceClass"`
	Quantity       string          `json:"quantity"`
	AllocationFrom time.Time       `json:"allocationFrom"`
	AllocationTo   time.Time       `json:"allocationTo"`
	Attributes     json.RawMessage `json:"attributes"`
	CreatedAt      time.Time       `json:"createdAt"`
	RatedUsage     *RatedUsage     `json:"ratedUsage"`
}

type RatedUsage struct {
	UsageFactID  string          `json:"usageFactId"`
	PriceBookID  string          `json:"priceBookId"`
	PriceVersion int             `json:"priceVersion"`
	AmountMinor  int64           `json:"amountMinor"`
	Currency     string          `json:"currency"`
	Calculation  json.RawMessage `json:"calculation"`
	CalculatedAt time.Time       `json:"calculatedAt"`
}

type CreateUsageFactParams struct {
	Mutation       tenancy.MutationContext
	TenantID       string
	ProjectID      string
	ResourceClass  string
	Quantity       string
	AllocationFrom time.Time
	AllocationTo   time.Time
	Attributes     json.RawMessage
}

type Repository interface {
	CreateUsageFact(context.Context, CreateUsageFactParams) (tenancy.Acceptance, error)
	GetUsageFact(context.Context, string) (UsageFact, error)
}

func toPortUsageFact(fact UsageFact) ports.UsageFact {
	return ports.UsageFact{ID: fact.ID, TenantID: fact.TenantID, ProjectID: fact.ProjectID, ResourceClass: fact.ResourceClass, Quantity: fact.Quantity, AllocationFrom: fact.AllocationFrom, AllocationTo: fact.AllocationTo, Attributes: fact.Attributes}
}
