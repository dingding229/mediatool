-- +goose Up
-- Existing definitions keep their original bytes for rollback but are not
-- executed: only an explicit native-repository import assigns this source.
ALTER TABLE site_definitions ADD COLUMN source_repository text NOT NULL DEFAULT '';
-- +goose Down
ALTER TABLE site_definitions DROP COLUMN source_repository;
