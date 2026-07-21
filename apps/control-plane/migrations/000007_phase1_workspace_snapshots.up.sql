CREATE TABLE workspace_snapshots (
  id uuid PRIMARY KEY,
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  name text NOT NULL CHECK (name ~ '^[a-z0-9]([-a-z0-9]*[a-z0-9])?$'),
  source_pvc_name text NOT NULL CHECK (source_pvc_name ~ '^[a-z0-9]([-a-z0-9]*[a-z0-9])?$'),
  state text NOT NULL CHECK (state IN ('pending', 'succeeded', 'failed')),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (workspace_id, name)
);

CREATE INDEX workspace_snapshots_workspace_created_idx ON workspace_snapshots (workspace_id, created_at, id);
