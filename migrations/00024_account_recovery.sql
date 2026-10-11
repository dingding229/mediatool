-- +goose Up
CREATE TABLE administrator_recovery_codes (
 account_id uuid PRIMARY KEY REFERENCES administrator_accounts(id) ON DELETE CASCADE,
 code_digest bytea NOT NULL UNIQUE CHECK(octet_length(code_digest)=32),
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
DROP TABLE administrator_recovery_codes;
