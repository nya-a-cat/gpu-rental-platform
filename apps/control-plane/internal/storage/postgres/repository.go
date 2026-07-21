package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/identity"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/operation"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/outbox"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/ports"
)

const maxOutboxErrorRunes = 4096

type Repository struct {
	database *sql.DB
	now      func() time.Time
	billing  ports.BillingEngine
}

func NewRepository(database *sql.DB, billingEngines ...ports.BillingEngine) *Repository {
	var billing ports.BillingEngine
	if len(billingEngines) > 0 {
		billing = billingEngines[0]
	}
	return &Repository{database: database, now: time.Now, billing: billing}
}

// Create is a convenience transaction wrapper for callers that only create an
// operation. Business mutations that also write domain state or idempotency
// records should own the transaction and call CreateInTx.
func (repository *Repository) Create(ctx context.Context, params operation.CreateParams) (operation.Operation, error) {
	transaction, err := repository.database.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return operation.Operation{}, fmt.Errorf("begin operation transaction: %w", err)
	}
	defer transaction.Rollback()

	created, err := repository.CreateInTx(ctx, transaction, params)
	if err != nil {
		return operation.Operation{}, err
	}
	if err := transaction.Commit(); err != nil {
		return operation.Operation{}, fmt.Errorf("commit operation transaction: %w", err)
	}
	return created, nil
}

func (repository *Repository) CreateInTx(
	ctx context.Context,
	transaction *sql.Tx,
	params operation.CreateParams,
) (operation.Operation, error) {
	if transaction == nil {
		return operation.Operation{}, errors.New("operation transaction is required")
	}
	if strings.TrimSpace(params.Kind) == "" {
		return operation.Operation{}, errors.New("operation kind is required")
	}
	if strings.TrimSpace(params.Target.Type) == "" || strings.TrimSpace(params.Target.ID) == "" {
		return operation.Operation{}, errors.New("operation target is required")
	}
	if strings.TrimSpace(params.RequestID) == "" {
		return operation.Operation{}, errors.New("operation request ID is required")
	}

	operationID := params.ID
	if operationID == "" {
		var err error
		operationID, err = identity.NewUUID()
		if err != nil {
			return operation.Operation{}, fmt.Errorf("generate operation ID: %w", err)
		}
	}
	if !identity.IsUUID(operationID) {
		return operation.Operation{}, errors.New("operation ID must be a UUID")
	}

	eventID, err := identity.NewUUID()
	if err != nil {
		return operation.Operation{}, fmt.Errorf("generate outbox event ID: %w", err)
	}
	now := repository.now().UTC()
	created := operation.Operation{
		ID:                operationID,
		Kind:              params.Kind,
		Status:            operation.StatusQueued,
		Target:            params.Target,
		ParentOperationID: params.ParentOperationID,
		Steps:             []operation.Step{},
		Progress:          0,
		Deadline:          params.Deadline,
		Retryable:         params.Retryable,
		RequestID:         params.RequestID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	payload, err := json.Marshal(map[string]any{
		"operationId": operationID,
		"kind":        params.Kind,
		"target":      params.Target,
		"requestId":   params.RequestID,
	})
	if err != nil {
		return operation.Operation{}, fmt.Errorf("encode operation event: %w", err)
	}

	if _, err := transaction.ExecContext(ctx, `
INSERT INTO operations (
  id, kind, status, target_type, target_id, parent_operation_id, steps,
  progress, deadline, retryable, request_id, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, '[]'::jsonb, 0, $7, $8, $9, $10, $10)`,
		created.ID,
		created.Kind,
		created.Status,
		created.Target.Type,
		created.Target.ID,
		nullableString(created.ParentOperationID),
		created.Deadline,
		created.Retryable,
		created.RequestID,
		created.CreatedAt,
	); err != nil {
		return operation.Operation{}, fmt.Errorf("insert operation: %w", err)
	}

	if _, err := transaction.ExecContext(ctx, `
INSERT INTO outbox_events (
  id, aggregate_type, aggregate_id, event_type, payload, occurred_at, available_at
) VALUES ($1, 'operation', $2, 'operation.queued', $3, $4, $4)`,
		eventID,
		created.ID,
		payload,
		created.CreatedAt,
	); err != nil {
		return operation.Operation{}, fmt.Errorf("insert operation outbox event: %w", err)
	}

	return created, nil
}

func (repository *Repository) GetByID(ctx context.Context, operationID string) (operation.Operation, error) {
	if !identity.IsUUID(operationID) {
		return operation.Operation{}, operation.ErrNotFound
	}
	row := repository.database.QueryRowContext(ctx, `
SELECT
  id::text, kind, status, target_type, target_id, parent_operation_id::text,
  steps, progress, deadline, retryable, request_id, error,
  created_at, updated_at, started_at, finished_at
FROM operations
WHERE id = $1`, operationID)
	result, err := scanOperation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return operation.Operation{}, operation.ErrNotFound
	}
	if err != nil {
		return operation.Operation{}, fmt.Errorf("get operation: %w", err)
	}
	return result, nil
}

func (repository *Repository) Claim(ctx context.Context, params outbox.ClaimParams) ([]outbox.Event, error) {
	if strings.TrimSpace(params.WorkerID) == "" {
		return nil, errors.New("outbox worker ID is required")
	}
	if len(params.EventType) > 255 {
		return nil, errors.New("outbox event type must contain at most 255 characters")
	}
	if params.Limit <= 0 || params.Limit > 1000 {
		return nil, errors.New("outbox claim limit must be between 1 and 1000")
	}
	if params.LeaseDuration < time.Millisecond {
		return nil, errors.New("outbox lease duration must be at least 1ms")
	}

	rows, err := repository.database.QueryContext(ctx, `
WITH candidates AS (
  SELECT id
  FROM outbox_events
  WHERE delivered_at IS NULL
    AND dead_lettered_at IS NULL
    AND available_at <= now()
    AND (locked_until IS NULL OR locked_until <= now())
    AND ($4 = '' OR event_type = $4)
  ORDER BY available_at, occurred_at, id
  FOR UPDATE SKIP LOCKED
  LIMIT $1
)
UPDATE outbox_events AS event
SET
  locked_by = $2,
  locked_until = now() + ($3::bigint * interval '1 millisecond'),
  attempts = event.attempts + 1
FROM candidates
WHERE event.id = candidates.id
RETURNING
  event.id::text, event.aggregate_type, event.aggregate_id, event.event_type,
  event.payload, event.occurred_at, event.available_at, event.attempts,
  event.locked_by, event.locked_until, event.delivered_at,
  event.dead_lettered_at, event.last_error`,
		params.Limit,
		params.WorkerID,
		params.LeaseDuration.Milliseconds(),
		params.EventType,
	)
	if err != nil {
		return nil, fmt.Errorf("claim outbox events: %w", err)
	}
	defer rows.Close()

	events := make([]outbox.Event, 0, params.Limit)
	for rows.Next() {
		event, err := scanOutboxEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan claimed outbox event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed outbox events: %w", err)
	}
	return events, nil
}

func (repository *Repository) MarkDelivered(
	ctx context.Context,
	eventID string,
	workerID string,
	generation int,
) error {
	if err := validateOutboxLease(eventID, workerID, generation); err != nil {
		return err
	}
	result, err := repository.database.ExecContext(ctx, `
UPDATE outbox_events
SET delivered_at = now(), locked_by = NULL, locked_until = NULL, last_error = NULL
WHERE id = $1
  AND delivered_at IS NULL
  AND dead_lettered_at IS NULL
  AND locked_by = $2
  AND locked_until > now()
  AND attempts = $3`, eventID, workerID, generation)
	if err != nil {
		return fmt.Errorf("mark outbox event delivered: %w", err)
	}
	return requireAffectedOutboxEvent(result)
}

func (repository *Repository) MarkFailed(
	ctx context.Context,
	eventID string,
	workerID string,
	generation int,
	failure string,
	backoff time.Duration,
) error {
	if err := validateOutboxLease(eventID, workerID, generation); err != nil {
		return err
	}
	if backoff < time.Millisecond {
		return errors.New("outbox retry backoff must be at least 1ms")
	}
	failure = truncateRunes(failure, maxOutboxErrorRunes)
	result, err := repository.database.ExecContext(ctx, `
UPDATE outbox_events
SET
  available_at = now() + ($4::bigint * interval '1 millisecond'),
  locked_by = NULL,
  locked_until = NULL,
  last_error = $5
WHERE id = $1
  AND delivered_at IS NULL
  AND dead_lettered_at IS NULL
  AND locked_by = $2
  AND locked_until > now()
  AND attempts = $3`,
		eventID,
		workerID,
		generation,
		backoff.Milliseconds(),
		failure,
	)
	if err != nil {
		return fmt.Errorf("release failed outbox event: %w", err)
	}
	return requireAffectedOutboxEvent(result)
}

func (repository *Repository) MarkDeadLettered(
	ctx context.Context,
	eventID string,
	workerID string,
	generation int,
	failure string,
) error {
	if err := validateOutboxLease(eventID, workerID, generation); err != nil {
		return err
	}
	failure = truncateRunes(failure, maxOutboxErrorRunes)
	result, err := repository.database.ExecContext(ctx, `
UPDATE outbox_events
SET
  dead_lettered_at = now(),
  locked_by = NULL,
  locked_until = NULL,
  last_error = $4
WHERE id = $1
  AND delivered_at IS NULL
  AND dead_lettered_at IS NULL
  AND locked_by = $2
  AND locked_until > now()
  AND attempts = $3`,
		eventID,
		workerID,
		generation,
		failure,
	)
	if err != nil {
		return fmt.Errorf("mark outbox event dead-lettered: %w", err)
	}
	return requireAffectedOutboxEvent(result)
}

type scanner interface {
	Scan(...any) error
}

func scanOperation(row scanner) (operation.Operation, error) {
	var result operation.Operation
	var parentID sql.NullString
	var steps []byte
	var operationError []byte
	var deadline sql.NullTime
	var startedAt sql.NullTime
	var finishedAt sql.NullTime
	if err := row.Scan(
		&result.ID,
		&result.Kind,
		&result.Status,
		&result.Target.Type,
		&result.Target.ID,
		&parentID,
		&steps,
		&result.Progress,
		&deadline,
		&result.Retryable,
		&result.RequestID,
		&operationError,
		&result.CreatedAt,
		&result.UpdatedAt,
		&startedAt,
		&finishedAt,
	); err != nil {
		return operation.Operation{}, err
	}
	if err := json.Unmarshal(steps, &result.Steps); err != nil {
		return operation.Operation{}, fmt.Errorf("decode operation steps: %w", err)
	}
	if len(operationError) > 0 {
		result.Error = &operation.StructuredError{}
		if err := json.Unmarshal(operationError, result.Error); err != nil {
			return operation.Operation{}, fmt.Errorf("decode operation error: %w", err)
		}
	}
	if parentID.Valid {
		result.ParentOperationID = &parentID.String
	}
	if deadline.Valid {
		result.Deadline = &deadline.Time
	}
	if startedAt.Valid {
		result.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		result.FinishedAt = &finishedAt.Time
	}
	return result, nil
}

func scanOutboxEvent(row scanner) (outbox.Event, error) {
	var event outbox.Event
	var lockedBy sql.NullString
	var lockedUntil sql.NullTime
	var deliveredAt sql.NullTime
	var deadLetteredAt sql.NullTime
	var lastError sql.NullString
	if err := row.Scan(
		&event.ID,
		&event.AggregateType,
		&event.AggregateID,
		&event.EventType,
		&event.Payload,
		&event.OccurredAt,
		&event.AvailableAt,
		&event.Attempts,
		&lockedBy,
		&lockedUntil,
		&deliveredAt,
		&deadLetteredAt,
		&lastError,
	); err != nil {
		return outbox.Event{}, err
	}
	if lockedBy.Valid {
		event.LockedBy = &lockedBy.String
	}
	if lockedUntil.Valid {
		event.LockedUntil = &lockedUntil.Time
	}
	if deliveredAt.Valid {
		event.DeliveredAt = &deliveredAt.Time
	}
	if deadLetteredAt.Valid {
		event.DeadLetteredAt = &deadLetteredAt.Time
	}
	if lastError.Valid {
		event.LastError = &lastError.String
	}
	return event, nil
}

func validateOutboxLease(eventID, workerID string, generation int) error {
	if !identity.IsUUID(eventID) {
		return outbox.ErrNotFound
	}
	if strings.TrimSpace(workerID) == "" {
		return errors.New("outbox worker ID is required")
	}
	if generation < 1 {
		return errors.New("outbox claim generation must be greater than zero")
	}
	return nil
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func requireAffectedOutboxEvent(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read affected outbox rows: %w", err)
	}
	if affected == 0 {
		return outbox.ErrNotFound
	}
	return nil
}

var _ operation.Repository = (*Repository)(nil)
var _ outbox.Repository = (*Repository)(nil)
