-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
-- This dedicated table permits only id='cz88-community' (migration 79 CHECK).
-- Fail closed rather than erase an unexpected provider/key. No other table changes.
-- +goose StatementBegin
DO $$
BEGIN
 IF to_regclass('ip_geo_settings') IS NOT NULL THEN
  IF EXISTS(SELECT 1 FROM ip_geo_settings WHERE id <> 'cz88-community') THEN
   RAISE EXCEPTION 'unexpected IP provider settings; refusing removal';
  END IF;
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE IF EXISTS ip_geo_settings;

-- +goose Down
-- Recreate only the empty historical schema; erased authorization is not restored.
CREATE TABLE ip_geo_settings(id text PRIMARY KEY DEFAULT 'cz88-community' CHECK(id='cz88-community'),singleton boolean UNIQUE NOT NULL DEFAULT true CHECK(singleton),encrypted_credentials bytea NOT NULL,updated_at timestamptz NOT NULL DEFAULT now());
