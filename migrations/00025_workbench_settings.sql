-- +goose Up
CREATE TABLE application_settings (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 value jsonb NOT NULL DEFAULT '{}'::jsonb,
 revision bigint NOT NULL DEFAULT 1,
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO application_settings(singleton)VALUES(true);
ALTER TABLE administrator_accounts ADD COLUMN must_change_password boolean NOT NULL DEFAULT false;
ALTER TABLE administrator_accounts ADD COLUMN temporary_password_expires_at timestamptz;
CREATE TABLE administrator_bootstrap_resets (
 marker_digest text PRIMARY KEY,
 account_id uuid NOT NULL REFERENCES administrator_accounts(id) ON DELETE CASCADE,
 applied_at timestamptz NOT NULL DEFAULT now()
);
-- Obsolete web recovery codes cannot be replayed after this upgrade.
DELETE FROM administrator_recovery_codes;
CREATE TABLE extension_plugins (
 id text PRIMARY KEY CHECK(id ~ '^[a-z][a-z0-9-]{2,63}$'),
 name text NOT NULL,
 version text NOT NULL,
 enabled boolean NOT NULL DEFAULT false,
 manifest jsonb NOT NULL,
 config jsonb NOT NULL DEFAULT '{}'::jsonb,
 revision bigint NOT NULL DEFAULT 1,
 updated_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
-- Explicit downgrade only; never silently restore a temporary-password account.
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM administrator_accounts WHERE must_change_password) THEN
  RAISE EXCEPTION 'resolve mandatory password reset before downgrade';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE extension_plugins;
DROP TABLE administrator_bootstrap_resets;
ALTER TABLE administrator_accounts DROP COLUMN must_change_password,DROP COLUMN temporary_password_expires_at;
DROP TABLE application_settings;
