-- +goose Up
CREATE TABLE playback_grants (
 id uuid PRIMARY KEY,
 asset_id uuid NOT NULL UNIQUE REFERENCES media_assets(id) ON DELETE CASCADE,
 token_digest bytea NOT NULL,
 token_ciphertext bytea NOT NULL,
 revoked_at timestamptz,
 generation integer NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
DROP TABLE playback_grants;
