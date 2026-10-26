-- +goose Up
CREATE TABLE player_ip_block_policy(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),last_counter_batch uuid);
INSERT INTO player_ip_block_policy(singleton) VALUES(true);
CREATE TABLE player_ip_block_rules(
 id uuid PRIMARY KEY,network cidr NOT NULL UNIQUE CHECK(masklen(network)>0),note text NOT NULL DEFAULT '' CHECK(char_length(note)<=500),
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 hit_count bigint NOT NULL DEFAULT 0 CHECK(hit_count>=0),last_hit_at timestamptz
);
-- +goose Down
DROP TABLE player_ip_block_rules;
DROP TABLE player_ip_block_policy;
