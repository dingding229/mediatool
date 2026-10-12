-- +goose Up
CREATE TABLE site_metrics (
 account_id uuid PRIMARY KEY REFERENCES integrations(id) ON DELETE CASCADE,
 metrics jsonb NOT NULL DEFAULT '{}'::jsonb,
 collected_at timestamptz,
 attempted_at timestamptz NOT NULL DEFAULT now(),
 state text NOT NULL DEFAULT 'not_synced',
 message text NOT NULL DEFAULT '',
 account_revision bigint NOT NULL DEFAULT 0,
 definition_fingerprint text NOT NULL DEFAULT ''
);
ALTER TABLE extension_plugins ADD COLUMN config_ciphertext bytea;
ALTER TABLE extension_plugins ADD COLUMN package_digest text NOT NULL DEFAULT '';
ALTER TABLE extension_plugins ADD COLUMN package_files jsonb NOT NULL DEFAULT '[]'::jsonb;
CREATE TABLE extension_runs (
 id uuid PRIMARY KEY,
 plugin_id text NOT NULL REFERENCES extension_plugins(id) ON DELETE CASCADE,
 kind text NOT NULL,
 state text NOT NULL,
 message text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 finished_at timestamptz
);
CREATE INDEX extension_runs_plugin ON extension_runs(plugin_id,created_at DESC);
-- +goose Down
DROP TABLE extension_runs;
ALTER TABLE extension_plugins DROP COLUMN config_ciphertext,DROP COLUMN package_digest,DROP COLUMN package_files;
DROP TABLE site_metrics;
