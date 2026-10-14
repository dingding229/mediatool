-- +goose Up
CREATE TABLE directory_classification(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),value jsonb NOT NULL DEFAULT 'null',revision bigint NOT NULL DEFAULT 1,updated_at timestamptz NOT NULL DEFAULT now());
INSERT INTO directory_classification(singleton)VALUES(true);
ALTER TABLE transfer_tasks ADD COLUMN layout_snapshot jsonb NOT NULL DEFAULT '{}';
CREATE TABLE external_api_keys(id uuid PRIMARY KEY,name text NOT NULL,token_digest bytea NOT NULL UNIQUE,token_ciphertext bytea NOT NULL,prefix text NOT NULL,scopes jsonb NOT NULL,expires_at timestamptz NOT NULL,revoked_at timestamptz,last_used_at timestamptz,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE external_api_calls(id bigserial PRIMARY KEY,key_id uuid REFERENCES external_api_keys(id),method text NOT NULL,path text NOT NULL,status integer NOT NULL,created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX external_api_calls_recent ON external_api_calls(created_at DESC);
CREATE TABLE download_cleanup_requests(task_id uuid PRIMARY KEY REFERENCES download_tasks(id),state text NOT NULL DEFAULT 'pending',message text NOT NULL DEFAULT '',evidence jsonb NOT NULL DEFAULT '{}',approved_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now());
-- +goose Down
DROP TABLE download_cleanup_requests,external_api_calls,external_api_keys;
ALTER TABLE transfer_tasks DROP COLUMN layout_snapshot;
DROP TABLE directory_classification;
