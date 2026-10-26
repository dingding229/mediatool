-- +goose Up
-- Retire only the obsolete execution toggle; recovery interlocks stay intact.
DELETE FROM app_settings WHERE key='online_update.execution';
UPDATE app_settings SET value=value || '{"intervalHours":12}'::jsonb,updated_at=now()
WHERE key='system.update-check' AND NOT value ? 'intervalHours';

-- +goose Down
-- Old binaries default safely to execution disabled. Never acknowledge recovery.
INSERT INTO app_settings(key,value)VALUES('online_update.execution','false'::jsonb)
ON CONFLICT(key) DO NOTHING;
UPDATE app_settings SET value=value-'intervalHours',updated_at=now()
WHERE key='system.update-check';
