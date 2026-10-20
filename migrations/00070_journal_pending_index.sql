-- +goose NO TRANSACTION
-- +goose Up
SET lock_timeout='5s';
SET statement_timeout='10min';
DROP INDEX CONCURRENTLY IF EXISTS q_runs_event_reference;
CREATE INDEX CONCURRENTLY q_runs_event_reference ON plugin_v2_runs(event_cursor) WHERE event_cursor IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS q_outbox_unbridged;
CREATE INDEX CONCURRENTLY q_outbox_unbridged ON outbox_events(created_at,id) WHERE plugin_journal_cursor IS NULL AND event_type IN('library.organized','files.succeeded','files.failed','download.started','site.account.updated','site.account.deleted','site.metrics.refreshed','subscription.created','subscription.updated','subscription.deleted','library.organize.failed','library.subtitles.copied','library.subtitles.failed','library.history.hidden','download.files.deleted','download.task.deleted','metadata.scraped','subscription.coverage.completed','notification.queued','system.operation.failed','settings.changed','plugin.config.changed','messaging.inbound','messaging.action.received','workflow.started');
DROP INDEX CONCURRENTLY IF EXISTS q_outbox_compact;
CREATE INDEX CONCURRENTLY q_outbox_compact ON outbox_events(plugin_journal_cursor,id) WHERE NOT plugin_compacted AND dispatched_at IS NOT NULL AND plugin_journal_cursor IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS q_outbox_bridge;
RESET lock_timeout;
RESET statement_timeout;
-- +goose Down
SET lock_timeout='5s';
SET statement_timeout='10min';
DROP INDEX CONCURRENTLY IF EXISTS q_outbox_compact;
DROP INDEX CONCURRENTLY IF EXISTS q_outbox_unbridged;
CREATE INDEX CONCURRENTLY q_outbox_bridge ON outbox_events(created_at,id) WHERE event_type IN('library.organized','files.succeeded','files.failed','download.started','site.account.updated','site.account.deleted','site.metrics.refreshed','subscription.created','subscription.updated','subscription.deleted','library.organize.failed','library.subtitles.copied','library.subtitles.failed','library.history.hidden','download.files.deleted','download.task.deleted','metadata.scraped','subscription.coverage.completed','notification.queued','system.operation.failed','settings.changed','plugin.config.changed','messaging.inbound','messaging.action.received','workflow.started');
RESET lock_timeout;
RESET statement_timeout;
