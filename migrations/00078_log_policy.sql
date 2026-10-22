-- +goose Up
-- File logs remain bounded JSONL; only their policy lives in PostgreSQL.
UPDATE application_settings SET value=jsonb_set(value,'{logging}', '{"retentionDays":7,"debugUntil":"0001-01-01T00:00:00Z"}'::jsonb),revision=revision+1 WHERE NOT value ? 'logging';
-- +goose Down
UPDATE application_settings SET value=value-'logging',revision=revision+1 WHERE value ? 'logging';
