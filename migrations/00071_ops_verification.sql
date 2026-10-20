-- +goose Up
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='60s';
CREATE TABLE ops_request_verifications(
 request_id uuid PRIMARY KEY REFERENCES ops_requests(id) ON DELETE CASCADE,
 request_digest text NOT NULL CHECK(request_digest ~ '^[a-f0-9]{64}$'),
 schema_version bigint NOT NULL CHECK(schema_version>0),
 revision text NOT NULL CHECK(revision ~ '^[a-f0-9]{40}$'),
 protected_digest text NOT NULL CHECK(protected_digest ~ '^[a-f0-9]{64}$'),
 verified_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN IF EXISTS(SELECT FROM ops_request_verifications) THEN RAISE EXCEPTION 'verification receipts exist; restore backup rather than erase approvals'; END IF; END $$;
-- +goose StatementEnd
DROP TABLE ops_request_verifications;
