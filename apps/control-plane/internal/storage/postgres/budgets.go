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

func (repository *Repository) SetBudget(ctx context.Context, params metering.SetBudgetParams) (tenancy.Acceptance, error) {
	if !identity.IsUUID(params.TenantID) || !identity.IsUUID(params.ProjectID) || params.LimitMinor < 0 || len(strings.TrimSpace(params.Currency)) != 3 {
		return tenancy.Acceptance{}, metering.ErrInvalid
	}
	currency := strings.ToUpper(strings.TrimSpace(params.Currency))
	budgetID := params.ProjectID
	return repository.acceptMutation(ctx, params.Mutation, acceptedMutationSpec{
		kind: "project.budget.set", resourceType: "project-budget", resourceID: budgetID, eventType: "project.budget.updated",
		scopeType: string(tenancy.ScopeProject), scopeID: params.ProjectID,
		eventFields: map[string]any{"tenantId": params.TenantID, "projectId": params.ProjectID, "currency": currency, "limitMinor": params.LimitMinor}, completeImmediately: true,
		apply: func(ctx context.Context, transaction *sql.Tx, now time.Time) error {
			var projectTenant string
			if err := transaction.QueryRowContext(ctx, `SELECT tenant_id::text FROM projects WHERE id = $1`, params.ProjectID).Scan(&projectTenant); errors.Is(err, sql.ErrNoRows) {
				return metering.ErrNotFound
			} else if err != nil {
				return fmt.Errorf("load budget project: %w", err)
			}
			if projectTenant != params.TenantID {
				return metering.ErrNotFound
			}
			var usedMinor int64
			if err := transaction.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE WHEN entry_type = 'debit' THEN amount_minor ELSE -amount_minor END), 0) FROM ledger_entries WHERE project_id = $1 AND currency = $2`, params.ProjectID, currency).Scan(&usedMinor); err != nil {
				return fmt.Errorf("calculate budget usage: %w", err)
			}
			if usedMinor > params.LimitMinor {
				return metering.ErrBudgetExceeded
			}
			_, err := transaction.ExecContext(ctx, `INSERT INTO project_budgets (project_id, tenant_id, currency, limit_minor, generation, created_at, updated_at) VALUES ($1,$2,$3,$4,1,$5,$5) ON CONFLICT (project_id) DO UPDATE SET currency = EXCLUDED.currency, limit_minor = EXCLUDED.limit_minor, generation = project_budgets.generation + 1, updated_at = EXCLUDED.updated_at`, params.ProjectID, params.TenantID, currency, params.LimitMinor, now)
			return mapWorkspaceWriteError(err)
		},
	})
}

func (repository *Repository) GetBudget(ctx context.Context, projectID string) (metering.Budget, error) {
	if !identity.IsUUID(projectID) {
		return metering.Budget{}, metering.ErrNotFound
	}
	var result metering.Budget
	if err := repository.database.QueryRowContext(ctx, `SELECT tenant_id::text, project_id::text, currency, limit_minor, generation, updated_at FROM project_budgets WHERE project_id = $1`, projectID).Scan(&result.TenantID, &result.ProjectID, &result.Currency, &result.LimitMinor, &result.Generation, &result.UpdatedAt); errors.Is(err, sql.ErrNoRows) {
		return metering.Budget{}, metering.ErrNotFound
	} else if err != nil {
		return metering.Budget{}, fmt.Errorf("get project budget: %w", err)
	}
	if err := repository.database.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE WHEN entry_type = 'debit' THEN amount_minor ELSE -amount_minor END), 0) FROM ledger_entries WHERE project_id = $1 AND currency = $2`, projectID, result.Currency).Scan(&result.UsedMinor); err != nil {
		return metering.Budget{}, fmt.Errorf("calculate project budget usage: %w", err)
	}
	result.AvailableMinor = result.LimitMinor - result.UsedMinor
	return result, nil
}
