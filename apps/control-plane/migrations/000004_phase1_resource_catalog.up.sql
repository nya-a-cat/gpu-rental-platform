CREATE TABLE resource_classes (
  name text PRIMARY KEY CHECK (name ~ '^[a-z0-9][a-z0-9._-]{0,63}$'),
  unit text NOT NULL CHECK (unit IN ('count', 'bytes', 'millicores')),
  description text NOT NULL CHECK (char_length(description) BETWEEN 1 AND 255),
  created_at timestamptz NOT NULL
);

INSERT INTO resource_classes (name, unit, description, created_at) VALUES
  ('gpu.nvidia.full', 'count', 'Whole NVIDIA GPU devices exposed through the Kubernetes device plugin.', now()),
  ('cpu.millicores', 'millicores', 'CPU capacity measured in Kubernetes millicores.', now()),
  ('memory.bytes', 'bytes', 'Memory capacity measured in bytes.', now()),
  ('storage.bytes', 'bytes', 'Persistent storage capacity measured in bytes.', now());

CREATE TABLE clusters (
  id uuid PRIMARY KEY,
  managed_cluster_name text NOT NULL UNIQUE CHECK (managed_cluster_name ~ '^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$'),
  display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 120),
  management_state text NOT NULL CHECK (management_state IN ('enabled', 'disabled', 'draining', 'maintenance', 'quarantined')),
  connection_state text NOT NULL CHECK (connection_state IN ('connected', 'degraded', 'offline')),
  connected boolean NOT NULL,
  schedulable boolean NOT NULL,
  inventory_fresh boolean NOT NULL,
  execution_healthy boolean NOT NULL,
  fenced boolean NOT NULL DEFAULT false,
  agent_epoch text,
  report_sequence bigint NOT NULL DEFAULT 0 CHECK (report_sequence >= 0),
  fencing_token text,
  fencing_enabled boolean NOT NULL DEFAULT false,
  inventory_generation bigint NOT NULL DEFAULT 0 CHECK (inventory_generation >= 0),
  source_generation text,
  last_heartbeat_at timestamptz,
  last_inventory_at timestamptz,
  conditions jsonb NOT NULL DEFAULT '[]'::jsonb,
  generation bigint NOT NULL DEFAULT 1 CHECK (generation > 0),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  CHECK (source_generation IS NULL OR source_generation ~ '^[0-9a-f]{64}$'),
  CHECK ((agent_epoch IS NULL AND report_sequence = 0) OR agent_epoch IS NOT NULL)
);

CREATE TABLE resource_providers (
  id uuid PRIMARY KEY,
  cluster_id uuid NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
  parent_id uuid REFERENCES resource_providers(id) ON DELETE CASCADE,
  provider_type text NOT NULL CHECK (provider_type IN ('cluster', 'node_pool', 'node', 'gpu_device')),
  name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 255),
  generation bigint NOT NULL DEFAULT 1 CHECK (generation > 0),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (cluster_id, provider_type, name)
);

CREATE TABLE node_pools (
  id uuid PRIMARY KEY REFERENCES resource_providers(id) ON DELETE CASCADE,
  cluster_id uuid NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
  name text NOT NULL CHECK (name ~ '^[a-z0-9]([-a-z0-9]*[a-z0-9])?$'),
  management_state text NOT NULL CHECK (management_state IN ('enabled', 'disabled', 'draining', 'maintenance', 'quarantined')),
  last_seen_inventory_generation bigint NOT NULL CHECK (last_seen_inventory_generation >= 0),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (cluster_id, name)
);

CREATE TABLE nodes (
  id uuid PRIMARY KEY REFERENCES resource_providers(id) ON DELETE CASCADE,
  cluster_id uuid NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
  node_pool_id uuid NOT NULL REFERENCES node_pools(id) ON DELETE CASCADE,
  opaque_key text NOT NULL CHECK (char_length(opaque_key) BETWEEN 1 AND 128),
  management_state text NOT NULL CHECK (management_state IN ('enabled', 'disabled', 'draining', 'maintenance', 'quarantined')),
  health_state text NOT NULL CHECK (health_state IN ('healthy', 'degraded', 'unreachable', 'failed', 'unknown')),
  schedulable boolean NOT NULL,
  last_seen_inventory_generation bigint NOT NULL CHECK (last_seen_inventory_generation >= 0),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (cluster_id, opaque_key)
);

CREATE TABLE gpu_devices (
  id uuid PRIMARY KEY REFERENCES resource_providers(id) ON DELETE CASCADE,
  cluster_id uuid NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
  node_id uuid NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  opaque_key text NOT NULL CHECK (char_length(opaque_key) BETWEEN 1 AND 128),
  resource_class text NOT NULL REFERENCES resource_classes(name),
  model text NOT NULL CHECK (char_length(model) BETWEEN 1 AND 120),
  memory_mib bigint NOT NULL CHECK (memory_mib > 0),
  accelerator_mode text NOT NULL CHECK (accelerator_mode IN ('whole', 'mig', 'hami', 'time-slicing')),
  health_state text NOT NULL CHECK (health_state IN ('healthy', 'degraded', 'unreachable', 'failed', 'unknown')),
  allocatable boolean NOT NULL,
  last_seen_inventory_generation bigint NOT NULL CHECK (last_seen_inventory_generation >= 0),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (node_id, opaque_key)
);

CREATE TABLE resource_traits (
  resource_provider_id uuid NOT NULL REFERENCES resource_providers(id) ON DELETE CASCADE,
  trait_key text NOT NULL CHECK (trait_key ~ '^[a-z0-9][a-z0-9._/-]{0,127}$'),
  trait_value text NOT NULL CHECK (char_length(trait_value) BETWEEN 1 AND 255),
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (resource_provider_id, trait_key)
);

CREATE TABLE resource_inventories (
  resource_provider_id uuid NOT NULL REFERENCES resource_providers(id) ON DELETE CASCADE,
  resource_class text NOT NULL REFERENCES resource_classes(name),
  total bigint NOT NULL CHECK (total >= 0),
  reserved bigint NOT NULL DEFAULT 0 CHECK (reserved >= 0),
  allocated bigint NOT NULL DEFAULT 0 CHECK (allocated >= 0),
  generation bigint NOT NULL CHECK (generation > 0),
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (resource_provider_id, resource_class),
  CHECK (reserved + allocated <= total)
);

CREATE TABLE accelerator_profiles (
  id uuid PRIMARY KEY,
  name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  slug text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$'),
  accelerator_mode text NOT NULL CHECK (accelerator_mode IN ('whole', 'mig', 'hami', 'time-slicing')),
  resource_class text NOT NULL REFERENCES resource_classes(name),
  gpu_count integer NOT NULL CHECK (gpu_count > 0),
  memory_mib bigint CHECK (memory_mib IS NULL OR memory_mib > 0),
  traits jsonb NOT NULL DEFAULT '{}'::jsonb,
  enabled boolean NOT NULL DEFAULT true,
  generation bigint NOT NULL DEFAULT 1 CHECK (generation > 0),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE TABLE capacity_pools (
  id uuid PRIMARY KEY,
  name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  cluster_id uuid NOT NULL REFERENCES clusters(id),
  node_pool_id uuid NOT NULL REFERENCES node_pools(id),
  accelerator_profile_id uuid NOT NULL REFERENCES accelerator_profiles(id),
  scheduler_profile text NOT NULL CHECK (scheduler_profile IN ('none', 'hpc-volcano', 'standard-kueue')),
  total bigint NOT NULL CHECK (total >= 0),
  reserved bigint NOT NULL DEFAULT 0 CHECK (reserved >= 0),
  allocated bigint NOT NULL DEFAULT 0 CHECK (allocated >= 0),
  enabled boolean NOT NULL DEFAULT true,
  generation bigint NOT NULL DEFAULT 1 CHECK (generation > 0),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (node_pool_id, accelerator_profile_id),
  CHECK (reserved + allocated <= total)
);

CREATE INDEX resource_providers_cluster_parent_idx ON resource_providers (cluster_id, parent_id, provider_type, id);
CREATE INDEX nodes_pool_health_idx ON nodes (node_pool_id, health_state, schedulable, id);
CREATE INDEX gpu_devices_node_health_idx ON gpu_devices (node_id, health_state, allocatable, id);
CREATE INDEX capacity_pools_cluster_enabled_idx ON capacity_pools (cluster_id, enabled, id);