-- +goose Up
CREATE TABLE app_settings (
 key text PRIMARY KEY,
 value jsonb NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE site_definitions (
 id text PRIMARY KEY,
 name text NOT NULL,
 schema_name text NOT NULL DEFAULT '',
 source_commit text NOT NULL,
 definition jsonb NOT NULL,
 local_overrides jsonb NOT NULL DEFAULT '{}',
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE site_accounts (
 id uuid PRIMARY KEY,
 definition_id text NOT NULL REFERENCES site_definitions(id),
 name text NOT NULL,
 credentials_ciphertext bytea NOT NULL,
 enabled boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE subscriptions (
 id uuid PRIMARY KEY,
 title text NOT NULL,
 media_type text NOT NULL CHECK (media_type IN ('movie','series')),
 identity jsonb NOT NULL DEFAULT '{}',
 rules jsonb NOT NULL DEFAULT '{}',
 coverage jsonb NOT NULL DEFAULT '{}',
 enabled boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE download_tasks (
 id uuid PRIMARY KEY,
 site_account_id uuid REFERENCES site_accounts(id),
 subscription_id uuid REFERENCES subscriptions(id),
 torrent_id text,
 info_hash text,
 download_state text NOT NULL DEFAULT 'pending',
 seeding_state text NOT NULL DEFAULT 'pending',
 downloader_task_id text,
 details jsonb NOT NULL DEFAULT '{}',
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(site_account_id,torrent_id)
);
CREATE TABLE storage_connections (
 id uuid PRIMARY KEY,
 name text NOT NULL,
 provider text NOT NULL CHECK(provider='clouddrive2'),
 settings jsonb NOT NULL DEFAULT '{}',
 credentials_ciphertext bytea,
 enabled boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cloud_files (
 id uuid PRIMARY KEY,
 storage_id uuid NOT NULL REFERENCES storage_connections(id),
 provider_file_id text,
 pick_code text,
 full_path text NOT NULL,
 size_bytes bigint NOT NULL CHECK(size_bytes>=0),
 state text NOT NULL DEFAULT 'present',
 metadata jsonb NOT NULL DEFAULT '{}',
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(storage_id,full_path)
);
CREATE TABLE media_assets (
 id uuid PRIMARY KEY,
 cloud_file_id uuid NOT NULL REFERENCES cloud_files(id),
 media_identity jsonb NOT NULL DEFAULT '{}',
 edition text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE strm_entries (
 id uuid PRIMARY KEY,
 asset_id uuid NOT NULL REFERENCES media_assets(id),
 output_path text NOT NULL UNIQUE,
 -- Do not store a secret-bearing URL; reconstruct it from the key reference.
 key_reference text NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(asset_id,output_path)
);
CREATE TABLE emby_item_mappings (
 emby_instance text NOT NULL,
 item_id text NOT NULL,
 media_source_id text NOT NULL,
 asset_id uuid NOT NULL REFERENCES media_assets(id),
 PRIMARY KEY(emby_instance,item_id,media_source_id)
);
CREATE TABLE jobs (
 id uuid PRIMARY KEY,
 kind text NOT NULL,
 payload jsonb NOT NULL DEFAULT '{}',
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','running','retry','succeeded','failed')),
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts>=0),
 max_attempts integer NOT NULL DEFAULT 3 CHECK(max_attempts>0),
 available_at timestamptz NOT NULL DEFAULT now(),
 lease_until timestamptz,
 lease_token uuid,
 locked_by text,
 last_error text,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(attempts<=max_attempts),
 CHECK((state='running' AND lease_until IS NOT NULL AND lease_token IS NOT NULL AND locked_by IS NOT NULL)
    OR (state<>'running' AND lease_until IS NULL AND lease_token IS NULL AND locked_by IS NULL))
);
CREATE INDEX jobs_available_idx ON jobs(available_at,created_at) WHERE state IN ('pending','retry');
CREATE INDEX jobs_expired_idx ON jobs(lease_until) WHERE state='running';
CREATE TABLE outbox_events (
 id uuid PRIMARY KEY,
 event_type text NOT NULL,
 aggregate_id uuid NOT NULL,
 payload jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 dispatched_at timestamptz
);
CREATE INDEX outbox_pending_idx ON outbox_events(created_at) WHERE dispatched_at IS NULL;
CREATE TABLE notification_channels (
 id uuid PRIMARY KEY,
 name text NOT NULL,
 kind text NOT NULL,
 settings jsonb NOT NULL DEFAULT '{}',
 credentials_ciphertext bytea,
 enabled boolean NOT NULL DEFAULT false
);
CREATE TABLE notification_deliveries (
 id uuid PRIMARY KEY,
 event_id uuid NOT NULL REFERENCES outbox_events(id),
 channel_id uuid NOT NULL REFERENCES notification_channels(id),
 state text NOT NULL DEFAULT 'pending',
 attempts integer NOT NULL DEFAULT 0,
 UNIQUE(event_id,channel_id)
);
CREATE TABLE audit_logs (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 actor text NOT NULL,
 action text NOT NULL,
 details jsonb NOT NULL DEFAULT '{}',
 created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
DROP TABLE audit_logs,notification_deliveries,notification_channels,outbox_events,jobs,emby_item_mappings,strm_entries,media_assets,cloud_files,storage_connections,download_tasks,subscriptions,site_accounts,site_definitions,app_settings;
