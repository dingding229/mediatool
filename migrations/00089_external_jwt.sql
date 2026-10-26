-- +goose Up
LOCK TABLE external_api_keys, external_api_calls IN ACCESS EXCLUSIVE MODE;
ALTER TABLE external_api_calls ADD COLUMN credential_name text NOT NULL DEFAULT '';
-- Call history is an immutable observation, not an active authorization reference.
ALTER TABLE external_api_calls DROP CONSTRAINT external_api_calls_key_id_fkey;
UPDATE external_api_calls c SET credential_name=k.name FROM external_api_keys k WHERE c.key_id=k.id;
CREATE TABLE external_api_migration_receipt(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),legacy_count bigint NOT NULL,migrated_at timestamptz NOT NULL DEFAULT now());
INSERT INTO external_api_migration_receipt(singleton,legacy_count)SELECT true,count(*)FROM external_api_keys;
DELETE FROM external_api_keys;
ALTER TABLE external_api_keys DROP CONSTRAINT external_api_keys_permanent;
ALTER TABLE external_api_keys DROP COLUMN token_digest,DROP COLUMN token_ciphertext,DROP COLUMN prefix,DROP COLUMN expires_at;
ALTER TABLE external_api_keys ADD COLUMN last_ip inet,ADD COLUMN last_ip_source text NOT NULL DEFAULT '',ADD COLUMN last_ip_at timestamptz;
ALTER TABLE external_api_calls ADD COLUMN client_ip inet,ADD COLUMN ip_source text NOT NULL DEFAULT '';
CREATE INDEX external_api_calls_key_recent ON external_api_calls(key_id,created_at DESC,id DESC);
-- +goose Down
DROP TABLE external_api_migration_receipt;
-- Structural downgrade only: never reconstruct obsolete credentials from JWTs.
DELETE FROM external_api_calls WHERE key_id IN(SELECT id FROM external_api_keys);
DELETE FROM external_api_keys;
DROP INDEX external_api_calls_key_recent;
ALTER TABLE external_api_calls DROP COLUMN client_ip,DROP COLUMN ip_source,DROP COLUMN credential_name;
UPDATE external_api_calls SET key_id=NULL;
ALTER TABLE external_api_calls ADD CONSTRAINT external_api_calls_key_id_fkey FOREIGN KEY(key_id)REFERENCES external_api_keys(id);
ALTER TABLE external_api_keys DROP COLUMN last_ip,DROP COLUMN last_ip_source,DROP COLUMN last_ip_at;
ALTER TABLE external_api_keys ADD COLUMN token_digest bytea NOT NULL UNIQUE,ADD COLUMN token_ciphertext bytea NOT NULL,ADD COLUMN prefix text NOT NULL,ADD COLUMN expires_at timestamptz;
ALTER TABLE external_api_keys ADD CONSTRAINT external_api_keys_permanent CHECK(expires_at IS NULL);
