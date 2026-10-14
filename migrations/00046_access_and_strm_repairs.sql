-- +goose Up
CREATE TABLE player_access_events (
 id uuid PRIMARY KEY,
 observation_id text NOT NULL,
 user_agent text NOT NULL,
 client_ip text NOT NULL,
 entry text NOT NULL,
 method text NOT NULL,
 allowed boolean NOT NULL,
 reason text NOT NULL,
 policy_revision bigint NOT NULL,
 emby_user_id text NOT NULL DEFAULT '',
 emby_user_name text NOT NULL DEFAULT '',
 emby_verified boolean NOT NULL DEFAULT false,
 status integer NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX player_access_events_recent ON player_access_events(created_at DESC,id);
CREATE INDEX player_access_events_ua ON player_access_events(observation_id,created_at DESC);
CREATE TABLE strm_repair_batches (
 id uuid PRIMARY KEY,
 state text NOT NULL DEFAULT 'preview',
 digest text NOT NULL,
 expires_at timestamptz NOT NULL,
 job_id uuid REFERENCES jobs(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE strm_repair_items (
 batch_id uuid NOT NULL REFERENCES strm_repair_batches(id) ON DELETE CASCADE,
 ordinal integer NOT NULL,
 asset_id uuid NOT NULL REFERENCES media_assets(id),
 asset_revision bigint NOT NULL,
 relative_path text NOT NULL,
 content_digest bytea,
 journal_id uuid REFERENCES media_artifact_journal(id),
 state text NOT NULL,
 message text NOT NULL,
 PRIMARY KEY(batch_id,ordinal)
);
-- +goose Down
DROP TABLE strm_repair_items,strm_repair_batches,player_access_events;
