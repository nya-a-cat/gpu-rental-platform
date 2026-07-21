package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/identity"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/metering"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

func (repository *Repository) CreateInvoice(ctx context.Context, params metering.CreateInvoiceParams) (tenancy.Acceptance, error) {
	if !identity.IsUUID(params.TenantID) || !identity.IsUUID(params.ProjectID) || params.PeriodFrom.IsZero() || params.PeriodTo.IsZero() || !params.PeriodTo.After(params.PeriodFrom) {
		return tenancy.Acceptance{}, metering.ErrInvalid
	}
	invoiceID, err := identity.NewUUID()
	if err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("generate invoice ID: %w", err)
	}
	periodFrom, periodTo := params.PeriodFrom.UTC(), params.PeriodTo.UTC()
	return repository.acceptMutation(ctx, params.Mutation, acceptedMutationSpec{
		kind: "invoice.create", resourceType: "invoice", resourceID: invoiceID, eventType: "invoice.created",
		scopeType: string(tenancy.ScopeProject), scopeID: params.ProjectID,
		eventFields:         map[string]any{"tenantId": params.TenantID, "projectId": params.ProjectID, "periodFrom": periodFrom, "periodTo": periodTo},
		completeImmediately: true,
		apply: func(ctx context.Context, transaction *sql.Tx, now time.Time) error {
			var projectTenant string
			if err := transaction.QueryRowContext(ctx, `SELECT tenant_id::text FROM projects WHERE id = $1`, params.ProjectID).Scan(&projectTenant); errors.Is(err, sql.ErrNoRows) {
				return metering.ErrNotFound
			} else if err != nil {
				return fmt.Errorf("load invoice project: %w", err)
			}
			if projectTenant != params.TenantID {
				return metering.ErrNotFound
			}
			rows, err := transaction.QueryContext(ctx, `SELECT r.usage_fact_id::text, r.amount_minor, r.currency FROM rated_usage r JOIN usage_facts f ON f.id = r.usage_fact_id WHERE f.project_id = $1 AND f.allocation_from < $2 AND f.allocation_to > $3 ORDER BY f.allocation_from, r.usage_fact_id`, params.ProjectID, periodTo, periodFrom)
			if err != nil {
				return fmt.Errorf("list invoice usage: %w", err)
			}
			defer rows.Close()
			type line struct {
				usageFactID, currency string
				amount                int64
			}
			lines := []line{}
			var subtotal int64
			currency := "CNY"
			for rows.Next() {
				var current line
				if err := rows.Scan(&current.usageFactID, &current.amount, &current.currency); err != nil {
					return fmt.Errorf("scan invoice usage: %w", err)
				}
				if currency == "CNY" {
					currency = current.currency
				} else if currency != current.currency {
					return fmt.Errorf("invoice contains multiple currencies: %w", metering.ErrInvalid)
				}
				if current.amount > 0 && subtotal > math.MaxInt64-current.amount {
					return fmt.Errorf("invoice subtotal overflow: %w", metering.ErrInvalid)
				}
				subtotal += current.amount
				lines = append(lines, current)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterate invoice usage: %w", err)
			}
			if err := rows.Close(); err != nil {
				return fmt.Errorf("close invoice usage rows: %w", err)
			}
			if _, err := transaction.ExecContext(ctx, `INSERT INTO invoices (id, tenant_id, project_id, period_from, period_to, currency, subtotal_minor, status, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,'issued',$8,$8)`, invoiceID, params.TenantID, params.ProjectID, periodFrom, periodTo, currency, subtotal, now); err != nil {
				if strings.Contains(err.Error(), "duplicate key") {
					return metering.ErrConflict
				}
				return mapWorkspaceWriteError(err)
			}
			for _, current := range lines {
				lineID, err := identity.NewUUID()
				if err != nil {
					return fmt.Errorf("generate invoice line ID: %w", err)
				}
				if _, err := transaction.ExecContext(ctx, `INSERT INTO invoice_lines (id, invoice_id, usage_fact_id, amount_minor, currency) VALUES ($1,$2,$3,$4,$5)`, lineID, invoiceID, current.usageFactID, current.amount, current.currency); err != nil {
					return mapWorkspaceWriteError(err)
				}
			}
			return nil
		},
	})
}

func (repository *Repository) GetInvoice(ctx context.Context, id string) (metering.Invoice, error) {
	if !identity.IsUUID(id) {
		return metering.Invoice{}, metering.ErrNotFound
	}
	var result metering.Invoice
	if err := repository.database.QueryRowContext(ctx, `SELECT id::text, tenant_id::text, project_id::text, period_from, period_to, currency, subtotal_minor, status, created_at, updated_at FROM invoices WHERE id = $1`, id).Scan(&result.ID, &result.TenantID, &result.ProjectID, &result.PeriodFrom, &result.PeriodTo, &result.Currency, &result.SubtotalMinor, &result.Status, &result.CreatedAt, &result.UpdatedAt); errors.Is(err, sql.ErrNoRows) {
		return metering.Invoice{}, metering.ErrNotFound
	} else if err != nil {
		return metering.Invoice{}, fmt.Errorf("get invoice: %w", err)
	}
	rows, err := repository.database.QueryContext(ctx, `SELECT id::text, usage_fact_id::text, amount_minor, currency FROM invoice_lines WHERE invoice_id = $1 ORDER BY id`, id)
	if err != nil {
		return metering.Invoice{}, fmt.Errorf("list invoice lines: %w", err)
	}
	defer rows.Close()
	result.Lines = []metering.InvoiceLine{}
	for rows.Next() {
		var line metering.InvoiceLine
		if err := rows.Scan(&line.ID, &line.UsageFactID, &line.AmountMinor, &line.Currency); err != nil {
			return metering.Invoice{}, fmt.Errorf("scan invoice line: %w", err)
		}
		result.Lines = append(result.Lines, line)
	}
	if err := rows.Err(); err != nil {
		return metering.Invoice{}, fmt.Errorf("iterate invoice lines: %w", err)
	}
	return result, nil
}
