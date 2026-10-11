-- +goose Up
ALTER TABLE subscriptions ADD COLUMN source text NOT NULL DEFAULT 'search' CHECK(source IN ('search','rss','both'));
ALTER TABLE subscriptions ADD COLUMN interval_seconds integer NOT NULL DEFAULT 1800 CHECK(interval_seconds BETWEEN 300 AND 86400);
ALTER TABLE subscriptions ADD COLUMN next_run_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE subscriptions ADD COLUMN revision bigint NOT NULL DEFAULT 1;
ALTER TABLE subscriptions ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE subscriptions ADD COLUMN last_state text NOT NULL DEFAULT 'idle';
ALTER TABLE subscriptions ADD COLUMN last_message text NOT NULL DEFAULT '';
CREATE TABLE subscription_runs (
 id uuid PRIMARY KEY,
 subscription_id uuid NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
 preview boolean NOT NULL DEFAULT false,
 state text NOT NULL DEFAULT 'pending',
 search_run_id uuid REFERENCES search_runs(id),
 revision bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 finished_at timestamptz
);
CREATE UNIQUE INDEX subscription_single_active_run ON subscription_runs(subscription_id) WHERE state IN ('pending','running');
CREATE TABLE subscription_decisions (
 id uuid PRIMARY KEY,
 run_id uuid NOT NULL REFERENCES subscription_runs(id) ON DELETE CASCADE,
 resource_id uuid REFERENCES resources(id) ON DELETE SET NULL,
 title text NOT NULL,
 action text NOT NULL,
 reasons jsonb NOT NULL,
 covered_episodes jsonb NOT NULL DEFAULT '[]',
 download_task_id uuid REFERENCES download_tasks(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(run_id,resource_id)
);
CREATE TABLE subscription_rss_accounts (
 subscription_id uuid NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
 account_id uuid NOT NULL REFERENCES integrations(id) ON DELETE RESTRICT,
 initialized_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(subscription_id,account_id)
);
CREATE TABLE subscription_rss_seen (
 subscription_id uuid NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
 account_id uuid NOT NULL REFERENCES integrations(id) ON DELETE RESTRICT,
 guid_digest text NOT NULL,
 seen_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(subscription_id,account_id,guid_digest)
);
CREATE TABLE subscription_rss_run_items (
 run_id uuid NOT NULL REFERENCES subscription_runs(id) ON DELETE CASCADE,
 account_id uuid NOT NULL REFERENCES integrations(id) ON DELETE RESTRICT,
 guid_digest text NOT NULL,
 PRIMARY KEY(run_id,account_id,guid_digest)
);
ALTER TABLE media_asset_bindings ADD COLUMN episodes jsonb NOT NULL DEFAULT '[]';
-- +goose Down
ALTER TABLE media_asset_bindings DROP COLUMN episodes;
DROP TABLE subscription_rss_run_items,subscription_rss_seen,subscription_rss_accounts,subscription_decisions,subscription_runs;
ALTER TABLE subscriptions DROP COLUMN last_message,DROP COLUMN last_state,DROP COLUMN updated_at,DROP COLUMN revision,DROP COLUMN next_run_at,DROP COLUMN interval_seconds,DROP COLUMN source;
