-- +goose Up
ALTER TABLE jobs DROP CONSTRAINT jobs_state_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_state_check CHECK(state IN ('pending','running','retry','succeeded','failed','paused','cancelled'));
ALTER TABLE managed_media_artifacts ADD COLUMN owner_scope text NOT NULL DEFAULT '';
UPDATE managed_media_artifacts SET owner_scope='asset:'||asset_id::text;
CREATE TABLE media_artifact_journal (
 id uuid PRIMARY KEY,
 relative_path text NOT NULL,
 root_path text NOT NULL,
 asset_id uuid NOT NULL REFERENCES media_assets(id),
 owner_scope text NOT NULL,
 artifact_kind text NOT NULL,
 content_ciphertext bytea,
 content_digest bytea NOT NULL,
 expected_digest bytea,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','applied','conflict')),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX artifact_one_pending_path ON media_artifact_journal(relative_path) WHERE state IN ('pending','conflict');
ALTER TABLE media_assets ADD COLUMN revision bigint NOT NULL DEFAULT 1;
ALTER TABLE transfer_tasks ADD COLUMN hash_bytes bigint NOT NULL DEFAULT 0;
ALTER TABLE transfer_tasks ADD COLUMN transferred_ranges int8multirange NOT NULL DEFAULT '{}';
ALTER TABLE transfer_tasks ADD COLUMN control_revision bigint NOT NULL DEFAULT 1;
CREATE TABLE library_scan_schedules (
 id uuid PRIMARY KEY,
 integration_id uuid NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
 root text NOT NULL,
 enabled boolean NOT NULL DEFAULT false,
 interval_seconds integer NOT NULL CHECK(interval_seconds BETWEEN 300 AND 604800),
 next_run_at timestamptz NOT NULL DEFAULT now(),
 revision bigint NOT NULL DEFAULT 1,
 last_scan_id uuid REFERENCES library_scans(id),
 last_error text NOT NULL DEFAULT '',
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(integration_id,root)
);
-- +goose Down
UPDATE jobs SET state='failed' WHERE state IN ('paused','cancelled');
ALTER TABLE jobs DROP CONSTRAINT jobs_state_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_state_check CHECK(state IN ('pending','running','retry','succeeded','failed'));
DROP TABLE library_scan_schedules;
ALTER TABLE transfer_tasks DROP COLUMN control_revision, DROP COLUMN hash_bytes, DROP COLUMN transferred_ranges;
ALTER TABLE media_assets DROP COLUMN revision;
DROP TABLE media_artifact_journal;
ALTER TABLE managed_media_artifacts DROP COLUMN owner_scope;
