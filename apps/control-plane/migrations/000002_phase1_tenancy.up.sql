CREATE TABLE tenants (
  id uuid PRIMARY KEY,
  name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  slug text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$'),
  status text NOT NULL CHECK (status IN ('active', 'suspended')),
  generation bigint NOT NULL DEFAULT 1 CHECK (generation > 0),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE TABLE projects (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  slug text NOT NULL CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$'),
  isolation_class text NOT NULL CHECK (isolation_class IN ('shared', 'dedicated-node-pool', 'dedicated-cluster')),
  namespace_name text NOT NULL UNIQUE CHECK (namespace_name ~ '^[a-z0-9]([-a-z0-9]*[a-z0-9])?$'),
  desired_state text NOT NULL CHECK (desired_state IN ('active', 'suspended', 'deleted')),
  observed_state text NOT NULL CHECK (observed_state IN ('pending', 'active', 'suspended', 'deleted', 'unknown')),
  provisioning_state text NOT NULL CHECK (provisioning_state IN ('pending', 'provisioning', 'succeeded', 'failed', 'deleting')),
  conditions jsonb NOT NULL DEFAULT '[]'::jsonb,
  generation bigint NOT NULL DEFAULT 1 CHECK (generation > 0),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (tenant_id, slug)
);

CREATE INDEX projects_tenant_created_at_idx ON projects (tenant_id, created_at, id);

CREATE TABLE role_bindings (
  id uuid PRIMARY KEY,
  scope_type text NOT NULL CHECK (scope_type IN ('tenant', 'project')),
  scope_id uuid NOT NULL,
  subject_type text NOT NULL CHECK (subject_type IN ('user', 'group', 'service_account')),
  subject_id text NOT NULL CHECK (char_length(subject_id) BETWEEN 1 AND 255),
  role text NOT NULL CHECK (role IN (
    'tenant_owner', 'project_admin', 'operator', 'developer', 'viewer',
    'billing_admin', 'auditor', 'service_account'
  )),
  created_by text NOT NULL CHECK (char_length(created_by) BETWEEN 1 AND 255),
  created_at timestamptz NOT NULL,
  UNIQUE (scope_type, scope_id, subject_type, subject_id, role)
);

CREATE INDEX role_bindings_subject_idx
  ON role_bindings (subject_type, subject_id, scope_type, scope_id);
CREATE INDEX role_bindings_scope_idx
  ON role_bindings (scope_type, scope_id, created_at, id);

CREATE TABLE project_quotas (
  project_id uuid NOT NULL REFERENCES projects(id),
  resource_class text NOT NULL CHECK (resource_class ~ '^[a-z0-9][a-z0-9._-]{0,63}$'),
  hard_limit bigint NOT NULL CHECK (hard_limit >= 0),
  reserved bigint NOT NULL DEFAULT 0 CHECK (reserved >= 0),
  allocated bigint NOT NULL DEFAULT 0 CHECK (allocated >= 0),
  generation bigint NOT NULL DEFAULT 1 CHECK (generation > 0),
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (project_id, resource_class),
  CHECK (reserved + allocated <= hard_limit)
);

CREATE TABLE quota_reservations (
  id uuid PRIMARY KEY,
  project_id uuid NOT NULL,
  resource_class text NOT NULL,
  amount bigint NOT NULL CHECK (amount > 0),
  status text NOT NULL CHECK (status IN ('pending', 'committed', 'released', 'expired')),
  operation_id uuid NOT NULL UNIQUE REFERENCES operations(id),
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  FOREIGN KEY (project_id, resource_class)
    REFERENCES project_quotas(project_id, resource_class)
);

CREATE INDEX quota_reservations_expiry_idx
  ON quota_reservations (expires_at, id)
  WHERE status = 'pending';
CREATE INDEX quota_reservations_project_idx
  ON quota_reservations (project_id, resource_class, created_at, id);
