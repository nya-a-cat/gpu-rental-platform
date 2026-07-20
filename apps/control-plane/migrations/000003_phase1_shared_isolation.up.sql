ALTER TABLE projects
  ADD COLUMN target_cluster_id text,
  ADD COLUMN manifest_work_name text,
  ADD COLUMN observed_generation bigint NOT NULL DEFAULT 0 CHECK (observed_generation >= 0),
  ADD COLUMN applied_gpu_quota bigint NOT NULL DEFAULT 0 CHECK (applied_gpu_quota >= 0),
  ADD COLUMN last_reconciled_at timestamptz;

ALTER TABLE projects
  ADD CONSTRAINT projects_target_cluster_id_format
  CHECK (
    target_cluster_id IS NULL OR
    target_cluster_id ~ '^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$'
  ),
  ADD CONSTRAINT projects_manifest_work_name_format
  CHECK (
    manifest_work_name IS NULL OR
    manifest_work_name ~ '^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$'
  ),
  ADD CONSTRAINT projects_work_identity_complete
  CHECK (
    (target_cluster_id IS NULL AND manifest_work_name IS NULL) OR
    (target_cluster_id IS NOT NULL AND manifest_work_name IS NOT NULL)
  );

CREATE UNIQUE INDEX projects_manifest_work_identity_idx
  ON projects (target_cluster_id, manifest_work_name)
  WHERE target_cluster_id IS NOT NULL AND manifest_work_name IS NOT NULL;
