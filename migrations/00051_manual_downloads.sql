-- +goose Up
CREATE TABLE manual_download_tasks (
 id uuid PRIMARY KEY,
 integration_id uuid NOT NULL REFERENCES integrations(id),
 info_hash text NOT NULL,
 title text NOT NULL,
 source_kind text NOT NULL CHECK(source_kind IN ('file','url','magnet')),
 source_label text NOT NULL,
 payload_ciphertext bytea NOT NULL,
 endpoint text NOT NULL,
 save_path text NOT NULL,
 category text NOT NULL,
 state text NOT NULL DEFAULT 'pending',
 submit_attempted boolean NOT NULL DEFAULT false,
 progress double precision,
 qb_state text NOT NULL DEFAULT '',
 download_speed bigint NOT NULL DEFAULT 0,
 upload_speed bigint NOT NULL DEFAULT 0,
 total_bytes bigint NOT NULL DEFAULT 0,
 message text NOT NULL DEFAULT '',
 synced_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(integration_id,info_hash)
);
-- +goose Down
DROP TABLE manual_download_tasks;
