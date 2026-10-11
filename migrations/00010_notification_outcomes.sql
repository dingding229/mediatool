-- +goose Up
ALTER TABLE notification_deliveries ADD COLUMN started_at timestamptz;
ALTER TABLE notification_deliveries ADD COLUMN integration_revision bigint;
ALTER TABLE notification_deliveries ADD COLUMN last_http_code integer;
-- +goose Down
ALTER TABLE notification_deliveries DROP COLUMN last_http_code,DROP COLUMN integration_revision,DROP COLUMN started_at;
