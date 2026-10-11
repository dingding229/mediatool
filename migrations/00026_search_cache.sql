-- +goose Up
ALTER TABLE search_runs ADD COLUMN cache_key text;
-- +goose Down
ALTER TABLE search_runs DROP COLUMN cache_key;
