CREATE TABLE placement_decisions (
  id uuid PRIMARY KEY,
  project_id uuid NOT NULL REFERENCES projects(id),
  capacity_pool_id uuid NOT NULL REFERENCES capacity_pools(id),
  cluster_id uuid NOT NULL REFERENCES clusters(id),
  node_pool_id uuid NOT NULL REFERENCES node_pools(id),
  accelerator_profile_id uuid NOT NULL REFERENCES accelerator_profiles(id),
  quantity integer NOT NULL CHECK (quantity > 0),
  traits jsonb NOT NULL DEFAULT '{}'::jsonb,
  status text NOT NULL CHECK (status IN ('reserved', 'released', 'committed')),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE INDEX placement_decisions_project_created_idx ON placement_decisions (project_id, created_at, id);
