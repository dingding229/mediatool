-- +goose Up
ALTER TABLE download_tasks ADD COLUMN source_control_revision bigint NOT NULL DEFAULT 1;
ALTER TABLE download_tasks ADD COLUMN source_recovery_required boolean NOT NULL DEFAULT false;
ALTER TABLE download_tasks ADD COLUMN source_resume_digest text;
ALTER TABLE download_tasks ADD COLUMN source_resume_previous_revision bigint;
-- +goose Down
ALTER TABLE download_tasks DROP COLUMN source_resume_previous_revision,DROP COLUMN source_resume_digest,DROP COLUMN source_recovery_required,DROP COLUMN source_control_revision;
