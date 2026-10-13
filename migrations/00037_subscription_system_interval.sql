-- +goose Up
UPDATE application_settings SET value=jsonb_set(value,'{subscriptionIntervalMinutes}','30'::jsonb),revision=revision+1,updated_at=now() WHERE singleton AND NOT(value ? 'subscriptionIntervalMinutes');
-- The owner requested removal of these constraints and one global interval.
-- Keep identities, enabled states, keyword semantics, coverage and existing downloads.
UPDATE subscriptions SET
 rules=rules||'{"minSize":0,"maxSize":0,"minSeeders":0}'::jsonb,
 interval_seconds=((SELECT value->>'subscriptionIntervalMinutes' FROM application_settings WHERE singleton)::integer)*60,
 revision=revision+1,
 next_run_at=CASE WHEN enabled THEN now()+make_interval(mins=>(SELECT (value->>'subscriptionIntervalMinutes')::integer FROM application_settings WHERE singleton)) ELSE next_run_at END,
 updated_at=now();
-- +goose Down
-- Removed per-subscription limits cannot be reconstructed; rollback needs the private backup.
UPDATE application_settings SET value=value-'subscriptionIntervalMinutes',revision=revision+1,updated_at=now() WHERE singleton;
