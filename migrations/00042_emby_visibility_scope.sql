-- +goose Up
ALTER TABLE library_emby_checks ADD COLUMN integration_revision bigint NOT NULL DEFAULT 0;
ALTER TABLE library_emby_checks ADD COLUMN user_id text NOT NULL DEFAULT '';
-- +goose Down
ALTER TABLE library_emby_checks DROP COLUMN user_id,DROP COLUMN integration_revision;
