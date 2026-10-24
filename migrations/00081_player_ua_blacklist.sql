-- +goose Up
SET LOCAL lock_timeout = '5s';
ALTER TABLE player_ua_policy ADD COLUMN mode text NOT NULL DEFAULT 'off' CHECK(mode IN ('off','whitelist','blacklist'));
UPDATE player_ua_policy SET mode=CASE WHEN enabled THEN 'whitelist' ELSE 'off' END;
ALTER TABLE player_ua_rules ADD COLUMN list_type text NOT NULL DEFAULT 'whitelist' CHECK(list_type IN ('whitelist','blacklist'));
ALTER TABLE player_ua_rules DROP CONSTRAINT player_ua_rules_match_type_check;
ALTER TABLE player_ua_rules ADD CONSTRAINT player_ua_rules_match_type_check CHECK(match_type IN ('exact','prefix','contains'));
ALTER TABLE player_ua_rules DROP CONSTRAINT player_ua_rules_match_type_user_agent_key;
ALTER TABLE player_ua_rules ADD UNIQUE(list_type,match_type,user_agent);
-- +goose Down
-- Downgrading cannot safely represent blacklist policies; restore the saved whitelist.
UPDATE player_ua_policy SET enabled=(mode='whitelist');
DELETE FROM player_ua_rules WHERE list_type='blacklist' OR match_type='contains';
ALTER TABLE player_ua_rules DROP CONSTRAINT player_ua_rules_list_type_match_type_user_agent_key;
ALTER TABLE player_ua_rules DROP COLUMN list_type;
ALTER TABLE player_ua_rules DROP CONSTRAINT player_ua_rules_match_type_check;
ALTER TABLE player_ua_rules ADD CONSTRAINT player_ua_rules_match_type_check CHECK(match_type IN ('exact','prefix'));
ALTER TABLE player_ua_rules ADD UNIQUE(match_type,user_agent);
ALTER TABLE player_ua_policy DROP COLUMN mode;
