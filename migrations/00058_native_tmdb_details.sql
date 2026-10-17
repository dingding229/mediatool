-- +goose Up
-- Cached, normalized TMDB responses used by native media projections. The payload
-- is provider data only; it never contains a playback URL, cloud path or token.
CREATE TABLE native_tmdb_details (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 tmdb_type text NOT NULL CONSTRAINT native_tmdb_details_type_check CHECK (tmdb_type IN ('movie','series','season','episode')),
 tmdb_id bigint NOT NULL CHECK (tmdb_id > 0),
 season_number integer CHECK (season_number IS NULL OR season_number BETWEEN 0 AND 999),
 episode_number integer CHECK (episode_number IS NULL OR episode_number BETWEEN 1 AND 9999),
 language text NOT NULL DEFAULT 'zh-CN' CHECK (char_length(language) BETWEEN 2 AND 16 AND language !~ '[[:cntrl:]]'),
 payload jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(payload)='object' AND octet_length(payload::text) <= 4194304),
 state text NOT NULL DEFAULT 'pending' CONSTRAINT native_tmdb_details_state_check CHECK (state IN ('pending','fetching','ready','failed')),
 fetching_started_at timestamptz,
 attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0 AND attempts <= 100),
 next_attempt_at timestamptz NOT NULL DEFAULT now(),
 fetched_at timestamptz,
 error text NOT NULL DEFAULT '' CHECK (char_length(error) <= 8192),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 identity_key text GENERATED ALWAYS AS (tmdb_type||':'||tmdb_id::text||':'||coalesce(season_number,-1)::text||':'||coalesce(episode_number,-1)::text||':'||language) STORED UNIQUE,
 CHECK ((tmdb_type IN ('movie','series') AND season_number IS NULL AND episode_number IS NULL)
     OR (tmdb_type='season' AND season_number IS NOT NULL AND episode_number IS NULL)
     OR (tmdb_type='episode' AND season_number IS NOT NULL AND episode_number IS NOT NULL)),
 CHECK ((state='fetching')=(fetching_started_at IS NOT NULL)),
 CHECK ((state='ready' AND fetched_at IS NOT NULL) OR state<>'ready')
);
CREATE UNIQUE INDEX native_tmdb_details_identity ON native_tmdb_details(tmdb_type,tmdb_id,coalesce(season_number,-1),coalesce(episode_number,-1),language);
CREATE INDEX native_tmdb_details_due ON native_tmdb_details(state,next_attempt_at);
CREATE INDEX native_tmdb_details_tmdb_id ON native_tmdb_details(tmdb_type,tmdb_id);

-- +goose Down
DROP TABLE native_tmdb_details;
