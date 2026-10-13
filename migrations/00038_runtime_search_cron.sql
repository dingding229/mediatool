-- +goose Up
ALTER TABLE search_runs ADD COLUMN media_context jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE search_runs ADD COLUMN cache_hidden boolean NOT NULL DEFAULT false;
ALTER TABLE site_metric_schedule ADD COLUMN cron_expression text NOT NULL DEFAULT '0 * * * *';
ALTER TABLE library_scan_schedules ALTER COLUMN interval_seconds SET DEFAULT 3600;
ALTER TABLE library_scan_schedules ADD COLUMN cron_expression text NOT NULL DEFAULT '0 * * * *';
UPDATE application_settings SET value=value || '{"subscriptionCron":"*/30 * * * *"}'::jsonb;
UPDATE site_metric_schedule SET next_run_at=date_trunc('hour',now())+interval '1 hour';
-- +goose Down
ALTER TABLE library_scan_schedules DROP COLUMN cron_expression;
ALTER TABLE site_metric_schedule DROP COLUMN cron_expression;
ALTER TABLE search_runs DROP COLUMN media_context, DROP COLUMN cache_hidden;
UPDATE application_settings SET value=value-'subscriptionCron';
