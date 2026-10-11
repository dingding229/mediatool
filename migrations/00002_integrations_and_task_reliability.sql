-- +goose Up
CREATE TABLE integrations (
 id uuid PRIMARY KEY,
 kind text NOT NULL CHECK(kind IN ('clouddrive2','qbittorrent','emby','tmdb','telegram','ptsite')),
 name text NOT NULL,
 enabled boolean NOT NULL DEFAULT false,
 settings jsonb NOT NULL DEFAULT '{}',
 credentials_ciphertext bytea NOT NULL,
 key_version integer NOT NULL DEFAULT 1,
 revision bigint NOT NULL DEFAULT 1,
 last_check_state text NOT NULL DEFAULT 'unchecked',
 last_check_message text NOT NULL DEFAULT '',
 last_checked_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX integrations_one_enabled_kind ON integrations(kind) WHERE enabled AND kind <> 'ptsite';
ALTER TABLE jobs ADD COLUMN idempotency_key text;
ALTER TABLE jobs ADD COLUMN queue text NOT NULL DEFAULT 'default';
ALTER TABLE jobs ADD COLUMN result jsonb;
CREATE UNIQUE INDEX jobs_idempotency_idx ON jobs(idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX jobs_queue_available_idx ON jobs(queue,available_at) WHERE state IN ('pending','retry');
CREATE TABLE worker_heartbeats (
 owner text PRIMARY KEY,
 queue text NOT NULL,
 last_seen_at timestamptz NOT NULL DEFAULT now(),
 active_job_id uuid REFERENCES jobs(id) ON DELETE SET NULL
);
CREATE TABLE notification_messages (
 id uuid PRIMARY KEY,
 event_id uuid NOT NULL UNIQUE REFERENCES outbox_events(id),
 title text NOT NULL,
 body text NOT NULL,
 severity text NOT NULL DEFAULT 'info',
 read_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE bootstrap_imports (
 name text PRIMARY KEY,
 imported_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
DROP TABLE bootstrap_imports,notification_messages,worker_heartbeats;
DROP INDEX jobs_queue_available_idx;
DROP INDEX jobs_idempotency_idx;
ALTER TABLE jobs DROP COLUMN result,DROP COLUMN queue,DROP COLUMN idempotency_key;
DROP TABLE integrations;
