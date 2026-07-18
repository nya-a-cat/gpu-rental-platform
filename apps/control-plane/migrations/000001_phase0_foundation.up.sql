CREATE TABLE operations (
  id uuid PRIMARY KEY,
  kind text NOT NULL,
  status text NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled', 'timed_out')),
  target_type text NOT NULL,
  target_id text NOT NULL,
  parent_operation_id uuid REFERENCES operations(id),
  steps jsonb NOT NULL DEFAULT '[]'::jsonb,
  progress smallint NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
  deadline timestamptz,
  retryable boolean NOT NULL DEFAULT false,
  request_id text NOT NULL,
  error jsonb,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  started_at timestamptz,
  finished_at timestamptz
);

CREATE INDEX operations_status_created_at_idx ON operations (status, created_at);
CREATE INDEX operations_target_idx ON operations (target_type, target_id, created_at DESC);
CREATE INDEX operations_request_id_idx ON operations (request_id);

CREATE TABLE outbox_events (
  id uuid PRIMARY KEY,
  aggregate_type text NOT NULL,
  aggregate_id text NOT NULL,
  event_type text NOT NULL,
  payload jsonb NOT NULL,
  occurred_at timestamptz NOT NULL,
  available_at timestamptz NOT NULL,
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  locked_by text,
  locked_until timestamptz,
  delivered_at timestamptz,
  dead_lettered_at timestamptz,
  last_error text
);

CREATE INDEX outbox_events_pending_idx
  ON outbox_events (available_at, occurred_at, id)
  WHERE delivered_at IS NULL AND dead_lettered_at IS NULL;

CREATE TABLE idempotency_records (
  scope text NOT NULL,
  idempotency_key text NOT NULL,
  request_hash text NOT NULL,
  response_status integer,
  response_headers jsonb,
  response_body jsonb,
  resource_type text,
  resource_id text,
  operation_id uuid REFERENCES operations(id),
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL,
  PRIMARY KEY (scope, idempotency_key)
);

CREATE INDEX idempotency_records_expires_at_idx ON idempotency_records (expires_at);

CREATE TABLE audit_events (
  id uuid NOT NULL,
  occurred_at timestamptz NOT NULL,
  actor_type text NOT NULL,
  actor_id text,
  scope_type text NOT NULL,
  scope_id text,
  action text NOT NULL,
  resource_type text NOT NULL,
  resource_id text,
  request_id text NOT NULL,
  source_ip inet,
  user_agent text,
  outcome text NOT NULL,
  details jsonb NOT NULL DEFAULT '{}'::jsonb,
  PRIMARY KEY (occurred_at, id)
) PARTITION BY RANGE (occurred_at);

DO $$
DECLARE
  current_month_start date := date_trunc('month', CURRENT_DATE)::date;
  next_month_start date := (current_month_start + interval '1 month')::date;
  following_month_start date := (current_month_start + interval '2 months')::date;
BEGIN
  EXECUTE format(
    'CREATE TABLE audit_events_%s PARTITION OF audit_events FOR VALUES FROM (%L) TO (%L)',
    to_char(current_month_start, 'YYYY_MM'),
    current_month_start,
    next_month_start
  );
  EXECUTE format(
    'CREATE TABLE audit_events_%s PARTITION OF audit_events FOR VALUES FROM (%L) TO (%L)',
    to_char(next_month_start, 'YYYY_MM'),
    next_month_start,
    following_month_start
  );
END
$$;

CREATE TABLE audit_events_default PARTITION OF audit_events DEFAULT;

CREATE INDEX audit_events_scope_occurred_at_idx
  ON audit_events (scope_type, scope_id, occurred_at DESC);
CREATE INDEX audit_events_resource_occurred_at_idx
  ON audit_events (resource_type, resource_id, occurred_at DESC);
CREATE INDEX audit_events_request_id_idx ON audit_events (request_id);
