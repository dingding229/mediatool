-- +goose Up
SET LOCAL lock_timeout='5s';
CREATE TABLE media_image_files (
 id uuid PRIMARY KEY, digest text NOT NULL CHECK(digest ~ '^[0-9a-f]{64}$'),
 filename text NOT NULL UNIQUE CHECK(filename ~ '^[0-9a-f-]{36}\.(jpg|png)$'),
 content_type text NOT NULL CHECK(content_type IN('image/jpeg','image/png')),
 width int NOT NULL CHECK(width BETWEEN 1 AND 8192), height int NOT NULL CHECK(height BETWEEN 1 AND 8192),
 size_bytes bigint NOT NULL CHECK(size_bytes BETWEEN 1 AND 10485760),
 source text NOT NULL CHECK(source IN('upload','tmdb')), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE media_image_overrides (
 media_id uuid NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
 entity_type text NOT NULL CHECK(entity_type IN('movie','series','season','episode')),
 season_number int NOT NULL DEFAULT -1, episode_number int NOT NULL DEFAULT -1,
 images jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(images)='object'),
 revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0), updated_at timestamptz NOT NULL DEFAULT now(), updated_by text NOT NULL DEFAULT '',
 PRIMARY KEY(media_id,entity_type,season_number,episode_number),
 CHECK((entity_type IN('movie','series') AND season_number=-1 AND episode_number=-1) OR
 (entity_type='season' AND season_number BETWEEN 0 AND 10000 AND episode_number=-1) OR
 (entity_type='episode' AND season_number BETWEEN 0 AND 10000 AND episode_number BETWEEN 1 AND 10000))
);
CREATE INDEX media_image_overrides_images ON media_image_overrides USING gin(images jsonb_path_ops);
-- +goose Down
DROP TABLE media_image_overrides;
DROP TABLE media_image_files;
