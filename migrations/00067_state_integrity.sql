-- +goose Up
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='10min';
-- NOT VALID fences new writes before the historical scan; invalid old data
-- aborts the migration, never normalize ciphertext, identity or authorization.
ALTER TABLE plugin_v2_state_tasks ADD CONSTRAINT q_state_revision CHECK(revision>0) NOT VALID;
ALTER TABLE plugin_v2_state_tasks ADD CONSTRAINT q_state_fingerprint CHECK(fingerprint ~ '^[a-f0-9]{64}$') NOT VALID;
ALTER TABLE plugin_v2_state_tasks ADD CONSTRAINT q_state_owner FOREIGN KEY(snapshot_id,instance_id) REFERENCES plugin_v2_state_snapshots(id,instance_id) NOT VALID;
ALTER TABLE plugin_v2_state_tasks ADD CONSTRAINT q_state_result CHECK(jsonb_typeof(result)='object') NOT VALID;
ALTER TABLE search_runs ADD CONSTRAINT q_search_accounts CHECK(jsonb_typeof(account_ids)='array') NOT VALID;
ALTER TABLE subscriptions ADD CONSTRAINT q_subscription_json CHECK(jsonb_typeof(identity)='object' AND jsonb_typeof(rules)='object' AND jsonb_typeof(coverage)='object') NOT VALID;
ALTER TABLE download_tasks ADD CONSTRAINT q_download_details CHECK(jsonb_typeof(details)='object') NOT VALID;
ALTER TABLE plugin_v2_state_tasks VALIDATE CONSTRAINT q_state_revision;
ALTER TABLE plugin_v2_state_tasks VALIDATE CONSTRAINT q_state_fingerprint;
ALTER TABLE plugin_v2_state_tasks VALIDATE CONSTRAINT q_state_owner;
ALTER TABLE plugin_v2_state_tasks VALIDATE CONSTRAINT q_state_result;
ALTER TABLE search_runs VALIDATE CONSTRAINT q_search_accounts;
ALTER TABLE subscriptions VALIDATE CONSTRAINT q_subscription_json;
ALTER TABLE download_tasks VALIDATE CONSTRAINT q_download_details;
-- +goose Down
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='60s';
ALTER TABLE download_tasks DROP CONSTRAINT q_download_details;
ALTER TABLE subscriptions DROP CONSTRAINT q_subscription_json;
ALTER TABLE search_runs DROP CONSTRAINT q_search_accounts;
ALTER TABLE plugin_v2_state_tasks DROP CONSTRAINT q_state_result, DROP CONSTRAINT q_state_owner, DROP CONSTRAINT q_state_fingerprint, DROP CONSTRAINT q_state_revision;
