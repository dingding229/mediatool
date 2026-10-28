-- +goose Up
-- Legacy /32 and /128 rules have no persisted input origin; expand them as singles.
-- Notes retain list order (newest first, ID ascending). Keep the 500-character
-- constraint: an oversized merged note or bigint hit overflow aborts ALL changes.
-- Merging is irreversible. A verified pre-upgrade database backup is required.
LOCK TABLE player_ip_block_policy, player_ip_block_rules IN EXCLUSIVE MODE;
CREATE TEMP TABLE i11_network_merge ON COMMIT DROP AS
WITH normalized AS (
 SELECT *, CASE WHEN masklen(network)=32 AND family(network)=4 THEN set_masklen(network,24)
                WHEN masklen(network)=128 AND family(network)=6 THEN set_masklen(network,64)
                ELSE network END AS target
 FROM player_ip_block_rules
)
SELECT target, (array_agg(id ORDER BY created_at,id))[1] AS survivor,
 COALESCE(string_agg(note,E'\n' ORDER BY created_at DESC,id) FILTER (WHERE note<>''),'') AS note,
 min(created_at) AS created_at,max(updated_at) AS updated_at,
 sum(hit_count)::bigint AS hit_count,max(last_hit_at) AS last_hit_at,
 count(*)>1 OR bool_or(network<>target) AS changed
FROM normalized GROUP BY target;
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM i11_network_merge WHERE char_length(note)>500) THEN
  RAISE EXCEPTION 'I11 merged IP blacklist note exceeds 500 characters; migration aborted without truncation';
 END IF;
END $$;
-- +goose StatementEnd
DELETE FROM player_ip_block_rules r USING i11_network_merge m
 WHERE (CASE WHEN masklen(r.network)=32 AND family(r.network)=4 THEN set_masklen(r.network,24)
             WHEN masklen(r.network)=128 AND family(r.network)=6 THEN set_masklen(r.network,64)
             ELSE r.network END)=m.target AND r.id<>m.survivor;
UPDATE player_ip_block_rules r SET network=m.target,note=m.note,created_at=m.created_at,
 updated_at=m.updated_at,hit_count=m.hit_count,last_hit_at=m.last_hit_at
 FROM i11_network_merge m WHERE r.id=m.survivor AND m.changed;
UPDATE player_ip_block_policy SET revision=revision+1
 WHERE singleton AND EXISTS(SELECT 1 FROM i11_network_merge WHERE changed);

-- +goose Down
-- Structure-only rollback: table/constraint shapes are unchanged. Never fabricate
-- lost singles/merged rows. Restore original data only from the verified backup.
SELECT 1;
