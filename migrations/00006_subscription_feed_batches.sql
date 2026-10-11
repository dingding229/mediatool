-- +goose Up
CREATE TABLE subscription_rss_run_accounts (
 run_id uuid NOT NULL REFERENCES subscription_runs(id) ON DELETE CASCADE,
 account_id uuid NOT NULL REFERENCES integrations(id) ON DELETE RESTRICT,
 PRIMARY KEY(run_id,account_id)
);
-- +goose Down
DROP TABLE subscription_rss_run_accounts;
