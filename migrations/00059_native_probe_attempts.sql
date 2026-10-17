-- +goose Up
-- Attempt scheduling lives ONLY in this table; jobs are disposable wake-ups.
ALTER TABLE media_probe_results RENAME COLUMN fingerprint TO file_fingerprint;
ALTER TABLE media_probe_results
 DROP CONSTRAINT media_probe_results_state_check,
 ADD CONSTRAINT media_probe_results_state_check CHECK(state IN('pending','probing','ready','failed','needs_action')),
 ADD COLUMN result_fingerprint text CHECK(char_length(result_fingerprint) BETWEEN 1 AND 512),
 ADD COLUMN last_error_class text CONSTRAINT media_probe_results_error_class_check CHECK(last_error_class IN('permanent','retryable','manual')),
 ADD COLUMN last_error_reason text CONSTRAINT media_probe_results_error_reason_check CHECK(last_error_reason IN('not_exposable','file_missing','strm_mismatch','authorization','rate_limited','timeout','unavailable','unsafe_target','ua_mismatch','ticket_unavailable','lease_expired','process_failed','invalid_result')),
 ADD COLUMN lease_until timestamptz,
 ADD COLUMN lease_token uuid;
-- Normalize incomplete/dirty legacy receipts BEFORE any new CHECK is added.
-- Failed/pending rows may also carry partial receipts or stale start timestamps.
UPDATE media_probe_results SET
 state=CASE WHEN state='probing' OR (state='ready' AND (result='{}'::jsonb OR probed_at IS NULL)) THEN 'pending' ELSE state END,
 next_attempt_at=CASE WHEN state='probing' OR (state='ready' AND (result='{}'::jsonb OR probed_at IS NULL)) THEN now() ELSE next_attempt_at END,
 probing_started_at=NULL,
 last_error_class=CASE WHEN state='probing' THEN 'retryable' WHEN state='failed' THEN 'permanent' END,
 last_error_reason=CASE WHEN state='probing' THEN 'lease_expired' WHEN state='failed' THEN 'process_failed' END;
UPDATE media_probe_results SET result_fingerprint=file_fingerprint WHERE probed_at IS NOT NULL AND result<>'{}'::jsonb;
UPDATE media_probe_results SET result='{}',probed_at=NULL WHERE result_fingerprint IS NULL;
-- No free-form upstream error text is retained or accepted after this migration.
ALTER TABLE media_probe_results DROP COLUMN error,
 ADD CONSTRAINT media_probe_results_error_pair_check CHECK((last_error_class IS NULL)=(last_error_reason IS NULL)),
 ADD CONSTRAINT media_probe_results_lease_until_check CHECK((state='probing')=(lease_until IS NOT NULL)),
 ADD CONSTRAINT media_probe_results_lease_token_check CHECK((state='probing')=(lease_token IS NOT NULL)),
 ADD CONSTRAINT media_probe_results_lease_time_check CHECK(lease_until>probing_started_at),
 ADD CONSTRAINT media_probe_results_snapshot_time_check CHECK((result_fingerprint IS NULL)=(probed_at IS NULL)),
 ADD CONSTRAINT media_probe_results_snapshot_result_check CHECK((result_fingerprint IS NULL)=(result='{}'::jsonb)),
 ADD CONSTRAINT media_probe_results_ready_snapshot_check CHECK(state<>'ready' OR (result_fingerprint=file_fingerprint AND last_error_class IS NULL)),
 ADD CONSTRAINT media_probe_results_manual_check CHECK(state<>'needs_action' OR (last_error_class IS NOT NULL AND last_error_class='manual')),
 ADD CONSTRAINT media_probe_results_failed_check CHECK(state<>'failed' OR (last_error_class IS NOT NULL AND last_error_class='permanent')),
 ADD CONSTRAINT media_probe_results_retry_check CHECK(state NOT IN('pending','probing') OR last_error_class IS NULL OR last_error_class='retryable');
ALTER TABLE native_server_settings ADD COLUMN probe_enabled boolean NOT NULL DEFAULT true;
CREATE INDEX media_probe_results_lease ON media_probe_results(lease_until) WHERE state='probing';
-- cloud_files.metadata is the persisted hash map (key "2" = SHA1); do NOT use
-- updated_at, which changes on unchanged scans. Optional mtime is used if present.
CREATE FUNCTION native_probe_fingerprint(bigint,text,jsonb) RETURNS text
 LANGUAGE sql IMMUTABLE PARALLEL SAFE RETURN encode(sha256(convert_to(jsonb_build_array(
 $1,coalesce($2,''),lower(coalesce($3->>'2',$3->>'sha1','')),
 lower(coalesce($3->>'1',$3->>'md5','')),coalesce($3->>'mtime',$3->>'modified_at',$3->>'modifiedAt',''))::text,'UTF8')),'hex');

-- +goose Down
-- Cache rollback preserves the last successful snapshot, but cannot preserve
-- the new attempt diagnostics/lease ownership. Re-enqueue after upgrading again.
ALTER TABLE native_server_settings DROP COLUMN probe_enabled;
DROP FUNCTION native_probe_fingerprint(bigint,text,jsonb);
DROP INDEX media_probe_results_lease;
-- Remove only the checks introduced here; the 00055 checks remain intact.
ALTER TABLE media_probe_results
 DROP CONSTRAINT media_probe_results_error_pair_check,DROP CONSTRAINT media_probe_results_lease_until_check,
 DROP CONSTRAINT media_probe_results_lease_token_check,DROP CONSTRAINT media_probe_results_lease_time_check,
 DROP CONSTRAINT media_probe_results_snapshot_time_check,DROP CONSTRAINT media_probe_results_snapshot_result_check,
 DROP CONSTRAINT media_probe_results_ready_snapshot_check,DROP CONSTRAINT media_probe_results_manual_check,
 DROP CONSTRAINT media_probe_results_failed_check,DROP CONSTRAINT media_probe_results_retry_check;
UPDATE media_probe_results SET file_fingerprint=coalesce(result_fingerprint,file_fingerprint),
 state=CASE WHEN state='needs_action' THEN 'failed' WHEN state='probing' THEN 'pending' ELSE state END,
 probing_started_at=NULL;
ALTER TABLE media_probe_results
 DROP CONSTRAINT media_probe_results_state_check,
 ADD CONSTRAINT media_probe_results_state_check CHECK(state IN('pending','probing','ready','failed')),
 DROP COLUMN result_fingerprint,DROP COLUMN last_error_class,DROP COLUMN last_error_reason,
 DROP COLUMN lease_until,DROP COLUMN lease_token,
 ADD COLUMN error text NOT NULL DEFAULT '' CHECK(char_length(error)<=8192);
ALTER TABLE media_probe_results RENAME COLUMN file_fingerprint TO fingerprint;
