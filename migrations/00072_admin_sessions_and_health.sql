-- +goose Up
ALTER TABLE administrator_sessions ADD COLUMN id uuid NOT NULL DEFAULT gen_random_uuid(), ADD COLUMN last_active_at timestamptz, ADD COLUMN ip text NOT NULL DEFAULT '', ADD COLUMN user_agent text NOT NULL DEFAULT '';
-- Existing sessions have no historical activity telemetry: creation is the
-- earliest confirmed activity, not a fabricated migration-time login.
UPDATE administrator_sessions SET last_active_at=created_at;
ALTER TABLE administrator_sessions ALTER COLUMN last_active_at SET NOT NULL,ALTER COLUMN last_active_at SET DEFAULT now();
CREATE UNIQUE INDEX administrator_sessions_id ON administrator_sessions(id);
CREATE TABLE integration_health_schedule(integration_id uuid PRIMARY KEY REFERENCES integrations(id) ON DELETE CASCADE,next_check_at timestamptz NOT NULL DEFAULT now()+interval '5 minutes',lease_token uuid,lease_until timestamptz,failures integer NOT NULL DEFAULT 0,config_revision bigint NOT NULL DEFAULT 0);
-- +goose Down
DROP TABLE integration_health_schedule;
ALTER TABLE administrator_sessions DROP COLUMN id,DROP COLUMN last_active_at,DROP COLUMN ip,DROP COLUMN user_agent;
