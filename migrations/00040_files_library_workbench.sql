-- +goose Up
ALTER TABLE library_scans ADD COLUMN coverage_started_at timestamptz;
ALTER TABLE library_scans ADD COLUMN integration_revision bigint NOT NULL DEFAULT 0;
ALTER TABLE library_scans ADD COLUMN generation bigint NOT NULL DEFAULT 1;
ALTER TABLE library_scans ADD COLUMN current_directory text NOT NULL DEFAULT '';
ALTER TABLE library_scans ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
DROP INDEX library_one_active_scan;
CREATE UNIQUE INDEX library_one_active_scan ON library_scans(integration_id,root) WHERE state IN ('pending','running','paused');
CREATE TABLE file_batches (
 id uuid PRIMARY KEY,
 digest text NOT NULL,
 state text NOT NULL DEFAULT 'preview' CHECK(state IN ('preview','running','succeeded','partial','needs_reconcile','expired')),
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE file_batch_items (
 batch_id uuid NOT NULL REFERENCES file_batches(id) ON DELETE CASCADE,
 ordinal integer NOT NULL,
 operation_id uuid REFERENCES file_operations(id),
 source text NOT NULL,
 state text NOT NULL DEFAULT 'preview',
 message text NOT NULL DEFAULT '',
 PRIMARY KEY(batch_id,ordinal),
 UNIQUE(operation_id)
);
CREATE TABLE file_batch_parents (
 batch_id uuid NOT NULL REFERENCES file_batches(id) ON DELETE CASCADE,
 integration_id uuid NOT NULL REFERENCES integrations(id),
 path text NOT NULL,
 identity jsonb NOT NULL,
 PRIMARY KEY(batch_id,integration_id,path)
);
CREATE INDEX file_batches_recent ON file_batches(created_at DESC);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM file_batches WHERE state IN ('running','needs_reconcile')) THEN RAISE EXCEPTION 'Reconcile file batches before downgrade'; END IF;
END $$;
-- +goose StatementEnd
DROP TABLE file_batch_parents,file_batch_items,file_batches;
UPDATE library_scans SET state='cancelled' WHERE state='paused';
DROP INDEX library_one_active_scan;
CREATE UNIQUE INDEX library_one_active_scan ON library_scans(integration_id,root) WHERE state IN ('pending','running');
ALTER TABLE library_scans DROP COLUMN coverage_started_at,DROP COLUMN integration_revision,DROP COLUMN generation,DROP COLUMN current_directory,DROP COLUMN updated_at;
