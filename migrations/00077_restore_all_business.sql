-- +goose Up
ALTER TABLE ops_web_restores ADD COLUMN table_changes jsonb NOT NULL DEFAULT '[]';
-- +goose Down
ALTER TABLE ops_web_restores DROP COLUMN table_changes;
