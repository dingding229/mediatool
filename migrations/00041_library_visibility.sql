-- +goose Up
CREATE TABLE library_emby_checks (
 asset_id uuid PRIMARY KEY REFERENCES media_assets(id) ON DELETE CASCADE,
 integration_id uuid NOT NULL REFERENCES integrations(id),
 asset_revision bigint NOT NULL,
 state text NOT NULL,
 message text NOT NULL,
 paths jsonb NOT NULL,
 checked_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
DROP TABLE library_emby_checks;
