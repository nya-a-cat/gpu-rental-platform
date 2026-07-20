package auditarchive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresSourceWritesOrderedJSONL(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	database, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	defer database.Close()

	monthStart := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	if _, err := database.Exec(ctx, "DELETE FROM audit_events WHERE actor_id = 'archive-test'"); err != nil {
		t.Fatalf("clear prior audit events: %v", err)
	}
	ids := []string{"00000000-0000-4000-8000-000000000002", "00000000-0000-4000-8000-000000000001"}
	for index, id := range ids {
		_, err := database.Exec(ctx, `
INSERT INTO audit_events (
  id, occurred_at, actor_type, actor_id, scope_type, scope_id, action,
  resource_type, resource_id, request_id, source_ip, user_agent, outcome, details
) VALUES ($1::uuid, $2, 'service-account', 'archive-test', 'system', NULL, 'test.export',
  'audit-event', $1::uuid::text, $3, '192.0.2.10', 'integration-test', 'succeeded', $4::jsonb)
`, id, monthStart.Add(time.Duration(index)*time.Second), "audit-archive-"+id, fmt.Sprintf(`{"index":%d}`, index))
		if err != nil {
			t.Fatalf("insert audit event: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = database.Exec(context.Background(), "DELETE FROM audit_events WHERE actor_id = 'archive-test'")
	})

	var output bytes.Buffer
	count, err := NewPostgresSource(database).WriteJSONL(ctx, monthStart, monthStart.AddDate(0, 1, 0), &output)
	if err != nil {
		t.Fatalf("WriteJSONL() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
	decoder := json.NewDecoder(&output)
	var first, second auditEvent
	if err := decoder.Decode(&first); err != nil {
		t.Fatalf("decode first event: %v", err)
	}
	if err := decoder.Decode(&second); err != nil {
		t.Fatalf("decode second event: %v", err)
	}
	if first.ID != ids[0] || second.ID != ids[1] {
		t.Fatalf("event order = %s/%s", first.ID, second.ID)
	}
	if first.SourceIP == nil || *first.SourceIP != "192.0.2.10/32" {
		t.Fatalf("source IP = %#v", first.SourceIP)
	}
}
