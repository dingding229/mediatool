-- +goose Up
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='60s';
-- Probe stream indices are bounded to 65535. External indices never overlap,
-- never get recycled, and remain stable across hide/delete/list-order changes.
CREATE SEQUENCE managed_subtitle_index AS integer MINVALUE 65536 MAXVALUE 2147483647 START 65536;
CREATE TABLE managed_subtitles (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 asset_id uuid NOT NULL REFERENCES media_assets(id) ON DELETE CASCADE,
 source text NOT NULL CHECK(source IN ('automatic','embedded','uploaded')),
 source_key text NOT NULL,
 stream_index integer NOT NULL DEFAULT nextval('managed_subtitle_index'),
 filename text NOT NULL CHECK(length(filename)<=256),
 extension text NOT NULL,
 storage_path text NOT NULL DEFAULT '',
 language text NOT NULL DEFAULT '' CHECK(length(language)<=16),
 title text NOT NULL DEFAULT '' CHECK(length(title)<=200),
 is_default boolean NOT NULL DEFAULT false,
 is_forced boolean NOT NULL DEFAULT false,
 hidden boolean NOT NULL DEFAULT false,
 deleting boolean NOT NULL DEFAULT false,
 overridden boolean NOT NULL DEFAULT false,
 digest bytea,
 size_bytes bigint NOT NULL DEFAULT 0 CHECK(size_bytes BETWEEN 0 AND 5242880),
 revision bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(asset_id,source,source_key), UNIQUE(asset_id,stream_index),
 CHECK((source='uploaded')=(storage_path<>'')),
 CHECK(source<>'uploaded' OR extension IN ('srt','ass','ssa','vtt')),
 CHECK(source<>'uploaded' OR (digest IS NOT NULL AND octet_length(digest)=32 AND size_bytes>0)),
 CHECK(source<>'embedded' OR NOT hidden)
);
-- +goose Down
DROP TABLE managed_subtitles;
DROP SEQUENCE managed_subtitle_index;
