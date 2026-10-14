-- +goose Up
CREATE TABLE player_ua_policy (
 id text PRIMARY KEY CHECK(id='main'),
 enabled boolean NOT NULL DEFAULT false,
 revision bigint NOT NULL DEFAULT 1,
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO player_ua_policy(id)VALUES('main');
CREATE TABLE player_ua_rules (
 id uuid PRIMARY KEY,
 label text NOT NULL CHECK(length(label)<=80),
 match_type text NOT NULL CHECK(match_type IN ('exact','prefix')),
 user_agent text NOT NULL CHECK(octet_length(user_agent) BETWEEN 1 AND 4096),
 enabled boolean NOT NULL DEFAULT true,
 UNIQUE(match_type,user_agent)
);
CREATE TABLE player_ua_observations (
 id text PRIMARY KEY CHECK(length(id)=64),
 user_agent text NOT NULL CHECK(octet_length(user_agent)<=4096),
 first_seen_at timestamptz NOT NULL DEFAULT now(),
 last_seen_at timestamptz NOT NULL DEFAULT now(),
 request_count bigint NOT NULL DEFAULT 1,
 allowed_count bigint NOT NULL DEFAULT 0,
 denied_count bigint NOT NULL DEFAULT 0,
 last_allowed boolean NOT NULL,
 last_reason text NOT NULL,
 last_entry text NOT NULL,
 last_method text NOT NULL,
 last_client_ip text NOT NULL,
 last_rule_id text NOT NULL DEFAULT '',
 last_policy_revision bigint NOT NULL
);
CREATE INDEX player_ua_observations_recent ON player_ua_observations(last_seen_at DESC,id);
-- +goose Down
DROP TABLE player_ua_observations,player_ua_rules,player_ua_policy;
