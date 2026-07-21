CREATE TABLE ledger_entries (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES projects(id),
  usage_fact_id uuid NOT NULL REFERENCES usage_facts(id),
  entry_type text NOT NULL CHECK (entry_type IN ('debit', 'credit')),
  amount_minor bigint NOT NULL CHECK (amount_minor >= 0),
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  reference_id text NOT NULL,
  created_at timestamptz NOT NULL,
  UNIQUE (usage_fact_id, entry_type)
);

CREATE INDEX ledger_entries_project_created_idx ON ledger_entries (project_id, created_at, id);

CREATE TABLE invoices (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES projects(id),
  period_from timestamptz NOT NULL,
  period_to timestamptz NOT NULL,
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  subtotal_minor bigint NOT NULL CHECK (subtotal_minor >= 0),
  status text NOT NULL CHECK (status IN ('issued', 'void')),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  CHECK (period_to > period_from),
  UNIQUE (project_id, period_from, period_to)
);

CREATE TABLE invoice_lines (
  id uuid PRIMARY KEY,
  invoice_id uuid NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
  usage_fact_id uuid NOT NULL REFERENCES usage_facts(id),
  amount_minor bigint NOT NULL CHECK (amount_minor >= 0),
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  UNIQUE (invoice_id, usage_fact_id)
);
