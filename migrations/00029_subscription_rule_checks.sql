-- +goose Up
ALTER TABLE subscription_decisions ADD COLUMN rule_checks jsonb NOT NULL DEFAULT '[]'::jsonb;
-- +goose Down
ALTER TABLE subscription_decisions DROP COLUMN rule_checks;
