-- +goose Up
CREATE TABLE administrator_sessions (
 token_digest bytea PRIMARY KEY,
 expires_at timestamptz NOT NULL,
 revoked_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX administrator_sessions_expiry ON administrator_sessions(expires_at);
-- +goose Down
DROP TABLE administrator_sessions;
