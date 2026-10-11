-- +goose Up
ALTER TABLE library_scan_schedules
 ADD COLUMN watch_changes boolean NOT NULL DEFAULT false,
 ADD COLUMN change_scan_after timestamptz,
 ADD COLUMN last_change_at timestamptz,
 ADD COLUMN watch_state text NOT NULL DEFAULT 'disabled',
 ADD COLUMN watch_message text NOT NULL DEFAULT '';
CREATE INDEX library_change_scan_due ON library_scan_schedules(change_scan_after)
 WHERE enabled AND watch_changes AND change_scan_after IS NOT NULL;

-- +goose Down
DROP INDEX library_change_scan_due;
ALTER TABLE library_scan_schedules DROP COLUMN watch_changes, DROP COLUMN change_scan_after,
 DROP COLUMN last_change_at, DROP COLUMN watch_state, DROP COLUMN watch_message;
