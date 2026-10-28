-- +goose Up
-- Remove management/physical cleanup only. Keep the journal, its archive evidence,
-- all active output/repair records, and every .media-tool-retired disk file intact.
SET LOCAL lock_timeout='5s';
LOCK TABLE media_archive_cleanup_operations,media_archive_cleanup_requests,media_archive_policy IN ACCESS EXCLUSIVE MODE;
LOCK TABLE jobs IN SHARE MODE;
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM media_archive_cleanup_operations WHERE state NOT IN('removed','cancelled')) THEN
  RAISE EXCEPTION 'unfinished archive cleanup approval; reconcile before removing management';
 END IF;
 IF EXISTS(SELECT 1 FROM jobs WHERE kind='library.archive.cleanup' AND state NOT IN('succeeded','failed','cancelled')) THEN
  RAISE EXCEPTION 'non-terminal archive cleanup job; reconcile before removing management';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE media_archive_cleanup_operations;
DROP TABLE media_archive_cleanup_requests;
DROP TABLE media_archive_policy;

-- +goose Down
-- Structure/default-policy recovery ONLY. Deleted policy values, approvals and
-- receipts are not reconstructible: use a verified pre-upgrade DB backup for data.
-- Down never modifies archive files, media, STRM bodies or the retained journal.
CREATE TABLE media_archive_policy (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 retention_days integer NOT NULL DEFAULT 30 CHECK(retention_days BETWEEN 1 AND 3650),
 revision bigint NOT NULL DEFAULT 1,
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO media_archive_policy(singleton)VALUES(true);
CREATE TABLE media_archive_cleanup_requests (
 id uuid PRIMARY KEY,
 plan_digest text NOT NULL CHECK(length(plan_digest)=64),
 archive_ids uuid[] NOT NULL,
 policy_revision bigint NOT NULL,
 retention_days integer NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE media_archive_cleanup_operations (
 id uuid PRIMARY KEY,
 request_id uuid NOT NULL REFERENCES media_archive_cleanup_requests(id),
 journal_id uuid NOT NULL UNIQUE REFERENCES media_artifact_journal(id),
 expected_digest bytea NOT NULL,
 quarantine_relative text NOT NULL,
 state text NOT NULL DEFAULT 'approved' CHECK(state IN('approved','quarantined','removed','conflict','cancelled')),
 message text NOT NULL DEFAULT '',
 approved_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX archive_cleanup_request_operations ON media_archive_cleanup_operations(request_id);
