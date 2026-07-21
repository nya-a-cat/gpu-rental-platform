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
	ErrConflict = errors.New("billing resource conflict")
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

type LedgerEntry struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenantId"`
	ProjectID   string    `json:"projectId"`
	UsageFactID string    `json:"usageFactId"`
	EntryType   string    `json:"entryType"`
	AmountMinor int64     `json:"amountMinor"`
	Currency    string    `json:"currency"`
	ReferenceID string    `json:"referenceId"`
	CreatedAt   time.Time `json:"createdAt"`
}

type InvoiceLine struct {
	ID          string `json:"id"`
	UsageFactID string `json:"usageFactId"`
	AmountMinor int64  `json:"amountMinor"`
	Currency    string `json:"currency"`
}

type Invoice struct {
	ID            string        `json:"id"`
	TenantID      string        `json:"tenantId"`
	ProjectID     string        `json:"projectId"`
	PeriodFrom    time.Time     `json:"periodFrom"`
	PeriodTo      time.Time     `json:"periodTo"`
	Currency      string        `json:"currency"`
	SubtotalMinor int64         `json:"subtotalMinor"`
	Status        string        `json:"status"`
	Lines         []InvoiceLine `json:"lines"`
	CreatedAt     time.Time     `json:"createdAt"`
	UpdatedAt     time.Time     `json:"updatedAt"`
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

type CreateInvoiceParams struct {
	Mutation   tenancy.MutationContext
	TenantID   string
	ProjectID  string
	PeriodFrom time.Time
	PeriodTo   time.Time
}

type Repository interface {
	CreateUsageFact(context.Context, CreateUsageFactParams) (tenancy.Acceptance, error)
	GetUsageFact(context.Context, string) (UsageFact, error)
	CreateInvoice(context.Context, CreateInvoiceParams) (tenancy.Acceptance, error)
	GetInvoice(context.Context, string) (Invoice, error)
}

func toPortUsageFact(fact UsageFact) ports.UsageFact {
	return ports.UsageFact{ID: fact.ID, TenantID: fact.TenantID, ProjectID: fact.ProjectID, ResourceClass: fact.ResourceClass, Quantity: fact.Quantity, AllocationFrom: fact.AllocationFrom, AllocationTo: fact.AllocationTo, Attributes: fact.Attributes}
}
