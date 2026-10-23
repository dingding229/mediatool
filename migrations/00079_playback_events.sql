-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
-- Durable per-play facts, not the per-item completion/resume projection.
CREATE TABLE media_playback_events (
 id uuid PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES media_users(id) ON DELETE CASCADE,
 device_id uuid NOT NULL,
 item_id bigint NOT NULL,
 series_id text NOT NULL DEFAULT '', season_id text NOT NULL DEFAULT '',
 media_type text NOT NULL CHECK(media_type IN ('movie','episode')),
 name text NOT NULL, episode_title text NOT NULL DEFAULT '',
 season integer, episode integer,
 client text NOT NULL DEFAULT '', user_agent text NOT NULL DEFAULT '',
 started_at timestamptz NOT NULL, last_seen_at timestamptz NOT NULL, ended_at timestamptz,
 state text NOT NULL CHECK(state IN ('playing','paused','stopped')),
 duration_seconds double precision NOT NULL DEFAULT 0 CHECK(duration_seconds>=0),
 position_ticks bigint NOT NULL DEFAULT 0 CHECK(position_ticks>=0),
 source_ip inet, redirect_count bigint NOT NULL DEFAULT 0,
 FOREIGN KEY(user_id,device_id) REFERENCES media_user_devices(user_id,id) ON DELETE CASCADE
);
CREATE INDEX media_playback_events_user_time ON media_playback_events(user_id,started_at DESC,id);
CREATE INDEX media_playback_events_time ON media_playback_events(started_at DESC,id);
CREATE INDEX media_playback_events_device ON media_playback_events(device_id,started_at DESC);
CREATE TABLE media_playback_ips (
 event_id uuid NOT NULL REFERENCES media_playback_events(id) ON DELETE CASCADE,
 ip inet NOT NULL, first_seen_at timestamptz NOT NULL, last_seen_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,ip)
);
CREATE INDEX media_playback_ips_ip ON media_playback_ips(ip,last_seen_at DESC);
CREATE TABLE media_device_ips (
 user_id uuid NOT NULL REFERENCES media_users(id) ON DELETE CASCADE,
 device_id uuid NOT NULL, ip inet NOT NULL,
 first_seen_at timestamptz NOT NULL, last_seen_at timestamptz NOT NULL,
 request_count bigint NOT NULL DEFAULT 1,
 PRIMARY KEY(device_id,ip),
 FOREIGN KEY(user_id,device_id) REFERENCES media_user_devices(user_id,id) ON DELETE CASCADE
);
CREATE INDEX media_device_ips_seen ON media_device_ips(user_id,last_seen_at DESC);

UPDATE application_settings SET value=jsonb_set(value,'{playbackRetentionDays}','180'::jsonb) WHERE NOT value ? 'playbackRetentionDays';
-- Runtime CZDB credentials use the existing master-key vault. No database is distributed.
CREATE TABLE ip_geo_settings(id text PRIMARY KEY DEFAULT 'cz88-community' CHECK(id='cz88-community'),singleton boolean UNIQUE NOT NULL DEFAULT true CHECK(singleton), encrypted_credentials bytea NOT NULL, updated_at timestamptz NOT NULL DEFAULT now());
-- +goose Down
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
DROP TABLE IF EXISTS ip_geo_settings,media_device_ips,media_playback_ips,media_playback_events;
UPDATE application_settings SET value=value-'playbackRetentionDays';
