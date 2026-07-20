package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/identity"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/operation"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/sharedisolation"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

var (
	slugPattern          = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)
	resourceClassPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
)

type acceptedMutationSpec struct {
	kind                string
	resourceType        string
	resourceID          string
	eventType           string
	scopeType           string
	scopeID             string
	eventFields         map[string]any
	completeImmediately bool
	apply               func(context.Context, *sql.Tx, time.Time) error
}

func (repository *Repository) CreateTenant(
	ctx context.Context,
	params tenancy.CreateTenantParams,
) (tenancy.Acceptance, error) {
	name := strings.TrimSpace(params.Name)
	slug := strings.TrimSpace(params.Slug)
	if name == "" || len([]rune(name)) > 120 {
		return tenancy.Acceptance{}, invalidTenancyRequest("tenant name must contain between 1 and 120 characters")
	}
	if !slugPattern.MatchString(slug) {
		return tenancy.Acceptance{}, invalidTenancyRequest("tenant slug must be a lowercase DNS-style label between 3 and 63 characters")
	}
	tenantID, err := identity.NewUUID()
	if err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("generate tenant ID: %w", err)
	}
	return repository.acceptMutation(ctx, params.Mutation, acceptedMutationSpec{
		kind:         "tenant.create",
		resourceType: "tenant",
		resourceID:   tenantID,
		eventType:    "tenant.created",
		scopeType:    "tenant",
		scopeID:      tenantID,
		eventFields:  map[string]any{"name": name, "slug": slug},
		apply: func(ctx context.Context, transaction *sql.Tx, now time.Time) error {
			_, err := transaction.ExecContext(ctx, `
INSERT INTO tenants (id, name, slug, status, generation, created_at, updated_at)
VALUES ($1, $2, $3, 'active', 1, $4, $4)`, tenantID, name, slug, now)
			return mapTenancyWriteError(err, "tenant")
		},
	})
}

func (repository *Repository) GetTenant(ctx context.Context, tenantID string) (tenancy.Tenant, error) {
	if !identity.IsUUID(tenantID) {
		return tenancy.Tenant{}, tenancy.ErrNotFound
	}
	row := repository.database.QueryRowContext(ctx, `
SELECT id::text, name, slug, status, generation, created_at, updated_at
FROM tenants
WHERE id = $1`, tenantID)
	var result tenancy.Tenant
	if err := row.Scan(
		&result.ID,
		&result.Name,
		&result.Slug,
		&result.Status,
		&result.Generation,
		&result.CreatedAt,
		&result.UpdatedAt,
	); errors.Is(err, sql.ErrNoRows) {
		return tenancy.Tenant{}, tenancy.ErrNotFound
	} else if err != nil {
		return tenancy.Tenant{}, fmt.Errorf("get tenant: %w", err)
	}
	return result, nil
}

func (repository *Repository) CreateProject(
	ctx context.Context,
	params tenancy.CreateProjectParams,
) (tenancy.Acceptance, error) {
	if !identity.IsUUID(params.TenantID) {
		return tenancy.Acceptance{}, tenancy.ErrNotFound
	}
	name := strings.TrimSpace(params.Name)
	slug := strings.TrimSpace(params.Slug)
	if name == "" || len([]rune(name)) > 120 {
		return tenancy.Acceptance{}, invalidTenancyRequest("project name must contain between 1 and 120 characters")
	}
	if !slugPattern.MatchString(slug) {
		return tenancy.Acceptance{}, invalidTenancyRequest("project slug must be a lowercase DNS-style label between 3 and 63 characters")
	}
	if params.IsolationClass != tenancy.IsolationShared {
		return tenancy.Acceptance{}, invalidTenancyRequest("Phase 1 project creation currently supports the shared isolation class")
	}
	projectID, err := identity.NewUUID()
	if err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("generate project ID: %w", err)
	}
	namespaceName := projectNamespace(projectID)
	return repository.acceptMutation(ctx, params.Mutation, acceptedMutationSpec{
		kind:         "project.create",
		resourceType: "project",
		resourceID:   projectID,
		eventType:    "project.created",
		scopeType:    "tenant",
		scopeID:      params.TenantID,
		eventFields: map[string]any{
			"tenantId":       params.TenantID,
			"name":           name,
			"slug":           slug,
			"isolationClass": params.IsolationClass,
			"namespaceName":  namespaceName,
		},
		apply: func(ctx context.Context, transaction *sql.Tx, now time.Time) error {
			if exists, err := rowExists(ctx, transaction, "SELECT 1 FROM tenants WHERE id = $1", params.TenantID); err != nil {
				return fmt.Errorf("check tenant: %w", err)
			} else if !exists {
				return tenancy.ErrNotFound
			}
			_, err := transaction.ExecContext(ctx, `
INSERT INTO projects (
  id, tenant_id, name, slug, isolation_class, namespace_name,
  desired_state, observed_state, provisioning_state, conditions,
  generation, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, 'active', 'pending', 'pending', '[]'::jsonb, 1, $7, $7)`,
				projectID,
				params.TenantID,
				name,
				slug,
				params.IsolationClass,
				namespaceName,
				now,
			)
			return mapTenancyWriteError(err, "project")
		},
	})
}

func (repository *Repository) GetProject(ctx context.Context, projectID string) (tenancy.Project, error) {
	if !identity.IsUUID(projectID) {
		return tenancy.Project{}, tenancy.ErrNotFound
	}
	row := repository.database.QueryRowContext(ctx, `
SELECT
  id::text, tenant_id::text, name, slug, isolation_class, namespace_name,
  target_cluster_id, desired_state, observed_state, provisioning_state, conditions,
  generation, observed_generation, last_reconciled_at, manifest_work_name,
  applied_gpu_quota, created_at, updated_at
FROM projects
WHERE id = $1`, projectID)
	return scanProject(row)
}

func (repository *Repository) CreateRoleBinding(
	ctx context.Context,
	params tenancy.CreateRoleBindingParams,
) (tenancy.Acceptance, error) {
	if !identity.IsUUID(params.ScopeID) {
		return tenancy.Acceptance{}, tenancy.ErrNotFound
	}
	if !validScopeType(params.ScopeType) {
		return tenancy.Acceptance{}, invalidTenancyRequest("role binding scope type must be tenant or project")
	}
	if !validSubjectType(params.SubjectType) {
		return tenancy.Acceptance{}, invalidTenancyRequest("role binding subject type is invalid")
	}
	subjectID := strings.TrimSpace(params.SubjectID)
	if subjectID == "" || len(subjectID) > 255 {
		return tenancy.Acceptance{}, invalidTenancyRequest("role binding subject ID must contain between 1 and 255 characters")
	}
	if !roleAllowedForScope(params.Role, params.ScopeType, params.SubjectType) {
		return tenancy.Acceptance{}, invalidTenancyRequest("role is not valid for the supplied scope and subject")
	}
	bindingID, err := identity.NewUUID()
	if err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("generate role binding ID: %w", err)
	}
	return repository.acceptMutation(ctx, params.Mutation, acceptedMutationSpec{
		kind:         "role_binding.create",
		resourceType: "role_binding",
		resourceID:   bindingID,
		eventType:    "role_binding.created",
		scopeType:    string(params.ScopeType),
		scopeID:      params.ScopeID,
		eventFields: map[string]any{
			"scopeType":   params.ScopeType,
			"scopeId":     params.ScopeID,
			"subjectType": params.SubjectType,
			"subjectId":   subjectID,
			"role":        params.Role,
		},
		apply: func(ctx context.Context, transaction *sql.Tx, now time.Time) error {
			if exists, err := tenancyScopeExists(ctx, transaction, params.ScopeType, params.ScopeID); err != nil {
				return err
			} else if !exists {
				return tenancy.ErrNotFound
			}
			_, err := transaction.ExecContext(ctx, `
INSERT INTO role_bindings (
  id, scope_type, scope_id, subject_type, subject_id, role, created_by, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
				bindingID,
				params.ScopeType,
				params.ScopeID,
				params.SubjectType,
				subjectID,
				params.Role,
				params.Mutation.PrincipalID,
				now,
			)
			return mapTenancyWriteError(err, "role binding")
		},
	})
}

func (repository *Repository) GetRoleBinding(ctx context.Context, bindingID string) (tenancy.RoleBinding, error) {
	if !identity.IsUUID(bindingID) {
		return tenancy.RoleBinding{}, tenancy.ErrNotFound
	}
	row := repository.database.QueryRowContext(ctx, `
SELECT id::text, scope_type, scope_id::text, subject_type, subject_id, role, created_by, created_at
FROM role_bindings
WHERE id = $1`, bindingID)
	var result tenancy.RoleBinding
	if err := row.Scan(
		&result.ID,
		&result.ScopeType,
		&result.ScopeID,
		&result.SubjectType,
		&result.SubjectID,
		&result.Role,
		&result.CreatedBy,
		&result.CreatedAt,
	); errors.Is(err, sql.ErrNoRows) {
		return tenancy.RoleBinding{}, tenancy.ErrNotFound
	} else if err != nil {
		return tenancy.RoleBinding{}, fmt.Errorf("get role binding: %w", err)
	}
	return result, nil
}

func (repository *Repository) SetQuota(
	ctx context.Context,
	params tenancy.SetQuotaParams,
) (tenancy.Acceptance, error) {
	if !identity.IsUUID(params.ProjectID) {
		return tenancy.Acceptance{}, tenancy.ErrNotFound
	}
	resourceClass := strings.TrimSpace(params.ResourceClass)
	if !resourceClassPattern.MatchString(resourceClass) {
		return tenancy.Acceptance{}, invalidTenancyRequest("quota resource class is invalid")
	}
	if params.HardLimit < 0 {
		return tenancy.Acceptance{}, invalidTenancyRequest("quota hard limit must be non-negative")
	}
	resourceID := params.ProjectID + "/" + resourceClass
	eventType := "quota.updated"
	if resourceClass == sharedisolation.GPUQuotaResourceClass {
		eventType = "project.gpu-quota.updated"
	}
	return repository.acceptMutation(ctx, params.Mutation, acceptedMutationSpec{
		kind:         "quota.set",
		resourceType: "quota",
		resourceID:   resourceID,
		eventType:    eventType,
		scopeType:    "project",
		scopeID:      params.ProjectID,
		eventFields: map[string]any{
			"projectId":     params.ProjectID,
			"resourceClass": resourceClass,
			"hardLimit":     params.HardLimit,
		},
		apply: func(ctx context.Context, transaction *sql.Tx, now time.Time) error {
			if exists, err := rowExists(ctx, transaction, "SELECT 1 FROM projects WHERE id = $1", params.ProjectID); err != nil {
				return fmt.Errorf("check project: %w", err)
			} else if !exists {
				return tenancy.ErrNotFound
			}
			row := transaction.QueryRowContext(ctx, `
INSERT INTO project_quotas (
  project_id, resource_class, hard_limit, reserved, allocated, generation, updated_at
) VALUES ($1, $2, $3, 0, 0, 1, $4)
ON CONFLICT (project_id, resource_class) DO UPDATE
SET
  hard_limit = EXCLUDED.hard_limit,
  generation = project_quotas.generation + 1,
  updated_at = EXCLUDED.updated_at
WHERE project_quotas.reserved + project_quotas.allocated <= EXCLUDED.hard_limit
RETURNING project_id`, params.ProjectID, resourceClass, params.HardLimit, now)
			var projectID string
			if err := row.Scan(&projectID); errors.Is(err, sql.ErrNoRows) {
				return tenancy.ErrQuotaBelowUsage
			} else if err != nil {
				return mapTenancyWriteError(err, "quota")
			}
			return nil
		},
	})
}

func (repository *Repository) GetQuota(
	ctx context.Context,
	projectID string,
	resourceClass string,
) (tenancy.Quota, error) {
	if !identity.IsUUID(projectID) || !resourceClassPattern.MatchString(resourceClass) {
		return tenancy.Quota{}, tenancy.ErrNotFound
	}
	row := repository.database.QueryRowContext(ctx, `
SELECT project_id::text, resource_class, hard_limit, reserved, allocated, generation, updated_at
FROM project_quotas
WHERE project_id = $1 AND resource_class = $2`, projectID, resourceClass)
	return scanQuota(row)
}

func (repository *Repository) ReserveQuota(
	ctx context.Context,
	params tenancy.ReserveQuotaParams,
) (tenancy.QuotaReservation, error) {
	if !identity.IsUUID(params.ProjectID) || !identity.IsUUID(params.OperationID) {
		return tenancy.QuotaReservation{}, tenancy.ErrNotFound
	}
	if !resourceClassPattern.MatchString(params.ResourceClass) {
		return tenancy.QuotaReservation{}, invalidTenancyRequest("quota resource class is invalid")
	}
	if params.Amount <= 0 {
		return tenancy.QuotaReservation{}, invalidTenancyRequest("quota reservation amount must be greater than zero")
	}
	now := repository.now().UTC()
	if !params.ExpiresAt.After(now) {
		return tenancy.QuotaReservation{}, invalidTenancyRequest("quota reservation expiry must be in the future")
	}
	transaction, err := repository.database.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return tenancy.QuotaReservation{}, fmt.Errorf("begin quota reservation transaction: %w", err)
	}
	defer transaction.Rollback()

	var hardLimit, reserved, allocated int64
	if err := transaction.QueryRowContext(ctx, `
SELECT hard_limit, reserved, allocated
FROM project_quotas
WHERE project_id = $1 AND resource_class = $2
FOR UPDATE`, params.ProjectID, params.ResourceClass).Scan(&hardLimit, &reserved, &allocated); errors.Is(err, sql.ErrNoRows) {
		return tenancy.QuotaReservation{}, tenancy.ErrNotFound
	} else if err != nil {
		return tenancy.QuotaReservation{}, fmt.Errorf("lock project quota: %w", err)
	}
	if params.Amount > hardLimit-reserved-allocated {
		return tenancy.QuotaReservation{}, tenancy.ErrQuotaExceeded
	}
	reservationID, err := identity.NewUUID()
	if err != nil {
		return tenancy.QuotaReservation{}, fmt.Errorf("generate quota reservation ID: %w", err)
	}
	result := tenancy.QuotaReservation{
		ID:            reservationID,
		ProjectID:     params.ProjectID,
		ResourceClass: params.ResourceClass,
		Amount:        params.Amount,
		Status:        tenancy.ReservationPending,
		OperationID:   params.OperationID,
		ExpiresAt:     params.ExpiresAt.UTC(),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if _, err := transaction.ExecContext(ctx, `
UPDATE project_quotas
SET reserved = reserved + $3, generation = generation + 1, updated_at = $4
WHERE project_id = $1 AND resource_class = $2`, params.ProjectID, params.ResourceClass, params.Amount, now); err != nil {
		return tenancy.QuotaReservation{}, fmt.Errorf("reserve project quota: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `
INSERT INTO quota_reservations (
  id, project_id, resource_class, amount, status, operation_id,
  expires_at, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)`,
		result.ID,
		result.ProjectID,
		result.ResourceClass,
		result.Amount,
		result.Status,
		result.OperationID,
		result.ExpiresAt,
		result.CreatedAt,
	); err != nil {
		return tenancy.QuotaReservation{}, mapTenancyWriteError(err, "quota reservation")
	}
	if err := repository.insertDomainEventInTx(ctx, transaction, "quota_reservation", result.ID, "quota_reservation.created", map[string]any{
		"reservationId": result.ID,
		"projectId":     result.ProjectID,
		"resourceClass": result.ResourceClass,
		"amount":        result.Amount,
		"operationId":   result.OperationID,
	}, now); err != nil {
		return tenancy.QuotaReservation{}, err
	}
	if err := transaction.Commit(); err != nil {
		return tenancy.QuotaReservation{}, fmt.Errorf("commit quota reservation: %w", err)
	}
	return result, nil
}

func (repository *Repository) CommitQuotaReservation(
	ctx context.Context,
	reservationID string,
) (tenancy.QuotaReservation, error) {
	return repository.transitionQuotaReservation(ctx, reservationID, tenancy.ReservationCommitted)
}

func (repository *Repository) ReleaseQuotaReservation(
	ctx context.Context,
	reservationID string,
	status tenancy.ReservationStatus,
) (tenancy.QuotaReservation, error) {
	if status != tenancy.ReservationReleased && status != tenancy.ReservationExpired {
		return tenancy.QuotaReservation{}, tenancy.ErrInvalidTransition
	}
	return repository.transitionQuotaReservation(ctx, reservationID, status)
}

func (repository *Repository) acceptMutation(
	ctx context.Context,
	mutation tenancy.MutationContext,
	spec acceptedMutationSpec,
) (tenancy.Acceptance, error) {
	if err := validateMutationContext(mutation); err != nil {
		return tenancy.Acceptance{}, err
	}
	transaction, err := repository.database.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("begin %s transaction: %w", spec.kind, err)
	}
	defer transaction.Rollback()

	idempotencyScope := spec.kind + ":" + mutation.PrincipalID
	if _, err := transaction.ExecContext(
		ctx,
		"SELECT pg_advisory_xact_lock(hashtextextended($1, 0))",
		idempotencyScope+":"+mutation.IdempotencyKey,
	); err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("lock idempotency key: %w", err)
	}
	if replayed, found, err := loadIdempotencyAcceptance(
		ctx,
		transaction,
		idempotencyScope,
		mutation.IdempotencyKey,
		mutation.RequestHash,
	); err != nil {
		return tenancy.Acceptance{}, err
	} else if found {
		if err := transaction.Commit(); err != nil {
			return tenancy.Acceptance{}, fmt.Errorf("commit idempotency replay: %w", err)
		}
		replayed.Replayed = true
		return replayed, nil
	}

	now := repository.now().UTC()
	if err := spec.apply(ctx, transaction, now); err != nil {
		return tenancy.Acceptance{}, err
	}
	createdOperation, err := repository.CreateInTx(ctx, transaction, operation.CreateParams{
		Kind:      spec.kind,
		Target:    operation.ResourceRef{Type: spec.resourceType, ID: spec.resourceID},
		Retryable: true,
		RequestID: mutation.RequestID,
	})
	if err != nil {
		return tenancy.Acceptance{}, err
	}
	if spec.completeImmediately {
		if _, err := transaction.ExecContext(ctx, `
UPDATE operations
SET status = 'succeeded', progress = 100, retryable = false,
    started_at = $2, finished_at = $2, updated_at = $2
WHERE id = $1`, createdOperation.ID, now); err != nil {
			return tenancy.Acceptance{}, fmt.Errorf("complete immediate operation: %w", err)
		}
		if _, err := transaction.ExecContext(ctx, `
UPDATE outbox_events
SET delivered_at = $2
WHERE aggregate_type = 'operation' AND aggregate_id = $1 AND event_type = 'operation.queued'`, createdOperation.ID, now); err != nil {
			return tenancy.Acceptance{}, fmt.Errorf("complete immediate operation event: %w", err)
		}
	}
	acceptance := tenancy.Acceptance{ResourceID: spec.resourceID, OperationID: createdOperation.ID}
	eventFields := map[string]any{
		"resourceId":  spec.resourceID,
		"operationId": createdOperation.ID,
		"requestId":   mutation.RequestID,
	}
	for key, value := range spec.eventFields {
		eventFields[key] = value
	}
	if err := repository.insertDomainEventInTx(
		ctx,
		transaction,
		spec.resourceType,
		spec.resourceID,
		spec.eventType,
		eventFields,
		now,
	); err != nil {
		return tenancy.Acceptance{}, err
	}
	if err := repository.insertAuditEventInTx(
		ctx,
		transaction,
		mutation,
		spec.kind,
		spec.resourceType,
		spec.resourceID,
		spec.scopeType,
		spec.scopeID,
		now,
	); err != nil {
		return tenancy.Acceptance{}, err
	}
	if err := recordIdempotencyAcceptance(
		ctx,
		transaction,
		idempotencyScope,
		mutation,
		spec.resourceType,
		acceptance,
		now,
	); err != nil {
		return tenancy.Acceptance{}, err
	}
	if err := transaction.Commit(); err != nil {
		return tenancy.Acceptance{}, fmt.Errorf("commit %s transaction: %w", spec.kind, err)
	}
	return acceptance, nil
}

func (repository *Repository) insertDomainEventInTx(
	ctx context.Context,
	transaction *sql.Tx,
	aggregateType string,
	aggregateID string,
	eventType string,
	payload any,
	now time.Time,
) error {
	eventID, err := identity.NewUUID()
	if err != nil {
		return fmt.Errorf("generate domain event ID: %w", err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode domain event: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `
INSERT INTO outbox_events (
  id, aggregate_type, aggregate_id, event_type, payload, occurred_at, available_at
) VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		eventID,
		aggregateType,
		aggregateID,
		eventType,
		encoded,
		now,
	); err != nil {
		return fmt.Errorf("insert domain outbox event: %w", err)
	}
	return nil
}

func (repository *Repository) insertAuditEventInTx(
	ctx context.Context,
	transaction *sql.Tx,
	mutation tenancy.MutationContext,
	action string,
	resourceType string,
	resourceID string,
	scopeType string,
	scopeID string,
	now time.Time,
) error {
	auditID, err := identity.NewUUID()
	if err != nil {
		return fmt.Errorf("generate audit event ID: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `
INSERT INTO audit_events (
  id, occurred_at, actor_type, actor_id, scope_type, scope_id,
  action, resource_type, resource_id, request_id, outcome, details
) VALUES ($1, $2, 'principal', $3, $4, $5, $6, $7, $8, $9, 'accepted', '{}'::jsonb)`,
		auditID,
		now,
		mutation.PrincipalID,
		scopeType,
		scopeID,
		action,
		resourceType,
		resourceID,
		mutation.RequestID,
	); err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

func (repository *Repository) transitionQuotaReservation(
	ctx context.Context,
	reservationID string,
	target tenancy.ReservationStatus,
) (tenancy.QuotaReservation, error) {
	if !identity.IsUUID(reservationID) {
		return tenancy.QuotaReservation{}, tenancy.ErrNotFound
	}
	transaction, err := repository.database.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return tenancy.QuotaReservation{}, fmt.Errorf("begin quota transition transaction: %w", err)
	}
	defer transaction.Rollback()

	result, err := scanQuotaReservation(transaction.QueryRowContext(ctx, `
SELECT
  id::text, project_id::text, resource_class, amount, status,
  operation_id::text, expires_at, created_at, updated_at
FROM quota_reservations
WHERE id = $1
FOR UPDATE`, reservationID))
	if errors.Is(err, tenancy.ErrNotFound) {
		return tenancy.QuotaReservation{}, err
	}
	if err != nil {
		return tenancy.QuotaReservation{}, fmt.Errorf("lock quota reservation: %w", err)
	}
	if result.Status == target {
		if err := transaction.Commit(); err != nil {
			return tenancy.QuotaReservation{}, fmt.Errorf("commit quota transition replay: %w", err)
		}
		return result, nil
	}
	if result.Status != tenancy.ReservationPending && !(result.Status == tenancy.ReservationCommitted && target == tenancy.ReservationReleased) {
		return tenancy.QuotaReservation{}, tenancy.ErrInvalidTransition
	}

	now := repository.now().UTC()
	var quotaUpdate string
	if result.Status == tenancy.ReservationPending && target == tenancy.ReservationCommitted {
		quotaUpdate = `
UPDATE project_quotas
SET reserved = reserved - $3, allocated = allocated + $3, generation = generation + 1, updated_at = $4
WHERE project_id = $1 AND resource_class = $2`
	} else if result.Status == tenancy.ReservationPending {
		quotaUpdate = `
UPDATE project_quotas
SET reserved = reserved - $3, generation = generation + 1, updated_at = $4
WHERE project_id = $1 AND resource_class = $2`
	} else {
		quotaUpdate = `
UPDATE project_quotas
SET allocated = allocated - $3, generation = generation + 1, updated_at = $4
WHERE project_id = $1 AND resource_class = $2`
	}
	if _, err := transaction.ExecContext(
		ctx,
		quotaUpdate,
		result.ProjectID,
		result.ResourceClass,
		result.Amount,
		now,
	); err != nil {
		return tenancy.QuotaReservation{}, fmt.Errorf("transition project quota: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `
UPDATE quota_reservations
SET status = $2, updated_at = $3
WHERE id = $1`, result.ID, target, now); err != nil {
		return tenancy.QuotaReservation{}, fmt.Errorf("transition quota reservation: %w", err)
	}
	result.Status = target
	result.UpdatedAt = now
	if err := repository.insertDomainEventInTx(
		ctx,
		transaction,
		"quota_reservation",
		result.ID,
		"quota_reservation."+string(target),
		map[string]any{
			"reservationId": result.ID,
			"projectId":     result.ProjectID,
			"resourceClass": result.ResourceClass,
			"amount":        result.Amount,
			"operationId":   result.OperationID,
		},
		now,
	); err != nil {
		return tenancy.QuotaReservation{}, err
	}
	if err := transaction.Commit(); err != nil {
		return tenancy.QuotaReservation{}, fmt.Errorf("commit quota transition: %w", err)
	}
	return result, nil
}

func loadIdempotencyAcceptance(
	ctx context.Context,
	transaction *sql.Tx,
	scope string,
	key string,
	requestHash string,
) (tenancy.Acceptance, bool, error) {
	var recordedHash string
	var resourceID sql.NullString
	var operationID sql.NullString
	err := transaction.QueryRowContext(ctx, `
SELECT request_hash, resource_id, operation_id::text
FROM idempotency_records
WHERE scope = $1 AND idempotency_key = $2`, scope, key).Scan(&recordedHash, &resourceID, &operationID)
	if errors.Is(err, sql.ErrNoRows) {
		return tenancy.Acceptance{}, false, nil
	}
	if err != nil {
		return tenancy.Acceptance{}, false, fmt.Errorf("load idempotency record: %w", err)
	}
	if recordedHash != requestHash {
		return tenancy.Acceptance{}, false, tenancy.ErrIdempotencyConflict
	}
	if !resourceID.Valid || !operationID.Valid {
		return tenancy.Acceptance{}, false, errors.New("idempotency record is incomplete")
	}
	return tenancy.Acceptance{ResourceID: resourceID.String, OperationID: operationID.String}, true, nil
}

func recordIdempotencyAcceptance(
	ctx context.Context,
	transaction *sql.Tx,
	scope string,
	mutation tenancy.MutationContext,
	resourceType string,
	acceptance tenancy.Acceptance,
	now time.Time,
) error {
	responseBody, err := json.Marshal(acceptance)
	if err != nil {
		return fmt.Errorf("encode idempotency response: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `
INSERT INTO idempotency_records (
  scope, idempotency_key, request_hash, response_status, response_headers,
  response_body, resource_type, resource_id, operation_id, expires_at, created_at
) VALUES ($1, $2, $3, 202, '{}'::jsonb, $4, $5, $6, $7, $8, $9)`,
		scope,
		mutation.IdempotencyKey,
		mutation.RequestHash,
		responseBody,
		resourceType,
		acceptance.ResourceID,
		acceptance.OperationID,
		now.Add(24*time.Hour),
		now,
	); err != nil {
		return fmt.Errorf("insert idempotency record: %w", err)
	}
	return nil
}

func invalidTenancyRequest(message string) error {
	return fmt.Errorf("%s: %w", message, tenancy.ErrInvalid)
}

func validateMutationContext(mutation tenancy.MutationContext) error {
	if strings.TrimSpace(mutation.PrincipalID) == "" || len(mutation.PrincipalID) > 255 {
		return invalidTenancyRequest("mutation principal ID is required")
	}
	if strings.TrimSpace(mutation.RequestID) == "" || len(mutation.RequestID) > 128 {
		return invalidTenancyRequest("mutation request ID is required")
	}
	if len(strings.TrimSpace(mutation.IdempotencyKey)) < 8 || len(mutation.IdempotencyKey) > 255 {
		return invalidTenancyRequest("Idempotency-Key must contain between 8 and 255 characters")
	}
	if strings.TrimSpace(mutation.RequestHash) == "" || len(mutation.RequestHash) > 128 {
		return invalidTenancyRequest("mutation request hash is required")
	}
	return nil
}

func projectNamespace(projectID string) string {
	return "gpu-p-" + strings.ReplaceAll(projectID, "-", "")[:12]
}

func validScopeType(scope tenancy.ScopeType) bool {
	return scope == tenancy.ScopeTenant || scope == tenancy.ScopeProject
}

func validSubjectType(subject tenancy.SubjectType) bool {
	return subject == tenancy.SubjectUser || subject == tenancy.SubjectGroup || subject == tenancy.SubjectServiceAccount
}

func roleAllowedForScope(role tenancy.Role, scope tenancy.ScopeType, subject tenancy.SubjectType) bool {
	if role == tenancy.RoleServiceAccount {
		return scope == tenancy.ScopeProject && subject == tenancy.SubjectServiceAccount
	}
	if scope == tenancy.ScopeTenant {
		return role == tenancy.RoleTenantOwner || role == tenancy.RoleBillingAdmin || role == tenancy.RoleAuditor || role == tenancy.RoleViewer
	}
	return role == tenancy.RoleProjectAdmin || role == tenancy.RoleOperator || role == tenancy.RoleDeveloper || role == tenancy.RoleViewer || role == tenancy.RoleBillingAdmin || role == tenancy.RoleAuditor
}

func tenancyScopeExists(
	ctx context.Context,
	transaction *sql.Tx,
	scope tenancy.ScopeType,
	scopeID string,
) (bool, error) {
	switch scope {
	case tenancy.ScopeTenant:
		return rowExists(ctx, transaction, "SELECT 1 FROM tenants WHERE id = $1", scopeID)
	case tenancy.ScopeProject:
		return rowExists(ctx, transaction, "SELECT 1 FROM projects WHERE id = $1", scopeID)
	default:
		return false, errors.New("unsupported role binding scope")
	}
}

func rowExists(ctx context.Context, transaction *sql.Tx, query string, argument any) (bool, error) {
	var marker int
	err := transaction.QueryRowContext(ctx, query, argument).Scan(&marker)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func mapTenancyWriteError(err error, resource string) error {
	if err == nil {
		return nil
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%s already exists: %w", resource, tenancy.ErrConflict)
		case "23503":
			return tenancy.ErrNotFound
		}
	}
	return err
}

func scanProject(row scanner) (tenancy.Project, error) {
	var result tenancy.Project
	var conditions []byte
	if err := row.Scan(
		&result.ID,
		&result.TenantID,
		&result.Name,
		&result.Slug,
		&result.IsolationClass,
		&result.NamespaceName,
		&result.TargetClusterID,
		&result.DesiredState,
		&result.ObservedState,
		&result.ProvisioningState,
		&conditions,
		&result.Generation,
		&result.ObservedGeneration,
		&result.LastReconciledAt,
		&result.ManifestWorkName,
		&result.AppliedGPUQuota,
		&result.CreatedAt,
		&result.UpdatedAt,
	); errors.Is(err, sql.ErrNoRows) {
		return tenancy.Project{}, tenancy.ErrNotFound
	} else if err != nil {
		return tenancy.Project{}, fmt.Errorf("scan project: %w", err)
	}
	if err := json.Unmarshal(conditions, &result.Conditions); err != nil {
		return tenancy.Project{}, fmt.Errorf("decode project conditions: %w", err)
	}
	return result, nil
}

func scanQuota(row scanner) (tenancy.Quota, error) {
	var result tenancy.Quota
	if err := row.Scan(
		&result.ProjectID,
		&result.ResourceClass,
		&result.HardLimit,
		&result.Reserved,
		&result.Allocated,
		&result.Generation,
		&result.UpdatedAt,
	); errors.Is(err, sql.ErrNoRows) {
		return tenancy.Quota{}, tenancy.ErrNotFound
	} else if err != nil {
		return tenancy.Quota{}, fmt.Errorf("scan quota: %w", err)
	}
	return result, nil
}

func scanQuotaReservation(row scanner) (tenancy.QuotaReservation, error) {
	var result tenancy.QuotaReservation
	if err := row.Scan(
		&result.ID,
		&result.ProjectID,
		&result.ResourceClass,
		&result.Amount,
		&result.Status,
		&result.OperationID,
		&result.ExpiresAt,
		&result.CreatedAt,
		&result.UpdatedAt,
	); errors.Is(err, sql.ErrNoRows) {
		return tenancy.QuotaReservation{}, tenancy.ErrNotFound
	} else if err != nil {
		return tenancy.QuotaReservation{}, fmt.Errorf("scan quota reservation: %w", err)
	}
	return result, nil
}

var _ tenancy.Repository = (*Repository)(nil)
