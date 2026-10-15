-- +goose Up
ALTER TABLE external_api_keys ALTER COLUMN expires_at DROP NOT NULL;
UPDATE external_api_keys SET expires_at=NULL;
ALTER TABLE external_api_keys ADD CONSTRAINT external_api_keys_permanent CHECK (expires_at IS NULL);

-- +goose Down
-- Tokens remain permanent on rollback; never silently assign an expiration.
ALTER TABLE external_api_keys DROP CONSTRAINT external_api_keys_permanent;
