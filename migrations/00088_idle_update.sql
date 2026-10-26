-- +goose Up
CREATE TABLE idle_update_policy (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 enabled boolean NOT NULL DEFAULT false,
 start_time text NOT NULL DEFAULT '02:00', end_time text NOT NULL DEFAULT '05:00',
 quiet_minutes integer NOT NULL DEFAULT 60 CHECK(quiet_minutes BETWEEN 15 AND 360),
 revision bigint NOT NULL DEFAULT 1, grant_id text NOT NULL DEFAULT '',
 account_id uuid, credential_version bigint, granted_at timestamptz,
 reason text NOT NULL DEFAULT '', updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO idle_update_policy(singleton) VALUES(true);
CREATE TABLE idle_update_activity (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton), last_issued_at timestamptz NOT NULL
);
-- A fresh install/upgraded app must first observe a complete quiet interval.
INSERT INTO idle_update_activity VALUES(true,clock_timestamp());
CREATE TABLE idle_update_attempts (
 id text PRIMARY KEY, version text NOT NULL, local_day date NOT NULL, timezone text NOT NULL,
 grant_id text NOT NULL, state text NOT NULL, job_id text NOT NULL DEFAULT '',
 message text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(version,local_day)
);
CREATE INDEX idle_update_attempts_recent ON idle_update_attempts(created_at DESC);
-- +goose Down
DROP TABLE idle_update_attempts;
DROP TABLE idle_update_activity;
DROP TABLE idle_update_policy;
