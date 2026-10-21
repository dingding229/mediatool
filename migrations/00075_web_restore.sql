-- +goose Up
CREATE TABLE ops_web_restores (
 id uuid PRIMARY KEY, backup_id uuid NOT NULL REFERENCES ops_backups(id),
 snapshot_id uuid REFERENCES ops_backups(id), state text NOT NULL DEFAULT 'preview',
 manifest_digest text NOT NULL, protected jsonb NOT NULL, applied_backup uuid,
 message text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL DEFAULT now()+interval '10 minutes', completed_at timestamptz
);
CREATE INDEX ops_web_restores_created ON ops_web_restores(created_at DESC);
CREATE UNIQUE INDEX ops_web_restores_active ON ops_web_restores((true)) WHERE state IN ('pending','running','unknown');
-- Imported in the same transaction as business data; the live progress/auth
-- tables are excluded from pg_restore so status/auth reads do not block.
CREATE TABLE ops_restore_commit(request_id uuid PRIMARY KEY,backup_id uuid NOT NULL);
-- +goose Down
DROP TABLE ops_restore_commit;
DROP TABLE ops_web_restores;
