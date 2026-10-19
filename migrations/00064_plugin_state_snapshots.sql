-- +goose Up
SET LOCAL lock_timeout='5s';
ALTER TABLE plugin_v2_instances ADD COLUMN data_operation uuid;
CREATE TABLE plugin_v2_state_snapshots (
 id uuid PRIMARY KEY, instance_id uuid NOT NULL REFERENCES plugin_v2_instances(id),
 plugin_id text NOT NULL, fingerprint text NOT NULL CHECK(fingerprint ~ '^[a-f0-9]{64}$'),
 config_schema integer NOT NULL CHECK(config_schema>0), data_schema integer NOT NULL CHECK(data_schema>0),
 operation_revision bigint NOT NULL CHECK(operation_revision>0),
 state text NOT NULL CHECK(state IN('writing','ready','failed','deleted')),
 bytes bigint NOT NULL CHECK(bytes BETWEEN 0 AND 83886080), files integer NOT NULL DEFAULT 0 CHECK(files BETWEEN 0 AND 4096),
 created_at timestamptz NOT NULL DEFAULT now(), data_path text NOT NULL CHECK(data_path ~ '^snapshots/[a-f0-9-]{36}/data$')
);
CREATE INDEX plugin_v2_state_snapshots_instance ON plugin_v2_state_snapshots(instance_id,created_at,id);
CREATE TABLE plugin_v2_state_values (
 id uuid PRIMARY KEY, snapshot_id uuid NOT NULL REFERENCES plugin_v2_state_snapshots(id) ON DELETE CASCADE,
 kind text NOT NULL CHECK(kind IN('config','kv')), key text NOT NULL CHECK(octet_length(key)<=128),
 revision bigint NOT NULL CHECK(revision>0), ciphertext bytea NOT NULL CHECK(octet_length(ciphertext) BETWEEN 28 AND 262144),
 UNIQUE(snapshot_id,kind,key), CHECK((kind='config' AND key='') OR (kind='kv' AND key<>''))
);
CREATE TABLE plugin_v2_state_tasks (
 id uuid PRIMARY KEY, instance_id uuid NOT NULL REFERENCES plugin_v2_instances(id), snapshot_id uuid NOT NULL REFERENCES plugin_v2_state_snapshots(id),
 operation_key text NOT NULL UNIQUE CHECK(octet_length(operation_key) BETWEEN 1 AND 256),
 request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'), kind text NOT NULL CHECK(kind IN('create','restore','delete')),
 reserved_bytes bigint NOT NULL DEFAULT 0 CHECK(reserved_bytes BETWEEN 0 AND 83886080),
 state text NOT NULL CHECK(state IN('writing','prepared','done','failed')), lease_until timestamptz NOT NULL,
 revision bigint NOT NULL, fingerprint text NOT NULL, stage_identity text NOT NULL DEFAULT '',
 cleaned boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now(), result jsonb NOT NULL DEFAULT '{}'
);
CREATE UNIQUE INDEX plugin_v2_state_tasks_active ON plugin_v2_state_tasks(instance_id) WHERE state IN('writing','prepared');
CREATE INDEX plugin_v2_state_tasks_gc ON plugin_v2_state_tasks(created_at,id) WHERE NOT cleaned OR state IN('done','failed');
CREATE INDEX plugin_v2_state_values_snapshot ON plugin_v2_state_values(snapshot_id);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM plugin_v2_state_snapshots WHERE state IN('ready','writing')) THEN
  RAISE EXCEPTION 'instance snapshots exist; restore backup instead of destructive downgrade';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE plugin_v2_state_tasks,plugin_v2_state_values,plugin_v2_state_snapshots;
ALTER TABLE plugin_v2_instances DROP COLUMN data_operation;
