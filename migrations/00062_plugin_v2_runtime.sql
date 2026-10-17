-- +goose Up
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='60s';
-- Retain all legacy ciphertext and foreign keys. Nothing translates or executes WASM.
ALTER TABLE extension_plugins RENAME TO plugin_legacy_wasm_plugins;
ALTER TABLE extension_deliveries RENAME TO plugin_legacy_wasm_deliveries;
ALTER TABLE extension_runs RENAME TO plugin_legacy_wasm_runs;
ALTER TABLE extension_repositories RENAME TO plugin_legacy_wasm_repositories;
ALTER TABLE extension_jobs RENAME TO plugin_legacy_wasm_jobs;
ALTER TABLE extension_service_ticks RENAME TO plugin_legacy_wasm_service_ticks;

CREATE TABLE plugin_v2_definitions (
 id text PRIMARY KEY CHECK(id ~ '^[a-z][a-z0-9-]{2,63}$'),
 manifest jsonb NOT NULL CHECK(jsonb_typeof(manifest)='object' AND octet_length(manifest::text)<=131072),
 fingerprint text NOT NULL CHECK(fingerprint ~ '^[a-f0-9]{64}$'),
 availability text NOT NULL DEFAULT 'available' CHECK(availability IN('available','missing','unsupported')),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0)
);
CREATE TABLE plugin_v2_instances (
 id uuid PRIMARY KEY,
 plugin_id text NOT NULL REFERENCES plugin_v2_definitions(id),
 clone_of uuid REFERENCES plugin_v2_instances(id),
 desired_state text NOT NULL DEFAULT 'disabled' CHECK(desired_state IN('enabled','disabled','absent')),
 runtime_state text NOT NULL DEFAULT 'stopped' CHECK(runtime_state IN('stopped','starting','running','stopping','backoff','needs_action')),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 grant_revision bigint NOT NULL DEFAULT 1 CHECK(grant_revision>0),
 approved_fingerprint text NOT NULL CHECK(approved_fingerprint ~ '^[a-f0-9]{64}$'),
 generation uuid,
 owner_id uuid,
 worker_pid integer NOT NULL DEFAULT 0 CHECK(worker_pid>=0),
 worker_start text NOT NULL DEFAULT '',
 lease_until timestamptz,
 restart_window timestamptz NOT NULL DEFAULT now(),
 restart_count integer NOT NULL DEFAULT 0 CHECK(restart_count BETWEEN 0 AND 10),
 next_restart_at timestamptz NOT NULL DEFAULT now(),
 reason text NOT NULL DEFAULT '' CHECK(reason IN('','worker_failed','resource_limit','permission_review_required','definition_missing','definition_unsupported','data_unavailable')),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK((generation IS NULL)=(owner_id IS NULL)),
 CHECK((generation IS NULL)=(lease_until IS NULL))
);
CREATE INDEX plugin_v2_instances_due ON plugin_v2_instances(next_restart_at,id) WHERE desired_state='enabled';
CREATE TABLE plugin_v2_configs (
 id uuid PRIMARY KEY REFERENCES plugin_v2_instances(id) ON DELETE CASCADE,
 ciphertext bytea NOT NULL CHECK(octet_length(ciphertext) BETWEEN 28 AND 262144),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0)
);
CREATE TABLE plugin_v2_kv (
 id uuid PRIMARY KEY,
 instance_id uuid NOT NULL REFERENCES plugin_v2_instances(id) ON DELETE CASCADE,
 key text NOT NULL CHECK(octet_length(key) BETWEEN 1 AND 128),
 ciphertext bytea NOT NULL CHECK(octet_length(ciphertext) BETWEEN 28 AND 262144),
 revision bigint NOT NULL CHECK(revision>0),
 UNIQUE(instance_id,key)
);
CREATE TABLE plugin_v2_grants (
 instance_id uuid NOT NULL REFERENCES plugin_v2_instances(id) ON DELETE CASCADE,
 scope text NOT NULL CHECK(octet_length(scope) BETWEEN 1 AND 120 AND position('*' IN scope)=0),
 fingerprint text NOT NULL CHECK(fingerprint ~ '^[a-f0-9]{64}$'),
 PRIMARY KEY(instance_id,scope)
);
CREATE TABLE plugin_v2_operations (
 key text PRIMARY KEY CHECK(octet_length(key) BETWEEN 1 AND 256),
 request_digest text NOT NULL CHECK(request_digest ~ '^[a-f0-9]{64}$'),
 kind text NOT NULL CHECK(kind IN('install','enable','disable','clone','configure','review','remove')),
 instance_id uuid NOT NULL REFERENCES plugin_v2_instances(id),
 result jsonb NOT NULL CHECK(octet_length(result::text)<=4096),
 created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='60s';
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM plugin_v2_instances) THEN
  RAISE EXCEPTION 'V2 data exists; restore a backup instead of a destructive downgrade';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE plugin_v2_operations,plugin_v2_grants,plugin_v2_kv,plugin_v2_configs,plugin_v2_instances,plugin_v2_definitions;
ALTER TABLE plugin_legacy_wasm_plugins RENAME TO extension_plugins;
ALTER TABLE plugin_legacy_wasm_deliveries RENAME TO extension_deliveries;
ALTER TABLE plugin_legacy_wasm_runs RENAME TO extension_runs;
ALTER TABLE plugin_legacy_wasm_repositories RENAME TO extension_repositories;
ALTER TABLE plugin_legacy_wasm_jobs RENAME TO extension_jobs;
ALTER TABLE plugin_legacy_wasm_service_ticks RENAME TO extension_service_ticks;
UPDATE extension_plugins SET enabled=false;
