package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/identity"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/metering"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

func (repository *Repository) CreateCreditAdjustment(ctx context.Context, params metering.CreateCreditAdjustmentParams) (tenancy.Acceptance, error) {
	if !identity.IsUUID(params.TenantID) || !identity.IsUUID(params.ProjectID) || params.AmountMinor <= 0 || len(strings.TrimSpace(params.Currency)) != 3 || strings.TrimSpace(params.ReferenceID) == "" || len(params.ReferenceID) > 255 {
		return tenancy.Acceptance{}, metering.ErrInvalid
	}
	currency := strings.ToUpper(strings.TrimSpace(params.Currency))
	description := strings.TrimSpace(params.Description)
	if len(description) > 1024 {
		return tenancy.Acceptance{}, metering.ErrInvalid
	}
	entryID, err := identity.NewUUID()
	if err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("generate credit entry ID: %w", err)
	}
	return repository.acceptMutation(ctx, params.Mutation, acceptedMutationSpec{
		kind: "ledger.credit-adjustment", resourceType: "ledger-entry", resourceID: entryID, eventType: "ledger.credit.created",
		scopeType: string(tenancy.ScopeProject), scopeID: params.ProjectID,
		eventFields:         map[string]any{"tenantId": params.TenantID, "projectId": params.ProjectID, "amountMinor": params.AmountMinor, "currency": currency, "referenceId": params.ReferenceID},
		completeImmediately: true,
		apply: func(ctx context.Context, transaction *sql.Tx, now time.Time) error {
			var projectTenant string
			if err := transaction.QueryRowContext(ctx, `SELECT tenant_id::text FROM projects WHERE id = $1`, params.ProjectID).Scan(&projectTenant); errors.Is(err, sql.ErrNoRows) {
				return metering.ErrNotFound
			} else if err != nil {
				return fmt.Errorf("load credit project: %w", err)
			}
			if projectTenant != params.TenantID {
				return metering.ErrNotFound
			}
			_, err := transaction.ExecContext(ctx, `INSERT INTO ledger_entries (id, tenant_id, project_id, usage_fact_id, entry_type, amount_minor, currency, reference_id, description, created_at) VALUES ($1,$2,$3,NULL,'credit',$4,$5,$6,$7,$8)`, entryID, params.TenantID, params.ProjectID, params.AmountMinor, currency, params.ReferenceID, description, now)
			if err != nil && strings.Contains(err.Error(), "duplicate key") {
				return metering.ErrConflict
			}
			return mapWorkspaceWriteError(err)
		},
	})
}
