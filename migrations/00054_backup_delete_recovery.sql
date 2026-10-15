-- +goose Up
ALTER TABLE ops_backups ADD COLUMN deletion_started_at timestamptz;
-- Do not infer a deletion start from the original backup creation time. Give
-- any already in-flight intent a full recovery grace period on this upgrade.
UPDATE ops_backups SET deletion_started_at=now() WHERE state='deleting';
CREATE INDEX ops_backups_deleting_started ON ops_backups((COALESCE(deletion_started_at,created_at))) WHERE state='deleting';
-- +goose Down
DROP INDEX ops_backups_deleting_started;
ALTER TABLE ops_backups DROP COLUMN deletion_started_at;
