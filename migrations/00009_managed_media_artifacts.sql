-- +goose Up
CREATE TABLE managed_media_artifacts (
 relative_path text PRIMARY KEY,
 asset_id uuid NOT NULL REFERENCES media_assets(id) ON DELETE CASCADE,
 artifact_kind text NOT NULL CHECK(artifact_kind IN ('nfo','subtitle')),
 content_digest bytea NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
DROP TABLE managed_media_artifacts;
