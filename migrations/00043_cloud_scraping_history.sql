-- +goose Up
ALTER TABLE media_assets ADD COLUMN scrape_report jsonb NOT NULL DEFAULT '[]';
ALTER TABLE media_assets ADD COLUMN scrape_policy jsonb NOT NULL DEFAULT '{}';
ALTER TABLE media_assets ADD COLUMN scrape_settings_revision bigint NOT NULL DEFAULT 0;
ALTER TABLE managed_media_artifacts DROP CONSTRAINT managed_media_artifacts_artifact_kind_check;
ALTER TABLE managed_media_artifacts ADD CONSTRAINT managed_media_artifacts_artifact_kind_check CHECK(artifact_kind IN ('strm','nfo','subtitle','artwork'));
CREATE TABLE library_hidden_history (
 kind text NOT NULL,
 record_id uuid NOT NULL,
 observed_at timestamptz NOT NULL,
 hidden_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(kind,record_id)
);
-- Add options without altering actual media or output manifests.
UPDATE application_settings SET value=jsonb_set(value,'{scraping}','{"movie":{"nfo":true},"series":{"nfo":true},"season":{},"episode":{"nfo":true}}') WHERE NOT(value ? 'scraping');
UPDATE application_settings SET value=jsonb_set(value,'{movieTemplate}','"{title} ({year})/{title} ({year}) - {videoFormat}"') WHERE value->>'movieTemplate'='{title} ({year}) - {resolution}{ext}';
UPDATE application_settings SET value=jsonb_set(value,'{seriesTemplate}','"{title} ({year})/Season {season}/{title} - {season_episode} - 第{episode}集"') WHERE value->>'seriesTemplate'='{title} - S{season}E{episode} - {resolution}{ext}';
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM managed_media_artifacts WHERE artifact_kind='artwork') THEN RAISE EXCEPTION 'Owned artwork must remain protected; cannot downgrade'; END IF;
END $$;
-- +goose StatementEnd
DROP TABLE library_hidden_history;
ALTER TABLE media_assets DROP COLUMN scrape_report,DROP COLUMN scrape_policy,DROP COLUMN scrape_settings_revision;
ALTER TABLE managed_media_artifacts DROP CONSTRAINT managed_media_artifacts_artifact_kind_check;
ALTER TABLE managed_media_artifacts ADD CONSTRAINT managed_media_artifacts_artifact_kind_check CHECK(artifact_kind IN ('strm','nfo','subtitle'));
