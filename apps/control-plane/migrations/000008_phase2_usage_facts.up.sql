CREATE TABLE usage_facts (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES projects(id),
  resource_class text NOT NULL,
  quantity text NOT NULL,
  allocation_from timestamptz NOT NULL,
  allocation_to timestamptz NOT NULL,
  attributes jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL,
  CHECK (allocation_to > allocation_from)
);

CREATE INDEX usage_facts_project_interval_idx ON usage_facts (project_id, allocation_from, allocation_to);
CREATE INDEX usage_facts_tenant_created_idx ON usage_facts (tenant_id, created_at, id);

CREATE TABLE rated_usage (
  usage_fact_id uuid PRIMARY KEY REFERENCES usage_facts(id) ON DELETE CASCADE,
  price_book_id text NOT NULL,
  price_version integer NOT NULL CHECK (price_version > 0),
  amount_minor bigint NOT NULL CHECK (amount_minor >= 0),
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  calculation jsonb NOT NULL,
  calculated_at timestamptz NOT NULL
);
