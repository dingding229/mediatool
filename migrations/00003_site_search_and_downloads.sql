-- +goose Up
ALTER TABLE site_definitions ADD COLUMN common_definition jsonb NOT NULL DEFAULT '{}';
ALTER TABLE site_definitions ADD COLUMN compatibility jsonb NOT NULL DEFAULT '{}';
CREATE TABLE search_runs (
 id uuid PRIMARY KEY,
 keyword text NOT NULL,
 account_ids jsonb NOT NULL,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','running','succeeded','partial','failed')),
 created_at timestamptz NOT NULL DEFAULT now(),
 finished_at timestamptz
);
CREATE TABLE search_site_results (
 run_id uuid NOT NULL REFERENCES search_runs(id) ON DELETE CASCADE,
 account_id uuid NOT NULL REFERENCES integrations(id) ON DELETE RESTRICT,
 state text NOT NULL DEFAULT 'pending',
 message text NOT NULL DEFAULT '',
 result_count integer NOT NULL DEFAULT 0,
 PRIMARY KEY(run_id,account_id)
);
CREATE TABLE resources (
 id uuid PRIMARY KEY,
 account_id uuid NOT NULL REFERENCES integrations(id) ON DELETE RESTRICT,
 torrent_id text NOT NULL,
 title text NOT NULL,
 size_bytes bigint NOT NULL DEFAULT 0,
 seeders integer NOT NULL DEFAULT 0,
 leechers integer NOT NULL DEFAULT 0,
 download_factor double precision,
 public_metadata jsonb NOT NULL DEFAULT '{}',
 download_hint_ciphertext bytea NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(account_id,torrent_id)
);
CREATE TABLE search_run_resources (
 run_id uuid NOT NULL REFERENCES search_runs(id) ON DELETE CASCADE,
 resource_id uuid NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
 PRIMARY KEY(run_id,resource_id)
);
ALTER TABLE download_tasks ADD COLUMN resource_id uuid REFERENCES resources(id);
ALTER TABLE download_tasks ADD COLUMN integration_id uuid REFERENCES integrations(id);
ALTER TABLE download_tasks ADD COLUMN media_type text;
ALTER TABLE download_tasks ADD COLUMN tmdb_id bigint;
ALTER TABLE download_tasks ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
CREATE UNIQUE INDEX download_resource_active_idx ON download_tasks(resource_id) WHERE download_state NOT IN ('failed','cancelled');
CREATE TABLE download_files (
 id uuid PRIMARY KEY,
 task_id uuid NOT NULL REFERENCES download_tasks(id) ON DELETE CASCADE,
 file_index integer NOT NULL,
 relative_path text NOT NULL,
 size_bytes bigint NOT NULL,
 local_path text,
 media_state text NOT NULL DEFAULT 'pending',
 UNIQUE(task_id,file_index)
);
-- +goose Down
DROP TABLE download_files;
DROP INDEX download_resource_active_idx;
ALTER TABLE download_tasks DROP COLUMN updated_at,DROP COLUMN tmdb_id,DROP COLUMN media_type,DROP COLUMN integration_id,DROP COLUMN resource_id;
DROP TABLE search_run_resources,resources,search_site_results,search_runs;
ALTER TABLE site_definitions DROP COLUMN compatibility,DROP COLUMN common_definition;
