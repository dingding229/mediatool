-- +goose Up
-- Historical links retain their identity but are NOT backfilled with today's
-- mutable resource metadata. Their absent snapshot requires a fresh search.
ALTER TABLE search_run_resources ADD COLUMN id uuid NOT NULL DEFAULT gen_random_uuid();
CREATE UNIQUE INDEX search_run_resources_snapshot_identity ON search_run_resources(id);
ALTER TABLE search_run_resources ADD COLUMN public_snapshot jsonb;
ALTER TABLE search_run_resources ADD COLUMN download_hint_ciphertext bytea;
ALTER TABLE search_run_resources ADD COLUMN account_revision bigint;
ALTER TABLE search_run_resources ADD COLUMN definition_fingerprint text;
ALTER TABLE search_run_resources ADD COLUMN observed_at timestamptz;
ALTER TABLE search_run_resources ADD CONSTRAINT search_run_snapshot_complete CHECK (
 (public_snapshot IS NULL AND download_hint_ciphertext IS NULL AND account_revision IS NULL AND definition_fingerprint IS NULL AND observed_at IS NULL)
 OR (public_snapshot IS NOT NULL AND download_hint_ciphertext IS NOT NULL AND account_revision IS NOT NULL AND definition_fingerprint IS NOT NULL AND observed_at IS NOT NULL)
);
ALTER TABLE search_site_results ADD COLUMN source_revision bigint;
ALTER TABLE search_site_results ADD COLUMN definition_fingerprint text;
ALTER TABLE search_site_results ADD COLUMN observed_at timestamptz;
ALTER TABLE subscription_runs ADD COLUMN collection_state text NOT NULL DEFAULT 'collecting' CHECK(collection_state IN ('collecting','collected','needs_refresh'));
ALTER TABLE subscription_runs ADD COLUMN collected_at timestamptz;
-- +goose Down
ALTER TABLE search_site_results DROP COLUMN observed_at,DROP COLUMN definition_fingerprint,DROP COLUMN source_revision;
ALTER TABLE subscription_runs DROP COLUMN collected_at,DROP COLUMN collection_state;
ALTER TABLE search_run_resources DROP CONSTRAINT search_run_snapshot_complete;
ALTER TABLE search_run_resources DROP COLUMN observed_at,DROP COLUMN definition_fingerprint,DROP COLUMN account_revision,DROP COLUMN download_hint_ciphertext,DROP COLUMN public_snapshot;
DROP INDEX search_run_resources_snapshot_identity;
ALTER TABLE search_run_resources DROP COLUMN id;
