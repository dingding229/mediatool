-- +goose Up
ALTER TABLE subscriptions ALTER COLUMN next_run_at DROP NOT NULL;
ALTER TABLE library_scan_schedules ALTER COLUMN next_run_at DROP NOT NULL;
ALTER TABLE site_metric_schedule ALTER COLUMN next_run_at DROP NOT NULL;
ALTER TABLE ops_policy ALTER COLUMN next_backup_at DROP NOT NULL;

-- +goose Down
-- Legacy scheduling cannot represent an exhausted plan. Deterministically stop
-- only NULL plans, keep their expressions, and fill the transaction timestamp.
UPDATE subscriptions SET enabled=false,next_run_at=transaction_timestamp() WHERE next_run_at IS NULL;
UPDATE library_scan_schedules SET enabled=false,next_run_at=transaction_timestamp() WHERE next_run_at IS NULL;
UPDATE site_metric_schedule SET enabled=false,next_run_at=transaction_timestamp() WHERE next_run_at IS NULL;
UPDATE ops_policy SET value=jsonb_set(value,'{backupEnabled}','false'),next_backup_at=transaction_timestamp() WHERE next_backup_at IS NULL;
ALTER TABLE subscriptions ALTER COLUMN next_run_at SET NOT NULL;
ALTER TABLE library_scan_schedules ALTER COLUMN next_run_at SET NOT NULL;
ALTER TABLE site_metric_schedule ALTER COLUMN next_run_at SET NOT NULL;
ALTER TABLE ops_policy ALTER COLUMN next_backup_at SET NOT NULL;
