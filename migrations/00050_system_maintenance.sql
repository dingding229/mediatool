-- +goose Up
ALTER TABLE jobs ADD COLUMN started_at timestamptz;
CREATE TABLE ops_policy(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),value jsonb NOT NULL DEFAULT '{}',revision bigint NOT NULL DEFAULT 1,next_backup_at timestamptz NOT NULL DEFAULT now(),next_sample_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now());
INSERT INTO ops_policy(singleton) VALUES(true);
CREATE TABLE ops_backups(id uuid PRIMARY KEY,state text NOT NULL DEFAULT 'pending',origin text NOT NULL DEFAULT 'manual',pinned boolean NOT NULL DEFAULT false,bytes bigint NOT NULL DEFAULT 0,digest text NOT NULL DEFAULT '',manifest jsonb NOT NULL DEFAULT '{}',message text NOT NULL DEFAULT '',job_id uuid,created_at timestamptz NOT NULL DEFAULT now(),completed_at timestamptz);
CREATE TABLE ops_drills(id uuid PRIMARY KEY,backup_id uuid NOT NULL REFERENCES ops_backups(id),state text NOT NULL DEFAULT 'pending',report jsonb NOT NULL DEFAULT '{}',message text NOT NULL DEFAULT '',job_id uuid,created_at timestamptz NOT NULL DEFAULT now(),completed_at timestamptz);
CREATE TABLE ops_releases(revision text PRIMARY KEY CHECK(revision ~ '^[a-f0-9]{40}$'),image text NOT NULL,schema_version bigint NOT NULL,digest text NOT NULL DEFAULT '',verified boolean NOT NULL DEFAULT false,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE ops_requests(id uuid PRIMARY KEY,kind text NOT NULL CHECK(kind IN('restore','rollback')),state text NOT NULL DEFAULT 'preview',payload jsonb NOT NULL,digest text NOT NULL,expires_at timestamptz NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),approved_at timestamptz,completed_at timestamptz,message text NOT NULL DEFAULT '');
CREATE TABLE ops_mode(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),paused boolean NOT NULL DEFAULT false,reason text NOT NULL DEFAULT '',revision bigint NOT NULL DEFAULT 1);
INSERT INTO ops_mode(singleton)VALUES(true);
CREATE TABLE ops_samples(id bigserial PRIMARY KEY,sampled_at timestamptz NOT NULL DEFAULT now(),value jsonb NOT NULL);
CREATE INDEX ops_samples_time ON ops_samples(sampled_at DESC);
CREATE TABLE ops_alerts(code text PRIMARY KEY,state text NOT NULL DEFAULT 'normal',hits integer NOT NULL DEFAULT 0,value double precision,first_seen_at timestamptz,last_seen_at timestamptz,last_sent_at timestamptz,acknowledged_at timestamptz,message text NOT NULL DEFAULT '');
-- +goose Down
ALTER TABLE jobs DROP COLUMN started_at;
DROP TABLE ops_alerts,ops_samples,ops_mode,ops_requests,ops_releases,ops_drills,ops_backups,ops_policy;
