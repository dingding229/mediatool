-- +goose Up
CREATE TABLE transfer_tasks (
 id uuid PRIMARY KEY,
 download_file_id uuid NOT NULL UNIQUE REFERENCES download_files(id),
 integration_id uuid NOT NULL REFERENCES integrations(id),
 destination text NOT NULL,
 device_id text NOT NULL,
 upload_id text,
 source_sha1 text,
 state text NOT NULL DEFAULT 'pending',
 bytes_transferred bigint NOT NULL DEFAULT 0,
 phase text NOT NULL DEFAULT 'pending',
 last_error text NOT NULL DEFAULT '',
 updated_at timestamptz NOT NULL DEFAULT now(),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(integration_id,destination)
);
CREATE TABLE media_items (
 id uuid PRIMARY KEY,
 media_type text NOT NULL CHECK(media_type IN ('movie','series')),
 tmdb_id bigint NOT NULL,
 title text NOT NULL,
 release_year text NOT NULL DEFAULT '',
 metadata jsonb NOT NULL,
 UNIQUE(media_type,tmdb_id)
);
CREATE TABLE media_asset_bindings (
 asset_id uuid PRIMARY KEY REFERENCES media_assets(id),
 media_id uuid NOT NULL REFERENCES media_items(id),
 download_file_id uuid UNIQUE REFERENCES download_files(id),
 season integer,
 episode integer,
 state text NOT NULL DEFAULT 'ready'
);
ALTER TABLE download_tasks ADD COLUMN pipeline_state text NOT NULL DEFAULT 'pending';
ALTER TABLE notification_deliveries ADD COLUMN integration_id uuid REFERENCES integrations(id);
ALTER TABLE notification_deliveries ALTER COLUMN channel_id DROP NOT NULL;
ALTER TABLE notification_deliveries ADD COLUMN next_attempt_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE notification_deliveries ADD COLUMN provider_message_id text;
ALTER TABLE notification_deliveries ADD COLUMN last_error text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX notification_integration_event ON notification_deliveries(event_id,integration_id) WHERE integration_id IS NOT NULL;
-- +goose Down
DROP INDEX notification_integration_event;
ALTER TABLE notification_deliveries DROP COLUMN last_error,DROP COLUMN provider_message_id,DROP COLUMN next_attempt_at,DROP COLUMN integration_id;
ALTER TABLE notification_deliveries ALTER COLUMN channel_id SET NOT NULL;
ALTER TABLE download_tasks DROP COLUMN pipeline_state;
DROP TABLE media_asset_bindings,media_items,transfer_tasks;
