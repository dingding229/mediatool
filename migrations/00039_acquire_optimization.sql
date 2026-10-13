-- +goose Up
CREATE TABLE network_proxy_settings (
 id text PRIMARY KEY CHECK(id='main'),
 tmdb_enabled boolean NOT NULL DEFAULT false,
 sites_enabled boolean NOT NULL DEFAULT false,
 url_ciphertext bytea NOT NULL DEFAULT ''::bytea,
 revision bigint NOT NULL DEFAULT 1,
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO network_proxy_settings(id) VALUES('main');
ALTER TABLE subscriptions ADD COLUMN archived_at timestamptz;
CREATE INDEX subscriptions_history ON subscriptions(archived_at,updated_at DESC);
-- +goose Down
DROP INDEX subscriptions_history;
ALTER TABLE subscriptions DROP COLUMN archived_at;
DROP TABLE network_proxy_settings;
