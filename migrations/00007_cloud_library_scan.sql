-- +goose Up
ALTER TABLE cloud_files ADD COLUMN missing_scan_count integer NOT NULL DEFAULT 0;
ALTER TABLE cloud_files ADD COLUMN missing_since timestamptz;
CREATE TABLE library_scans (
 id uuid PRIMARY KEY,
 integration_id uuid NOT NULL REFERENCES integrations(id),
 root text NOT NULL,
 state text NOT NULL DEFAULT 'pending',
 files_seen integer NOT NULL DEFAULT 0,
 directories_seen integer NOT NULL DEFAULT 0,
 message text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 completed_at timestamptz
);
CREATE UNIQUE INDEX library_one_active_scan ON library_scans(integration_id,root) WHERE state IN ('pending','running');
CREATE TABLE library_scan_directories (
 scan_id uuid NOT NULL REFERENCES library_scans(id) ON DELETE CASCADE,
 path text NOT NULL,
 state text NOT NULL DEFAULT 'pending',
 PRIMARY KEY(scan_id,path)
);
CREATE TABLE library_scan_seen (
 scan_id uuid NOT NULL REFERENCES library_scans(id) ON DELETE CASCADE,
 file_id uuid NOT NULL REFERENCES cloud_files(id) ON DELETE CASCADE,
 PRIMARY KEY(scan_id,file_id)
);
CREATE TABLE library_candidates (
 id uuid PRIMARY KEY,
 file_id uuid NOT NULL UNIQUE REFERENCES cloud_files(id) ON DELETE CASCADE,
 state text NOT NULL DEFAULT 'needs_confirmation',
 asset_id uuid REFERENCES media_assets(id),
 revision bigint NOT NULL DEFAULT 1,
 updated_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
DROP TABLE library_candidates,library_scan_seen,library_scan_directories,library_scans;
ALTER TABLE cloud_files DROP COLUMN missing_since,DROP COLUMN missing_scan_count;
