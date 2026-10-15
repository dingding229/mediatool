-- +goose Up
ALTER TABLE notification_deliveries ADD COLUMN attempt_token text NOT NULL DEFAULT '';
-- +goose Down
ALTER TABLE notification_deliveries DROP COLUMN attempt_token;
