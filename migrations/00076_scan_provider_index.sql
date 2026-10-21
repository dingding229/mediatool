-- +goose Up
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='120s';
CREATE INDEX cloud_files_storage_provider_idx ON cloud_files(storage_id,md5(provider_file_id)) WHERE provider_file_id IS NOT NULL AND provider_file_id<>'';
-- +goose Down
DROP INDEX cloud_files_storage_provider_idx;
