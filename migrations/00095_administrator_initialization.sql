-- +goose Up
CREATE TABLE administrator_initialization (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    token_digest bytea NOT NULL CHECK (octet_length(token_digest) = 32),
    expires_at timestamptz NOT NULL
);

-- +goose Down
DROP TABLE administrator_initialization;
