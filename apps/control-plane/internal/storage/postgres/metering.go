package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/identity"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/metering"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/ports"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

var meteringResourceClassPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

func (repository *Repository) CreateUsageFact(ctx context.Context, params metering.CreateUsageFactParams) (tenancy.Acceptance, error) {
	if repository.billing == nil {
		return tenancy.Acceptance{}, fmt.Errorf("billing engine is unavailable: %w", metering.ErrInvalid)
	}
	if !identity.IsUUID(params.TenantID) || !identity.IsUUID(params.ProjectID) {
		return tenancy.Acceptance{}, metering.ErrNotFound
	}
	resourceClass := strings.TrimSpace(params.ResourceClass)
	quantity := strings.TrimSpace(params.Quantity)
	if !meteringResourceClassPattern.MatchString(resourceClass) || quantity == "" || params.AllocationFrom.IsZero() || params.AllocationTo.IsZero() || !params.AllocationTo.After(params.AllocationFrom) {
		return tenancy.Acceptance{}, metering.ErrInvalid
	}
	attributes := params.Attributes
	if len(attributes) == 0 {
		attributes = json.RawMessage(`{}`)
	}
	if !json.Valid(attributes) {
		return tenancy.Acceptance{}, fmt.Errorf("usage attributes must be valid JSON: %w", metering.ErrInvalid)
	}
	factID, err := identity.NewUUID()
	if err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("generate usage fact ID: %w", err)
	}
	fact := metering.UsageFact{ID: factID, TenantID: params.TenantID, ProjectID: params.ProjectID, ResourceClass: resourceClass, Quantity: quantity, AllocationFrom: params.AllocationFrom.UTC(), AllocationTo: params.AllocationTo.UTC(), Attributes: attributes}
	rated, err := repository.billing.RateUsage(ctx, meteringToPortUsageFact(fact))
	if err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("rate usage fact: %w", err)
	}
	return repository.acceptMutation(ctx, params.Mutation, acceptedMutationSpec{
		kind: "usage-fact.create", resourceType: "usage-fact", resourceID: factID, eventType: "usage-fact.created",
		scopeType: string(tenancy.ScopeProject), scopeID: params.ProjectID,
		eventFields:         map[string]any{"tenantId": params.TenantID, "projectId": params.ProjectID, "resourceClass": resourceClass, "allocationFrom": fact.AllocationFrom, "allocationTo": fact.AllocationTo},
		completeImmediately: true,
		apply: func(ctx context.Context, transaction *sql.Tx, now time.Time) error {
			var projectTenant string
			if err := transaction.QueryRowContext(ctx, `SELECT tenant_id::text FROM projects WHERE id = $1`, params.ProjectID).Scan(&projectTenant); errors.Is(err, sql.ErrNoRows) {
				return metering.ErrNotFound
			} else if err != nil {
				return fmt.Errorf("load usage project: %w", err)
			}
			if projectTenant != params.TenantID {
				return metering.ErrNotFound
			}
			if _, err := transaction.ExecContext(ctx, `INSERT INTO usage_facts (id, tenant_id, project_id, resource_class, quantity, allocation_from, allocation_to, attributes, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, fact.ID, fact.TenantID, fact.ProjectID, fact.ResourceClass, fact.Quantity, fact.AllocationFrom, fact.AllocationTo, fact.Attributes, now); err != nil {
				return mapWorkspaceWriteError(err)
			}
			_, err := transaction.ExecContext(ctx, `INSERT INTO rated_usage (usage_fact_id, price_book_id, price_version, amount_minor, currency, calculation, calculated_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`, rated.UsageFactID, rated.PriceBookID, rated.PriceVersion, rated.AmountMinor, rated.Currency, rated.Calculation, rated.CalculatedAt)
			return mapWorkspaceWriteError(err)
		},
	})
}

func (repository *Repository) GetUsageFact(ctx context.Context, id string) (metering.UsageFact, error) {
	if !identity.IsUUID(id) {
		return metering.UsageFact{}, metering.ErrNotFound
	}
	var result metering.UsageFact
	var rated metering.RatedUsage
	err := repository.database.QueryRowContext(ctx, `SELECT f.id::text, f.tenant_id::text, f.project_id::text, f.resource_class, f.quantity, f.allocation_from, f.allocation_to, f.attributes, f.created_at, r.usage_fact_id::text, r.price_book_id, r.price_version, r.amount_minor, r.currency, r.calculation, r.calculated_at FROM usage_facts f JOIN rated_usage r ON r.usage_fact_id = f.id WHERE f.id = $1`, id).Scan(&result.ID, &result.TenantID, &result.ProjectID, &result.ResourceClass, &result.Quantity, &result.AllocationFrom, &result.AllocationTo, &result.Attributes, &result.CreatedAt, &rated.UsageFactID, &rated.PriceBookID, &rated.PriceVersion, &rated.AmountMinor, &rated.Currency, &rated.Calculation, &rated.CalculatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return metering.UsageFact{}, metering.ErrNotFound
	}
	if err != nil {
		return metering.UsageFact{}, fmt.Errorf("get usage fact: %w", err)
	}
	result.RatedUsage = &rated
	return result, nil
}

func meteringToPortUsageFact(fact metering.UsageFact) ports.UsageFact {
	return ports.UsageFact{ID: fact.ID, TenantID: fact.TenantID, ProjectID: fact.ProjectID, ResourceClass: fact.ResourceClass, Quantity: fact.Quantity, AllocationFrom: fact.AllocationFrom, AllocationTo: fact.AllocationTo, Attributes: fact.Attributes}
}
