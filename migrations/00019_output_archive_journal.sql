-- +goose Up
ALTER TABLE media_artifact_journal
 ADD COLUMN operation text NOT NULL DEFAULT 'write' CHECK(operation IN ('write','archive')),
 ADD COLUMN archive_relative text,
 ADD COLUMN requires_intents uuid[] NOT NULL DEFAULT '{}';
ALTER TABLE media_artifact_journal ADD CONSTRAINT archive_intent_shape
 CHECK((operation='write' AND archive_relative IS NULL) OR
       (operation='archive' AND archive_relative IS NOT NULL AND expected_digest IS NOT NULL));

-- +goose Down
-- Do not roll back while an unfinished archive intent exists; it would lose
-- the filesystem/DB recovery contract for already-moved output files.
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM media_artifact_journal WHERE operation='archive' AND state<>'applied') THEN
  RAISE EXCEPTION 'unfinished archive operations must be reconciled before rollback';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE media_artifact_journal DROP CONSTRAINT archive_intent_shape;
ALTER TABLE media_artifact_journal DROP COLUMN operation, DROP COLUMN archive_relative, DROP COLUMN requires_intents;
