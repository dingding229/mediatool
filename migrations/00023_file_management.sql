-- +goose Up
ALTER TABLE integrations DROP CONSTRAINT integrations_kind_check;
ALTER TABLE integrations ADD CONSTRAINT integrations_kind_check CHECK(kind IN ('clouddrive2','open115','local','qbittorrent','emby','tmdb','telegram','ptsite'));
DROP INDEX integrations_one_enabled_kind;
CREATE UNIQUE INDEX integrations_one_enabled_kind ON integrations(kind) WHERE enabled AND kind NOT IN ('ptsite','local');
CREATE TABLE file_operations (
 id uuid PRIMARY KEY,
 integration_id uuid NOT NULL REFERENCES integrations(id),
 integration_revision bigint NOT NULL,
 backend_kind text NOT NULL CHECK(backend_kind IN ('cloud','local')),
 operation text NOT NULL CHECK(operation IN ('rename','move','organize','mkdir','trash')),
 plan jsonb NOT NULL,
 plan_digest text NOT NULL,
 state text NOT NULL DEFAULT 'preview' CHECK(state IN ('preview','running','succeeded','needs_reconcile','failed','expired')),
 completed_steps integer NOT NULL DEFAULT 0,
 message text NOT NULL DEFAULT '',
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX file_operations_recent ON file_operations(created_at DESC);
CREATE INDEX file_operations_active ON file_operations(integration_id) WHERE state IN ('running','needs_reconcile');
-- +goose Down
-- Block downgrade rather than drop evidence for an unresolved external operation.
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM file_operations WHERE state IN ('running','needs_reconcile')) THEN RAISE EXCEPTION 'Reconcile file operations before downgrade'; END IF;
 IF EXISTS(SELECT 1 FROM integrations WHERE kind='local') THEN RAISE EXCEPTION 'Remove explicit local connections before downgrade'; END IF;
END $$;
-- +goose StatementEnd
DROP TABLE file_operations;
DROP INDEX integrations_one_enabled_kind;
CREATE UNIQUE INDEX integrations_one_enabled_kind ON integrations(kind) WHERE enabled AND kind<>'ptsite';
ALTER TABLE integrations DROP CONSTRAINT integrations_kind_check;
ALTER TABLE integrations ADD CONSTRAINT integrations_kind_check CHECK(kind IN ('clouddrive2','open115','qbittorrent','emby','tmdb','telegram','ptsite'));
