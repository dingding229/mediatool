-- +goose Up
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='60s';
-- Fence all old writers BEFORE inspecting emptiness. No DELETE/TRUNCATE/CASCADE.
LOCK TABLE plugin_legacy_wasm_plugins,plugin_legacy_wasm_deliveries,plugin_legacy_wasm_runs,plugin_legacy_wasm_repositories,plugin_legacy_wasm_jobs,plugin_legacy_wasm_service_ticks,site_accounts,notification_channels,bootstrap_imports,media_user_legacy_grants,notification_deliveries,download_tasks,site_definitions IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
DECLARE target text; present boolean;
BEGIN
 FOREACH target IN ARRAY ARRAY['plugin_legacy_wasm_plugins','plugin_legacy_wasm_deliveries','plugin_legacy_wasm_runs','plugin_legacy_wasm_repositories','plugin_legacy_wasm_jobs','plugin_legacy_wasm_service_ticks','site_accounts','notification_channels','bootstrap_imports','media_user_legacy_grants'] LOOP
  IF EXISTS(SELECT FROM pg_class WHERE oid=to_regclass(target)) THEN
   EXECUTE format('SELECT EXISTS(SELECT FROM %I LIMIT 1)',target) INTO STRICT present;
   IF present THEN RAISE EXCEPTION 'legacy retirement blocked: % is not empty; restore/inspect backup, no automatic deletion',target; END IF;
  END IF;
 END LOOP;
 IF EXISTS(SELECT FROM download_tasks WHERE site_account_id IS NOT NULL) OR EXISTS(SELECT FROM notification_deliveries WHERE channel_id IS NOT NULL) THEN
  RAISE EXCEPTION 'legacy retirement blocked: reference columns are not NULL';
 END IF;
 -- Historical local_overrides is NOT NULL DEFAULT {}. Only that empty object
 -- is absence; a JSON null, array, or any configured object must abort.
 IF EXISTS(SELECT FROM site_definitions WHERE local_overrides IS DISTINCT FROM '{}'::jsonb) THEN
  RAISE EXCEPTION 'legacy retirement blocked: site local overrides contain data';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE notification_deliveries DROP COLUMN channel_id;
ALTER TABLE download_tasks DROP COLUMN site_account_id;
ALTER TABLE site_definitions DROP COLUMN local_overrides;
DROP TABLE plugin_legacy_wasm_deliveries,plugin_legacy_wasm_runs,plugin_legacy_wasm_jobs,plugin_legacy_wasm_service_ticks,plugin_legacy_wasm_plugins,plugin_legacy_wasm_repositories,site_accounts,notification_channels,bootstrap_imports,media_user_legacy_grants;
-- +goose Down
-- Schema-only downgrade would disguise discarded contracts. Restore the full
-- pre-upgrade database + binary backup instead, keeping key/AAD/STRM together.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'legacy retirement is irreversible; restore the complete pre-upgrade backup'; END $$;
-- +goose StatementEnd
