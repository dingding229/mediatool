-- +goose Up
ALTER TABLE integrations DROP CONSTRAINT integrations_kind_check;
ALTER TABLE integrations ADD CONSTRAINT integrations_kind_check CHECK(kind IN ('clouddrive2','open115','qbittorrent','emby','tmdb','telegram','ptsite'));
CREATE TABLE open115_authorizations (
 id uuid PRIMARY KEY,
 integration_id uuid NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
 owner_account_id uuid NOT NULL REFERENCES administrator_accounts(id) ON DELETE CASCADE,
 owner_version bigint NOT NULL,
 integration_revision bigint NOT NULL,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','authorized','expired','cancelled','failed')),
 payload_ciphertext bytea NOT NULL,
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX open115_auth_expiry ON open115_authorizations(expires_at);
-- +goose Down
DROP TABLE open115_authorizations;
-- An active official integration cannot survive a rollback; remove explicitly first.
ALTER TABLE integrations DROP CONSTRAINT integrations_kind_check;
ALTER TABLE integrations ADD CONSTRAINT integrations_kind_check CHECK(kind IN ('clouddrive2','qbittorrent','emby','tmdb','telegram','ptsite'));
