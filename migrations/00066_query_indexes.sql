-- +goose NO TRANSACTION
-- +goose Up
-- Goose executes each nontransactional file on one reserved connection. Drop
-- first also repairs INVALID indexes left by an interrupted concurrent build.
SET lock_timeout='5s';
SET statement_timeout='10min';
DROP INDEX CONCURRENTLY IF EXISTS q_jobs_plugin_owner;
CREATE INDEX CONCURRENTLY q_jobs_plugin_owner ON jobs((payload->>'instanceId'),created_at DESC,id);
DROP INDEX CONCURRENTLY IF EXISTS q_jobs_plugin_service;
CREATE INDEX CONCURRENTLY q_jobs_plugin_service ON jobs((payload->>'instanceId'),(payload->>'serviceId'),(payload->>'snapshotId'),created_at) WHERE kind='plugin.v2.service';
DROP INDEX CONCURRENTLY IF EXISTS q_manual_job_owner;
CREATE INDEX CONCURRENTLY q_manual_job_owner ON plugin_v2_manual_requests(job_id,(instance_id::text));
DROP INDEX CONCURRENTLY IF EXISTS q_runs_workflow;
CREATE INDEX CONCURRENTLY q_runs_workflow ON plugin_v2_runs(dedupe_key text_pattern_ops);
DROP INDEX CONCURRENTLY IF EXISTS q_outbox_bridge;
CREATE INDEX CONCURRENTLY q_outbox_bridge ON outbox_events(created_at,id) WHERE event_type IN('library.organized','files.succeeded','files.failed','download.started','site.account.updated','site.account.deleted','site.metrics.refreshed','subscription.created','subscription.updated','subscription.deleted','library.organize.failed','library.subtitles.copied','library.subtitles.failed','library.history.hidden','download.files.deleted','download.task.deleted','metadata.scraped','subscription.coverage.completed','notification.queued','system.operation.failed','settings.changed','plugin.config.changed','messaging.inbound','messaging.action.received','workflow.started');
DROP INDEX CONCURRENTLY IF EXISTS q_state_snapshot_owner;
CREATE UNIQUE INDEX CONCURRENTLY q_state_snapshot_owner ON plugin_v2_state_snapshots(id,instance_id);
DROP INDEX CONCURRENTLY IF EXISTS plugin_v2_state_values_snapshot;
-- Superseded by the complete Bridge predicate above.
DROP INDEX CONCURRENTLY IF EXISTS plugin_v2_outbox_files;
RESET lock_timeout;
RESET statement_timeout;
-- +goose Down
SET lock_timeout='5s';
SET statement_timeout='10min';
CREATE INDEX CONCURRENTLY IF NOT EXISTS plugin_v2_state_values_snapshot ON plugin_v2_state_values(snapshot_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS plugin_v2_outbox_files ON outbox_events(created_at,id) WHERE event_type IN('files.succeeded','files.failed','download.started');
DROP INDEX CONCURRENTLY IF EXISTS q_state_snapshot_owner;
DROP INDEX CONCURRENTLY IF EXISTS q_outbox_bridge;
DROP INDEX CONCURRENTLY IF EXISTS q_runs_workflow;
DROP INDEX CONCURRENTLY IF EXISTS q_manual_job_owner;
DROP INDEX CONCURRENTLY IF EXISTS q_jobs_plugin_service;
DROP INDEX CONCURRENTLY IF EXISTS q_jobs_plugin_owner;
RESET lock_timeout;
RESET statement_timeout;
