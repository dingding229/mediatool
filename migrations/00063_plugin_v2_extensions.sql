-- +goose Up
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='60s';
CREATE TABLE plugin_v2_snapshots(
 id uuid PRIMARY KEY,instance_id uuid NOT NULL REFERENCES plugin_v2_instances(id),
 generation uuid NOT NULL,grant_revision bigint NOT NULL CHECK(grant_revision>0),
 fingerprint text NOT NULL CHECK(fingerprint ~ '^[a-f0-9]{64}$'),
 descriptor_hash text NOT NULL CHECK(descriptor_hash ~ '^[a-f0-9]{64}$'),
 descriptors jsonb NOT NULL CHECK(jsonb_typeof(descriptors)='object' AND octet_length(descriptors::text)<=131072),
 created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(instance_id,generation)
);
CREATE TABLE plugin_v2_extensions(instance_id uuid PRIMARY KEY REFERENCES plugin_v2_instances(id),snapshot_id uuid NOT NULL REFERENCES plugin_v2_snapshots(id));
CREATE TABLE plugin_v2_schedules(
 instance_id uuid NOT NULL REFERENCES plugin_v2_instances(id),service_id text NOT NULL,
 snapshot_id uuid NOT NULL REFERENCES plugin_v2_snapshots(id),next_at timestamptz,enabled boolean NOT NULL,wake_job uuid,
 PRIMARY KEY(instance_id,service_id)
);
CREATE INDEX plugin_v2_schedules_due ON plugin_v2_schedules(next_at,instance_id) WHERE enabled;
CREATE TABLE plugin_v2_event_journal(
 cursor bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,event_id uuid NOT NULL UNIQUE,
 event_type text NOT NULL,payload jsonb NOT NULL CHECK(octet_length(payload::text)<=32768),created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE plugin_v2_subscriptions(
 instance_id uuid NOT NULL REFERENCES plugin_v2_instances(id),handler text NOT NULL,event_type text NOT NULL,
 snapshot_id uuid NOT NULL REFERENCES plugin_v2_snapshots(id),cursor bigint NOT NULL CHECK(cursor>=0),
 source_since timestamptz NOT NULL DEFAULT clock_timestamp(),polled_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(instance_id,handler)
);
CREATE TABLE plugin_v2_runs(
 id uuid PRIMARY KEY,instance_id uuid NOT NULL REFERENCES plugin_v2_instances(id),
 snapshot_id uuid NOT NULL REFERENCES plugin_v2_snapshots(id),generation uuid NOT NULL,grant_revision bigint NOT NULL,
 kind text NOT NULL CHECK(kind IN('api','service','command','action','event','chain','module','ui')),
 handler text NOT NULL,service_id text,event_cursor bigint REFERENCES plugin_v2_event_journal(cursor),
 dedupe_key text NOT NULL UNIQUE CHECK(octet_length(dedupe_key)<=512),request_hash text NOT NULL,
 args_ciphertext bytea NOT NULL CHECK(octet_length(args_ciphertext)<=262144),result_ciphertext bytea,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN('pending','running','succeeded','failed','unknown','canceled')),
 reason text NOT NULL DEFAULT '' CHECK(reason IN('','revoked','deadline','worker_lost','invalid_result','plugin_failed')),
 lease_token uuid,lease_until timestamptz,deadline_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),finished_at timestamptz,
 CHECK(state<>'running' OR (lease_token IS NOT NULL AND lease_until IS NOT NULL))
);
CREATE INDEX plugin_v2_runs_pending ON plugin_v2_runs(instance_id,created_at,id) WHERE state='pending';
CREATE INDEX plugin_v2_runs_history ON plugin_v2_runs(instance_id,created_at DESC,id DESC);
CREATE UNIQUE INDEX plugin_v2_one_service_inflight ON plugin_v2_runs(instance_id,service_id) WHERE service_id IS NOT NULL AND state IN('pending','running');
CREATE TABLE plugin_v2_manual_requests(
 key text PRIMARY KEY CHECK(octet_length(key)<=512),instance_id uuid NOT NULL REFERENCES plugin_v2_instances(id),
 request_hash text NOT NULL,job_id uuid NOT NULL,created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE plugin_v2_api_keys(
 id uuid PRIMARY KEY,instance_id uuid NOT NULL REFERENCES plugin_v2_instances(id),name text NOT NULL CHECK(octet_length(name) BETWEEN 1 AND 100),
 token_digest bytea NOT NULL UNIQUE CHECK(octet_length(token_digest)=32),fingerprint text NOT NULL,grant_revision bigint NOT NULL,
 routes jsonb NOT NULL CHECK(jsonb_typeof(routes)='array' AND jsonb_array_length(routes) BETWEEN 1 AND 64),
 expires_at timestamptz NOT NULL,revoked_at timestamptz,created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX plugin_v2_api_keys_live ON plugin_v2_api_keys(instance_id,expires_at) WHERE revoked_at IS NULL;
CREATE TABLE plugin_v2_api_limits(instance_id uuid PRIMARY KEY REFERENCES plugin_v2_instances(id),bucket timestamptz NOT NULL,used integer NOT NULL CHECK(used>0));
CREATE INDEX plugin_v2_outbox_files ON outbox_events(created_at,id) WHERE event_type IN('files.succeeded','files.failed','download.started');
-- +goose Down
SET LOCAL lock_timeout='5s';
DROP INDEX IF EXISTS plugin_v2_outbox_files;
DROP TABLE IF EXISTS plugin_v2_api_limits,plugin_v2_api_keys,plugin_v2_manual_requests,plugin_v2_runs,plugin_v2_event_journal,plugin_v2_subscriptions,plugin_v2_schedules,plugin_v2_extensions,plugin_v2_snapshots;
