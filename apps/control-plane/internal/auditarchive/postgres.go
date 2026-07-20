package auditarchive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresSource struct {
	database *pgxpool.Pool
}

type auditEvent struct {
	ID           string          `json:"id"`
	OccurredAt   time.Time       `json:"occurredAt"`
	ActorType    string          `json:"actorType"`
	ActorID      *string         `json:"actorId"`
	ScopeType    string          `json:"scopeType"`
	ScopeID      *string         `json:"scopeId"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resourceType"`
	ResourceID   *string         `json:"resourceId"`
	RequestID    string          `json:"requestId"`
	SourceIP     *string         `json:"sourceIp"`
	UserAgent    *string         `json:"userAgent"`
	Outcome      string          `json:"outcome"`
	Details      json.RawMessage `json:"details"`
}

func NewPostgresSource(database *pgxpool.Pool) *PostgresSource {
	return &PostgresSource{database: database}
}

func (source *PostgresSource) WriteJSONL(ctx context.Context, start, end time.Time, destination io.Writer) (int64, error) {
	rows, err := source.database.Query(ctx, `
SELECT
  id::text,
  occurred_at,
  actor_type,
  actor_id,
  scope_type,
  scope_id,
  action,
  resource_type,
  resource_id,
  request_id,
  source_ip::text,
  user_agent,
  outcome,
  details::text
FROM audit_events
WHERE occurred_at >= $1 AND occurred_at < $2
ORDER BY occurred_at, id
`, start, end)
	if err != nil {
		return 0, fmt.Errorf("query audit events: %w", err)
	}
	defer rows.Close()

	encoder := json.NewEncoder(destination)
	encoder.SetEscapeHTML(false)
	var count int64
	for rows.Next() {
		var event auditEvent
		var actorID, scopeID, resourceID, sourceIP, userAgent pgtype.Text
		var details string
		if err := rows.Scan(
			&event.ID,
			&event.OccurredAt,
			&event.ActorType,
			&actorID,
			&event.ScopeType,
			&scopeID,
			&event.Action,
			&event.ResourceType,
			&resourceID,
			&event.RequestID,
			&sourceIP,
			&userAgent,
			&event.Outcome,
			&details,
		); err != nil {
			return 0, fmt.Errorf("scan audit event: %w", err)
		}
		event.OccurredAt = event.OccurredAt.UTC()
		event.ActorID = textPointer(actorID)
		event.ScopeID = textPointer(scopeID)
		event.ResourceID = textPointer(resourceID)
		event.SourceIP = textPointer(sourceIP)
		event.UserAgent = textPointer(userAgent)
		event.Details = json.RawMessage(details)
		if !json.Valid(event.Details) {
			return 0, fmt.Errorf("audit event %s contains invalid details JSON", event.ID)
		}
		if err := encoder.Encode(event); err != nil {
			return 0, fmt.Errorf("encode audit event %s: %w", event.ID, err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate audit events: %w", err)
	}
	return count, nil
}

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}
