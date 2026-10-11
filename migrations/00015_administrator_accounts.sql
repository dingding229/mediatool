-- +goose Up
CREATE TABLE administrator_accounts (
 id uuid PRIMARY KEY,
 singleton boolean NOT NULL DEFAULT true UNIQUE CHECK(singleton),
 username text NOT NULL UNIQUE CHECK(username ~ '^[a-z0-9][a-z0-9_.-]{2,31}$'),
 password_hash text NOT NULL,
 credential_version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
-- Old startup-key sessions have no account identity and must not survive.
DELETE FROM administrator_sessions;
ALTER TABLE administrator_sessions ADD COLUMN account_id uuid NOT NULL REFERENCES administrator_accounts(id) ON DELETE CASCADE;
ALTER TABLE administrator_sessions ADD COLUMN credential_version bigint NOT NULL;
CREATE INDEX administrator_sessions_account ON administrator_sessions(account_id);
CREATE TABLE administrator_auth_limits (
 scope_digest bytea PRIMARY KEY,
 window_start timestamptz NOT NULL,
 attempts integer NOT NULL
);
-- +goose Down
DELETE FROM administrator_sessions;
ALTER TABLE administrator_sessions DROP COLUMN account_id,DROP COLUMN credential_version;
DROP TABLE administrator_auth_limits;
DROP TABLE administrator_accounts;
