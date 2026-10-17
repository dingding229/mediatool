-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
-- Stop concurrent account/grant edits between the legacy snapshot and DELETE.
-- These migration-only locks are transaction scoped, not playback-path locks.
LOCK TABLE media_users,media_user_library_grants,native_media_libraries IN ACCESS EXCLUSIVE MODE;
-- All native entities share one non-cycling sequence. Runtime code must retire
-- mappings, never delete/reassign them or restart this sequence (gaps are valid).
CREATE SEQUENCE native_item_seq AS bigint MINVALUE 100000 START WITH 100000 NO CYCLE;
CREATE TABLE native_item_ids (
 id bigint PRIMARY KEY DEFAULT nextval('native_item_seq') CHECK (id >= 100000),
 entity_type text NOT NULL CONSTRAINT native_item_ids_entity_type_check CHECK (entity_type IN ('library','movie','series','season','episode')),
 internal_key text NOT NULL CHECK (char_length(internal_key) BETWEEN 1 AND 512),
 retired_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (entity_type,internal_key),
 UNIQUE (id,entity_type)
);
ALTER SEQUENCE native_item_seq OWNED BY native_item_ids.id;
CREATE TABLE native_libraries (
 id text PRIMARY KEY CHECK (char_length(id) BETWEEN 3 AND 163),
 item_id bigint NOT NULL UNIQUE REFERENCES native_item_ids(id),
 item_entity_type text GENERATED ALWAYS AS ('library'::text) STORED NOT NULL,
 group_id text NOT NULL CHECK (group_id ~ '^[a-zA-Z0-9_-]{1,80}$'),
 rule_id text CHECK (rule_id ~ '^[a-zA-Z0-9_-]{1,80}$'),
 name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 256),
 directory text NOT NULL CHECK (char_length(directory) BETWEEN 1 AND 1024 AND left(directory,1) <> '/' AND right(directory,1) <> '/' AND directory !~ '(^|/)[.]{1,2}(/|$)' AND position('//' in directory)=0 AND position(chr(92) in directory)=0 AND directory !~ '[[:cntrl:]]'),
 collection_type text NOT NULL CONSTRAINT native_libraries_collection_type_check CHECK (collection_type IN ('movies','tvshows')),
 hidden boolean NOT NULL DEFAULT false,
 sort_order integer NOT NULL DEFAULT 0 CHECK (sort_order >= 0),
 policy_revision bigint NOT NULL DEFAULT 1 CHECK (policy_revision > 0),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK (id = CASE WHEN rule_id IS NULL THEN 'g:'||group_id ELSE 'r:'||group_id||':'||rule_id END),
 FOREIGN KEY (item_id,item_entity_type) REFERENCES native_item_ids(id,entity_type)
);
CREATE INDEX native_libraries_visible_order ON native_libraries(sort_order,id) WHERE NOT hidden;

-- Preserve both browse grants and can_play before removing the fixed libraries.
-- T11 must expand movie/series rows to ALL matching category libraries in the
-- SAME transaction as deleting those legacy rows. No grant expansion happens here.
CREATE TABLE media_user_legacy_grants (
 user_id uuid NOT NULL REFERENCES media_users(id) ON DELETE CASCADE,
 media_type text NOT NULL CONSTRAINT media_user_legacy_grants_media_type_check CHECK (media_type IN ('movie','series')),
 can_play boolean NOT NULL DEFAULT false,
 PRIMARY KEY (user_id,media_type)
);
INSERT INTO media_user_legacy_grants(user_id,media_type,can_play)
 SELECT g.user_id,l.media_type,g.can_play FROM media_user_library_grants g
 JOIN native_media_libraries l ON l.id=g.library_id;
DELETE FROM media_user_library_grants;
ALTER TABLE media_user_library_grants
 DROP CONSTRAINT media_user_library_grants_library_id_fkey,
 ADD COLUMN can_download boolean NOT NULL DEFAULT false,
 ADD CONSTRAINT media_user_library_grants_library_id_fkey FOREIGN KEY (library_id) REFERENCES native_libraries(id);
CREATE INDEX media_user_library_grants_library ON media_user_library_grants(library_id);
DROP TABLE native_media_libraries;

CREATE TABLE native_asset_libraries (
 asset_id uuid PRIMARY KEY REFERENCES media_assets(id) ON DELETE CASCADE,
 library_id text NOT NULL REFERENCES native_libraries(id),
 source text NOT NULL CONSTRAINT native_asset_libraries_source_check CHECK (source IN ('classification','output_directory')),
 policy_revision bigint NOT NULL CHECK (policy_revision > 0),
 asset_revision bigint NOT NULL CHECK (asset_revision > 0),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX native_asset_libraries_library ON native_asset_libraries(library_id,asset_id);
CREATE TABLE native_server_settings (
 singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
 server_id char(32) NOT NULL UNIQUE CHECK (server_id::text ~ '^[0-9a-f]{32}$'),
 server_name text NOT NULL DEFAULT 'Media Tool' CHECK (char_length(server_name) BETWEEN 1 AND 100 AND server_name !~ '[[:cntrl:]]'),
 version text NOT NULL DEFAULT '4.8.10.0' CHECK (char_length(version) BETWEEN 1 AND 32 AND version ~ '^[0-9]{1,4}([.][0-9]{1,4}){3}$'),
 enabled boolean NOT NULL DEFAULT false,
 public_user_list boolean NOT NULL DEFAULT false,
 revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO native_server_settings(server_id) VALUES (replace(gen_random_uuid()::text,'-',''));
ALTER TABLE media_users
 ADD COLUMN max_streams integer NOT NULL DEFAULT 2 CHECK (max_streams BETWEEN 1 AND 20),
 ADD COLUMN max_parental_rating integer CHECK (max_parental_rating BETWEEN 0 AND 100),
 ADD COLUMN expires_at timestamptz;
CREATE TABLE media_user_devices (
 id uuid PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES media_users(id) ON DELETE CASCADE,
 device_key text NOT NULL CHECK (char_length(device_key) BETWEEN 1 AND 256),
 name text NOT NULL DEFAULT '' CHECK (char_length(name) <= 256),
 client text NOT NULL DEFAULT '' CHECK (char_length(client) <= 128),
 app_version text NOT NULL DEFAULT '' CHECK (char_length(app_version) <= 64),
 last_seen_at timestamptz NOT NULL DEFAULT now(),
 last_ip inet,
 last_user_agent text NOT NULL DEFAULT '' CHECK (char_length(last_user_agent) <= 2048),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (user_id,device_key),
 UNIQUE (user_id,id)
);
CREATE INDEX media_user_devices_user_seen ON media_user_devices(user_id,last_seen_at DESC,id);
CREATE TABLE media_user_tokens (
 token_digest bytea PRIMARY KEY CHECK (octet_length(token_digest)=32),
 user_id uuid NOT NULL REFERENCES media_users(id) ON DELETE CASCADE,
 device_id uuid NOT NULL,
 credential_version bigint NOT NULL CHECK (credential_version > 0),
 created_at timestamptz NOT NULL DEFAULT now(),
 last_seen_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL,
 revoked_at timestamptz,
 CHECK (expires_at > created_at),
 FOREIGN KEY (user_id,device_id) REFERENCES media_user_devices(user_id,id) ON DELETE CASCADE
);
CREATE INDEX media_user_tokens_active_user ON media_user_tokens(user_id) WHERE revoked_at IS NULL;
CREATE INDEX media_user_tokens_device ON media_user_tokens(user_id,device_id);
CREATE INDEX media_user_tokens_expiry ON media_user_tokens(expires_at) WHERE revoked_at IS NULL;
CREATE TABLE media_user_item_data (
 user_id uuid NOT NULL REFERENCES media_users(id) ON DELETE CASCADE,
 item_id bigint NOT NULL REFERENCES native_item_ids(id),
 position_ticks bigint NOT NULL DEFAULT 0 CHECK (position_ticks >= 0),
 play_count integer NOT NULL DEFAULT 0 CHECK (play_count >= 0),
 played boolean NOT NULL DEFAULT false,
 favorite boolean NOT NULL DEFAULT false,
 last_played_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (user_id,item_id)
);
CREATE INDEX media_user_item_data_item ON media_user_item_data(item_id);
CREATE INDEX media_user_item_data_resume ON media_user_item_data(user_id,last_played_at DESC,item_id) WHERE position_ticks > 0 AND NOT played;
CREATE INDEX media_user_item_data_favorites ON media_user_item_data(user_id,item_id) WHERE favorite;
CREATE INDEX media_user_item_data_history ON media_user_item_data(user_id,last_played_at DESC,item_id) WHERE last_played_at IS NOT NULL;
CREATE TABLE media_probe_results (
 cloud_file_id uuid PRIMARY KEY REFERENCES cloud_files(id) ON DELETE CASCADE,
 fingerprint text NOT NULL CHECK (char_length(fingerprint) BETWEEN 1 AND 512),
 state text NOT NULL DEFAULT 'pending' CONSTRAINT media_probe_results_state_check CHECK (state IN ('pending','probing','ready','failed')),
 attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
 next_attempt_at timestamptz NOT NULL DEFAULT now(),
 probing_started_at timestamptz,
 result jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(result)='object' AND octet_length(result::text) <= 4194304),
 error text NOT NULL DEFAULT '' CHECK (char_length(error) <= 8192),
 probed_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((state='probing') = (probing_started_at IS NOT NULL)),
 CHECK (state <> 'ready' OR (probed_at IS NOT NULL AND result <> '{}'::jsonb))
);
CREATE INDEX media_probe_results_due ON media_probe_results(state,next_attempt_at);
CREATE INDEX media_probe_results_stale ON media_probe_results(probing_started_at) WHERE state='probing';
CREATE TABLE media_stills (
 item_id bigint PRIMARY KEY REFERENCES native_item_ids(id),
 item_entity_type text GENERATED ALWAYS AS ('episode'::text) STORED NOT NULL,
 state text NOT NULL DEFAULT 'pending' CONSTRAINT media_stills_state_check CHECK (state IN ('pending','grabbing','ready','failed','skipped')),
 grabbing_started_at timestamptz,
 attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
 next_attempt_at timestamptz NOT NULL DEFAULT now(),
 relative_path text NOT NULL DEFAULT '' CHECK (char_length(relative_path) <= 1024 AND position('..' in relative_path)=0 AND left(relative_path,1) <> '/' AND position(chr(92) in relative_path)=0 AND position(':' in relative_path)=0 AND relative_path !~ '[[:cntrl:]]'),
 digest text NOT NULL DEFAULT '' CHECK (digest='' OR digest ~ '^[0-9a-f]{64}$'),
 width integer NOT NULL DEFAULT 0 CHECK (width BETWEEN 0 AND 16384),
 height integer NOT NULL DEFAULT 0 CHECK (height BETWEEN 0 AND 16384),
 error text NOT NULL DEFAULT '' CHECK (char_length(error) <= 8192),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((state='grabbing') = (grabbing_started_at IS NOT NULL)),
 CHECK (state <> 'ready' OR (relative_path <> '' AND digest <> '' AND width > 0 AND height > 0)),
 FOREIGN KEY (item_id,item_entity_type) REFERENCES native_item_ids(id,entity_type)
);
CREATE INDEX media_stills_due ON media_stills(state,next_attempt_at);
CREATE INDEX media_stills_stale ON media_stills(grabbing_started_at) WHERE state='grabbing';
CREATE TABLE native_playback_sessions (
 id uuid PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES media_users(id) ON DELETE CASCADE,
 device_id uuid NOT NULL,
 item_id bigint NOT NULL REFERENCES native_item_ids(id),
 media_source_id text NOT NULL CHECK (char_length(media_source_id) BETWEEN 1 AND 512),
 state text NOT NULL DEFAULT 'playing' CONSTRAINT native_playback_sessions_state_check CHECK (state IN ('playing','paused','stopped')),
 started_at timestamptz NOT NULL DEFAULT now(),
 last_progress_at timestamptz NOT NULL DEFAULT now(),
 ended_at timestamptz,
 position_ticks bigint NOT NULL DEFAULT 0 CHECK (position_ticks >= 0),
 CHECK ((state='stopped') = (ended_at IS NOT NULL)),
 CHECK (last_progress_at >= started_at AND (ended_at IS NULL OR ended_at >= started_at)),
 FOREIGN KEY (user_id,device_id) REFERENCES media_user_devices(user_id,id) ON DELETE CASCADE
);
CREATE INDEX native_playback_sessions_active_user ON native_playback_sessions(user_id) WHERE state <> 'stopped';
CREATE INDEX native_playback_sessions_stale ON native_playback_sessions(last_progress_at) WHERE state <> 'stopped';
CREATE INDEX native_playback_sessions_device ON native_playback_sessions(user_id,device_id);
CREATE INDEX native_playback_sessions_item ON native_playback_sessions(item_id);

-- +goose Down
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
LOCK TABLE media_users,media_user_library_grants,media_user_legacy_grants IN ACCESS EXCLUSIVE MODE;
-- Structural rollback drops the new catalog/user-data/cache/session tables.
-- Unexpanded legacy grants can be restored exactly. Grants already expanded or
-- newly edited against category libraries CANNOT be mapped back losslessly;
-- their rows are removed. Back up before rollback; never infer broader grants.
CREATE TABLE native_media_libraries (
 id text PRIMARY KEY,
 name text NOT NULL,
 media_type text NOT NULL UNIQUE CHECK (media_type IN ('movie','series'))
);
INSERT INTO native_media_libraries(id,name,media_type) VALUES ('movies','电影','movie'),('series','剧集','series');
DELETE FROM media_user_library_grants;
ALTER TABLE media_user_library_grants DROP CONSTRAINT media_user_library_grants_library_id_fkey;
DROP INDEX media_user_library_grants_library;
ALTER TABLE media_user_library_grants DROP COLUMN can_download,
 ADD CONSTRAINT media_user_library_grants_library_id_fkey FOREIGN KEY (library_id) REFERENCES native_media_libraries(id);
INSERT INTO media_user_library_grants(user_id,library_id,can_play)
 SELECT user_id,CASE media_type WHEN 'movie' THEN 'movies' ELSE 'series' END,can_play FROM media_user_legacy_grants;
DROP TABLE native_playback_sessions,media_stills,media_probe_results,media_user_item_data,media_user_tokens,media_user_devices,native_asset_libraries,native_server_settings,media_user_legacy_grants,native_libraries;
-- Dropping the ID table also drops its owned sequence.
DROP TABLE native_item_ids;
ALTER TABLE media_users DROP COLUMN max_streams,DROP COLUMN max_parental_rating,DROP COLUMN expires_at;
