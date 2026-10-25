-- +goose Up
SET LOCAL lock_timeout='5s';
CREATE TABLE media_metadata_overrides (
 media_id uuid NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
 entity_type text NOT NULL CHECK(entity_type IN ('movie','series','season','episode')),
 season_number integer NOT NULL DEFAULT -1,
 episode_number integer NOT NULL DEFAULT -1,
 overrides jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(overrides)='object'),
 revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0),
 updated_at timestamptz NOT NULL DEFAULT now(),
 updated_by text NOT NULL DEFAULT '',
 PRIMARY KEY(media_id,entity_type,season_number,episode_number),
 CHECK((entity_type IN ('movie','series') AND season_number=-1 AND episode_number=-1)
 OR (entity_type='season' AND season_number BETWEEN 0 AND 10000 AND episode_number=-1)
 OR (entity_type='episode' AND season_number BETWEEN 0 AND 10000 AND episode_number BETWEEN 1 AND 10000))
);
-- +goose Down
DROP TABLE media_metadata_overrides;
