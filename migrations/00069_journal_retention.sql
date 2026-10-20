-- +goose Up
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='10min';
ALTER TABLE outbox_events ADD COLUMN plugin_journal_cursor bigint CHECK(plugin_journal_cursor>0), ADD COLUMN plugin_compacted boolean NOT NULL DEFAULT false;
-- Keep source dedupe identities after collecting journal payloads. This is an
-- event-ID receipt, NOT a transaction-allocation/time high-water mark.
UPDATE outbox_events o SET plugin_journal_cursor=j.cursor FROM plugin_v2_event_journal j WHERE j.event_id=o.id;
ALTER TABLE plugin_v2_event_journal ADD COLUMN ingested_at timestamptz NOT NULL DEFAULT clock_timestamp();
CREATE TABLE plugin_v2_journal_retention(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),retired_cursor bigint NOT NULL DEFAULT 0 CHECK(retired_cursor>=0));
INSERT INTO plugin_v2_journal_retention(singleton)VALUES(true);
-- +goose Down
-- Removing durable receipts could replay previously collected events.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'journal retention requires full pre-upgrade backup for downgrade'; END $$;
-- +goose StatementEnd
