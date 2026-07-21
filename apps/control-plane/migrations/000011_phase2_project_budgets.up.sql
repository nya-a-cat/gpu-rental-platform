CREATE TABLE project_budgets (
  project_id uuid PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  limit_minor bigint NOT NULL CHECK (limit_minor >= 0),
  generation bigint NOT NULL CHECK (generation >= 1),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);
