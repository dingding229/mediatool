-- +goose Up
ALTER TABLE media_archive_cleanup_operations DROP CONSTRAINT media_archive_cleanup_operations_state_check;
ALTER TABLE media_archive_cleanup_operations ADD CONSTRAINT media_archive_cleanup_operations_state_check CHECK(state IN('approved','quarantined','removed','conflict','cancelled'));
ALTER TABLE jobs DROP CONSTRAINT jobs_state_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_state_check CHECK(state IN ('pending','running','retry','succeeded','failed','paused','cancelled','unknown','checking'));
ALTER TABLE jobs ADD COLUMN cancel_requested_at timestamptz, ADD COLUMN cancel_acknowledged_at timestamptz, ADD COLUMN control_revision bigint NOT NULL DEFAULT 1;
CREATE INDEX jobs_control_recovery ON jobs(lease_until,id) WHERE state='running';
CREATE INDEX jobs_checking_recovery ON jobs(updated_at,id) WHERE state='checking';
-- +goose Down
DROP INDEX IF EXISTS jobs_control_recovery;
DROP INDEX IF EXISTS jobs_checking_recovery;
UPDATE media_archive_cleanup_operations SET state='conflict' WHERE state='cancelled';
ALTER TABLE media_archive_cleanup_operations DROP CONSTRAINT media_archive_cleanup_operations_state_check;
ALTER TABLE media_archive_cleanup_operations ADD CONSTRAINT media_archive_cleanup_operations_state_check CHECK(state IN('approved','quarantined','removed','conflict'));
UPDATE jobs SET state='paused' WHERE state IN('unknown','checking');
ALTER TABLE jobs DROP CONSTRAINT jobs_state_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_state_check CHECK(state IN ('pending','running','retry','succeeded','failed','paused','cancelled'));
ALTER TABLE jobs DROP COLUMN cancel_requested_at,DROP COLUMN cancel_acknowledged_at,DROP COLUMN control_revision;
