-- +goose Up
SET LOCAL lock_timeout='5s';
ALTER TABLE plugin_v2_runs DROP CONSTRAINT plugin_v2_runs_kind_check;
ALTER TABLE plugin_v2_runs ADD CONSTRAINT plugin_v2_runs_kind_check CHECK(kind IN('chain','module','ui','api','service','command','action','event','tool'));
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN IF EXISTS(SELECT 1 FROM plugin_v2_runs WHERE kind='tool') THEN RAISE EXCEPTION 'tool history exists; restore a backup instead of destructive downgrade'; END IF; END $$;
-- +goose StatementEnd
ALTER TABLE plugin_v2_runs DROP CONSTRAINT plugin_v2_runs_kind_check;
ALTER TABLE plugin_v2_runs ADD CONSTRAINT plugin_v2_runs_kind_check CHECK(kind IN('chain','module','ui','api','service','command','action','event'));
