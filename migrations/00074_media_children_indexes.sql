-- +goose Up
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='60s';
-- Parent/season episode paging and same-directory external subtitle lookup.
CREATE INDEX media_asset_bindings_parent_episode_idx ON media_asset_bindings(media_id,season,episode,asset_id);
CREATE INDEX cloud_files_directory_idx ON cloud_files(storage_id,(regexp_replace(full_path,'/[^/]+$',''))) WHERE state='present';
-- Namespace content fingerprints so legacy build identities can migrate once,
-- without confusing a later same-manifest handler change with legacy adoption.
ALTER TABLE plugin_v2_definitions DROP CONSTRAINT plugin_v2_definitions_fingerprint_check;
ALTER TABLE plugin_v2_definitions ADD CHECK(fingerprint ~ '^(content-v1:)?[a-f0-9]{64}$');
ALTER TABLE plugin_v2_instances DROP CONSTRAINT plugin_v2_instances_approved_fingerprint_check;
ALTER TABLE plugin_v2_instances ADD CHECK(approved_fingerprint ~ '^(content-v1:)?[a-f0-9]{64}$');
ALTER TABLE plugin_v2_grants DROP CONSTRAINT plugin_v2_grants_fingerprint_check;
ALTER TABLE plugin_v2_grants ADD CHECK(fingerprint ~ '^(content-v1:)?[a-f0-9]{64}$');
ALTER TABLE plugin_v2_snapshots DROP CONSTRAINT plugin_v2_snapshots_fingerprint_check;
ALTER TABLE plugin_v2_snapshots ADD CHECK(fingerprint ~ '^(content-v1:)?[a-f0-9]{64}$');
ALTER TABLE plugin_v2_state_snapshots DROP CONSTRAINT plugin_v2_state_snapshots_fingerprint_check;
ALTER TABLE plugin_v2_state_snapshots ADD CHECK(fingerprint ~ '^(content-v1:)?[a-f0-9]{64}$');
ALTER TABLE plugin_v2_state_tasks DROP CONSTRAINT q_state_fingerprint;
ALTER TABLE plugin_v2_state_tasks ADD CONSTRAINT q_state_fingerprint CHECK(fingerprint ~ '^(content-v1:)?[a-f0-9]{64}$');
-- +goose Down
DROP INDEX cloud_files_directory_idx;
DROP INDEX media_asset_bindings_parent_episode_idx;
-- Existing content identities remain namespaced; no credentials/state is rewritten.
