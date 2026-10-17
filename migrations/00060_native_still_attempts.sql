-- +goose Up
-- Independent still attempts; jobs only wake the row scheduler. No file I/O.
ALTER TABLE native_server_settings ADD COLUMN stills_enabled boolean NOT NULL DEFAULT true;
ALTER TABLE media_stills DROP CONSTRAINT media_stills_state_check,
 ADD CONSTRAINT media_stills_state_check CHECK(state IN('pending','grabbing','ready','failed','skipped','needs_action')),
 ADD COLUMN asset_id uuid REFERENCES media_assets(id) ON DELETE SET NULL,
 ADD COLUMN fingerprint text NOT NULL DEFAULT '' CHECK(fingerprint='' OR fingerprint~'^[0-9a-f]{64}$'),
 ADD COLUMN duration_ticks bigint NOT NULL DEFAULT 0 CHECK(duration_ticks>=0),
 ADD COLUMN result_asset_id uuid REFERENCES media_assets(id) ON DELETE SET NULL,
 ADD COLUMN result_fingerprint text CHECK(result_fingerprint~'^[0-9a-f]{64}$'),
 ADD COLUMN published_at timestamptz,
 ADD COLUMN last_access_at timestamptz,
 -- Unknown legacy sizes remain 0 until GC calibration; admission fails closed.
 ADD COLUMN size_bytes bigint NOT NULL DEFAULT 0 CHECK(size_bytes BETWEEN 0 AND 4194304),
 ADD COLUMN reserved_bytes bigint NOT NULL DEFAULT 0 CHECK(reserved_bytes BETWEEN 0 AND 4194304),
 ADD COLUMN evicted_at timestamptz,
 ADD COLUMN lease_until timestamptz,
 ADD COLUMN lease_token uuid,
 ADD COLUMN last_error_class text CONSTRAINT media_stills_error_class_check CHECK(last_error_class IN('permanent','retryable','manual')),
 ADD COLUMN last_error_reason text CONSTRAINT media_stills_error_reason_check CHECK(last_error_reason IN('not_exposable','file_missing','strm_mismatch','authorization','rate_limited','timeout','unavailable','unsafe_target','ua_mismatch','ticket_unavailable','lease_expired','process_failed','invalid_image','tmdb_still','source_unavailable','storage_unavailable','storage_full','cache_evicted'));
-- Preserve complete legacy cache metadata, but do NOT invent its asset/fingerprint.
-- Normalize dirty legacy rows before adding new checks. All old in-flight/ready
-- rows need a fresh candidate; complete old images remain readable meanwhile.
WITH checked AS (SELECT item_id,
 relative_path<>'' AND char_length(relative_path)<=1024 AND position('..' in relative_path)=0 AND left(relative_path,1)<>'/'
 AND position(chr(92) in relative_path)=0 AND position(':' in relative_path)=0 AND relative_path !~ '[[:cntrl:]]'
 AND digest~'^[0-9a-f]{64}$' AND width BETWEEN 1 AND 16384 AND height BETWEEN 1 AND 16384 AS usable FROM media_stills)
UPDATE media_stills s SET
 published_at=CASE WHEN c.usable THEN s.updated_at END,last_access_at=CASE WHEN c.usable THEN s.updated_at END,
 relative_path=CASE WHEN c.usable THEN s.relative_path ELSE '' END,digest=CASE WHEN c.usable THEN s.digest ELSE '' END,
 width=CASE WHEN c.usable THEN s.width ELSE 0 END,height=CASE WHEN c.usable THEN s.height ELSE 0 END,
 state=CASE WHEN s.state IN('grabbing','ready') THEN 'pending' ELSE s.state END,attempts=greatest(s.attempts,0),grabbing_started_at=NULL,
 next_attempt_at=CASE WHEN s.state IN('grabbing','ready') THEN now() ELSE s.next_attempt_at END,
 last_error_class=CASE WHEN s.state='grabbing' THEN 'retryable' WHEN s.state IN('failed','skipped') THEN 'permanent' END,
 last_error_reason=CASE WHEN s.state='grabbing' THEN 'lease_expired' WHEN s.state='failed' THEN 'process_failed' WHEN s.state='skipped' THEN 'source_unavailable' END
 FROM checked c WHERE s.item_id=c.item_id;
ALTER TABLE media_stills DROP COLUMN error,
 ADD CONSTRAINT media_stills_error_pair_check CHECK((last_error_class IS NULL)=(last_error_reason IS NULL)),
 ADD CONSTRAINT media_stills_token_check CHECK((state='grabbing')=(lease_token IS NOT NULL)),
 ADD CONSTRAINT media_stills_lease_check CHECK((state='grabbing')=(lease_until IS NOT NULL)),
 ADD CONSTRAINT media_stills_lease_time_check CHECK(lease_until>grabbing_started_at AND lease_until<=grabbing_started_at+interval '5 minutes'),
 ADD CONSTRAINT media_stills_attempt_source_check CHECK(state<>'grabbing' OR (fingerprint<>'' AND duration_ticks>0)),
 ADD CONSTRAINT media_stills_source_check CHECK(asset_id IS NULL OR (fingerprint<>'' AND duration_ticks>0)),
 ADD CONSTRAINT media_stills_snapshot_check CHECK(
 (published_at IS NULL AND relative_path='' AND digest='' AND width=0 AND height=0 AND result_asset_id IS NULL AND result_fingerprint IS NULL AND last_access_at IS NULL AND size_bytes=0)
 OR (published_at IS NOT NULL AND relative_path<>'' AND digest<>'' AND width>0 AND height>0 AND last_access_at IS NOT NULL AND last_access_at>=published_at)),
 ADD CONSTRAINT media_stills_result_source_check CHECK(result_asset_id IS NULL OR (published_at IS NOT NULL AND result_fingerprint IS NOT NULL)),
 ADD CONSTRAINT media_stills_result_time_check CHECK(result_fingerprint IS NULL OR published_at IS NOT NULL),
 ADD CONSTRAINT media_stills_result_path_check CHECK(result_asset_id IS NULL OR relative_path=left(result_asset_id::text,2)||'/'||result_asset_id::text||'-'||left(result_fingerprint,16)||'.jpg'),
 ADD CONSTRAINT media_stills_result_width_check CHECK(result_fingerprint IS NULL OR width<=1280),
 -- Nullable FK tombstones must not block asset deletion/cleanup.
 ADD CONSTRAINT media_stills_ready_check CHECK(state<>'ready' OR (published_at IS NOT NULL AND result_fingerprint IS NOT NULL AND result_fingerprint=fingerprint AND last_error_class IS NULL AND (asset_id IS NULL OR result_asset_id IS NULL OR asset_id=result_asset_id))),
 ADD CONSTRAINT media_stills_retry_check CHECK(state NOT IN('pending','grabbing') OR last_error_class IS NULL OR last_error_class='retryable'),
 ADD CONSTRAINT media_stills_failed_check CHECK(state<>'failed' OR last_error_class='permanent' AND last_error_class IS NOT NULL),
 ADD CONSTRAINT media_stills_manual_check CHECK(state<>'needs_action' OR last_error_class='manual' AND last_error_class IS NOT NULL),
 ADD CONSTRAINT media_stills_skipped_check CHECK(state<>'skipped' OR last_error_class='permanent' AND last_error_class IS NOT NULL AND last_error_reason IN('tmdb_still','source_unavailable','cache_evicted')),
 ADD CONSTRAINT media_stills_reservation_check CHECK(reserved_bytes=0 OR state='grabbing'),
 ADD CONSTRAINT media_stills_eviction_check CHECK(last_error_reason IS DISTINCT FROM 'cache_evicted' OR (state='skipped' AND evicted_at IS NOT NULL AND published_at IS NULL));
CREATE INDEX media_stills_lease ON media_stills(lease_until) WHERE state='grabbing';
CREATE INDEX media_stills_access ON media_stills(last_access_at,item_id) WHERE published_at IS NOT NULL;
CREATE INDEX media_stills_path ON media_stills(relative_path) WHERE published_at IS NOT NULL;
CREATE INDEX media_stills_unknown_size ON media_stills(item_id) WHERE published_at IS NOT NULL AND size_bytes=0;
CREATE INDEX media_stills_cache_size ON media_stills(item_id) INCLUDE(size_bytes) WHERE published_at IS NOT NULL;
CREATE INDEX media_stills_reservations ON media_stills(lease_until) INCLUDE(reserved_bytes) WHERE reserved_bytes>0;
CREATE INDEX media_stills_evicted ON media_stills(evicted_at,item_id) WHERE last_error_reason='cache_evicted';
CREATE INDEX media_stills_asset ON media_stills(asset_id) WHERE asset_id IS NOT NULL;
CREATE INDEX media_stills_result_asset ON media_stills(result_asset_id) WHERE result_asset_id IS NOT NULL;

-- +goose Down
-- Cache metadata survives; provenance, lease/fence, access times and fixed error
-- diagnostics cannot be restored in schema 59. Files are never moved/deleted.
DROP INDEX media_stills_path,media_stills_unknown_size,media_stills_cache_size,media_stills_reservations,media_stills_evicted,media_stills_lease,media_stills_access,media_stills_asset,media_stills_result_asset;
ALTER TABLE media_stills DROP CONSTRAINT media_stills_error_pair_check,DROP CONSTRAINT media_stills_token_check,
 DROP CONSTRAINT media_stills_lease_check,DROP CONSTRAINT media_stills_lease_time_check,DROP CONSTRAINT media_stills_attempt_source_check,
 DROP CONSTRAINT media_stills_source_check,DROP CONSTRAINT media_stills_snapshot_check,DROP CONSTRAINT media_stills_result_source_check,
 DROP CONSTRAINT media_stills_result_time_check,DROP CONSTRAINT media_stills_result_path_check,DROP CONSTRAINT media_stills_result_width_check,
 DROP CONSTRAINT media_stills_ready_check,DROP CONSTRAINT media_stills_retry_check,DROP CONSTRAINT media_stills_failed_check,
 DROP CONSTRAINT media_stills_reservation_check,DROP CONSTRAINT media_stills_eviction_check,DROP CONSTRAINT media_stills_manual_check,DROP CONSTRAINT media_stills_skipped_check,DROP CONSTRAINT media_stills_state_check;
UPDATE media_stills SET state=CASE WHEN published_at IS NOT NULL THEN 'ready' WHEN state='needs_action' THEN 'failed' WHEN state='grabbing' THEN 'pending' ELSE state END,grabbing_started_at=NULL;
ALTER TABLE media_stills ADD CONSTRAINT media_stills_state_check CHECK(state IN('pending','grabbing','ready','failed','skipped')),
 ADD COLUMN error text NOT NULL DEFAULT '' CHECK(char_length(error)<=8192),
 DROP COLUMN asset_id,DROP COLUMN fingerprint,DROP COLUMN duration_ticks,DROP COLUMN result_asset_id,DROP COLUMN result_fingerprint,
 DROP COLUMN size_bytes,DROP COLUMN reserved_bytes,DROP COLUMN evicted_at,
 DROP COLUMN published_at,DROP COLUMN last_access_at,DROP COLUMN lease_until,DROP COLUMN lease_token,DROP COLUMN last_error_class,DROP COLUMN last_error_reason;
ALTER TABLE native_server_settings DROP COLUMN stills_enabled;
