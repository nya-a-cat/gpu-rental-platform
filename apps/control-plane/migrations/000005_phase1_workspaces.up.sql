CREATE TABLE workspaces (
  id uuid PRIMARY KEY,
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  cluster_id uuid NOT NULL REFERENCES clusters(id),
  accelerator_profile_id uuid NOT NULL REFERENCES accelerator_profiles(id),
  name text NOT NULL CHECK (name ~ '^[a-z0-9]([-a-z0-9]*[a-z0-9])?$'),
  gpu_count integer NOT NULL CHECK (gpu_count > 0),
  storage_gib integer NOT NULL DEFAULT 20 CHECK (storage_gib > 0 AND storage_gib <= 16384),
  namespace_name text NOT NULL CHECK (namespace_name ~ '^[a-z0-9]([-a-z0-9]*[a-z0-9])?$'),
  desired_state text NOT NULL CHECK (desired_state IN ('running', 'stopped', 'terminated')),
  observed_state text NOT NULL CHECK (observed_state IN ('pending', 'running', 'stopped', 'terminated', 'unknown')),
  provisioning_state text NOT NULL CHECK (provisioning_state IN ('pending', 'provisioning', 'succeeded', 'failed', 'deleting')),
  conditions jsonb NOT NULL DEFAULT '[]'::jsonb,
  generation bigint NOT NULL DEFAULT 1 CHECK (generation > 0),
  observed_generation bigint NOT NULL DEFAULT 0 CHECK (observed_generation >= 0),
  manifest_work_name text NOT NULL CHECK (manifest_work_name ~ '^[a-z0-9]([-a-z0-9]*[a-z0-9])?$'),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (project_id, name),
  UNIQUE (cluster_id, manifest_work_name)
);

CREATE INDEX workspaces_project_created_at_idx ON workspaces (project_id, created_at, id);
CREATE INDEX workspaces_cluster_desired_state_idx ON workspaces (cluster_id, desired_state, id);
