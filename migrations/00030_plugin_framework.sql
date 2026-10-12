-- +goose Up
ALTER TABLE extension_plugins ADD COLUMN wasm_module bytea;
ALTER TABLE extension_plugins ADD COLUMN plugin_data_ciphertext bytea;
ALTER TABLE extension_plugins ADD COLUMN readme text NOT NULL DEFAULT '';
CREATE TABLE extension_repositories(id uuid PRIMARY KEY,name text NOT NULL,url text NOT NULL UNIQUE,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE extension_jobs(id uuid PRIMARY KEY,plugin_id text NOT NULL REFERENCES extension_plugins(id)ON DELETE CASCADE,service_id text NOT NULL,revision bigint NOT NULL,state text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),finished_at timestamptz);
CREATE TABLE extension_service_ticks(plugin_id text NOT NULL REFERENCES extension_plugins(id)ON DELETE CASCADE,service_id text NOT NULL,revision bigint NOT NULL,next_at timestamptz NOT NULL,PRIMARY KEY(plugin_id,service_id));
-- +goose Down
DROP TABLE extension_service_ticks,extension_jobs,extension_repositories;
ALTER TABLE extension_plugins DROP COLUMN wasm_module,DROP COLUMN plugin_data_ciphertext,DROP COLUMN readme;
