CREATE TABLE workspace_access_tokens (
  id uuid PRIMARY KEY,
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  access_type text NOT NULL CHECK (access_type IN ('ssh', 'web-terminal', 'jupyter')),
  token_hash text NOT NULL UNIQUE CHECK (token_hash ~ '^[0-9a-f]{64}$'),
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  created_by text NOT NULL CHECK (char_length(created_by) BETWEEN 1 AND 255),
  created_at timestamptz NOT NULL
);

CREATE INDEX workspace_access_tokens_active_idx
  ON workspace_access_tokens (workspace_id, access_type, expires_at)
  WHERE revoked_at IS NULL;
