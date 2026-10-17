-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
-- One completion credit per play session, independent of item played status.
-- A replay may increment again, while progress retries and Stopped cannot.
ALTER TABLE native_playback_sessions ADD COLUMN play_counted boolean NOT NULL DEFAULT false;

-- +goose Down
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
ALTER TABLE native_playback_sessions DROP COLUMN play_counted;
