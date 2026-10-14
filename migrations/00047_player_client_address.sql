-- +goose Up
ALTER TABLE player_access_events ADD COLUMN connection_ip text NOT NULL DEFAULT '', ADD COLUMN ip_source text NOT NULL DEFAULT 'legacy_unknown', ADD COLUMN ip_public boolean NOT NULL DEFAULT false;
CREATE INDEX player_access_events_user ON player_access_events(emby_user_id,created_at DESC) WHERE emby_verified;
-- +goose Down
DROP INDEX player_access_events_user;
ALTER TABLE player_access_events DROP COLUMN connection_ip, DROP COLUMN ip_source, DROP COLUMN ip_public;
