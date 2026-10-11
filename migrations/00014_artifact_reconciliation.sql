-- +goose Up
ALTER TABLE managed_media_artifacts DROP CONSTRAINT managed_media_artifacts_artifact_kind_check;
ALTER TABLE managed_media_artifacts ADD CONSTRAINT managed_media_artifacts_artifact_kind_check CHECK(artifact_kind IN ('strm','nfo','subtitle'));
ALTER TABLE media_assets ADD COLUMN artifacts_state text NOT NULL DEFAULT 'ready' CHECK(artifacts_state IN ('ready','pending'));
-- +goose Down
DELETE FROM managed_media_artifacts WHERE artifact_kind='strm';
ALTER TABLE managed_media_artifacts DROP CONSTRAINT managed_media_artifacts_artifact_kind_check;
ALTER TABLE managed_media_artifacts ADD CONSTRAINT managed_media_artifacts_artifact_kind_check CHECK(artifact_kind IN ('nfo','subtitle'));
ALTER TABLE media_assets DROP COLUMN artifacts_state;
