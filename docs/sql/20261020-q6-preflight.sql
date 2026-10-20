-- Q6 发布前阻断预检；只读。必须从本项目操作者私有配置取得连接。
-- psql "$APP_DATABASE_URL" -v ON_ERROR_STOP=1 -v expected_database=media_tool -f docs/sql/20261020-q6-preflight.sql
-- 缺少 expected_database、版本不为65或任一断言失败，禁止启动新镜像/迁移。
BEGIN TRANSACTION READ ONLY;
SET LOCAL lock_timeout='3s';
SET LOCAL statement_timeout='30s';
SELECT set_config('app.preflight_expected_database', :'expected_database', true);
DO $preflight$
DECLARE version bigint; target text; populated boolean;
BEGIN
 IF current_database()<>current_setting('app.preflight_expected_database') THEN
  RAISE EXCEPTION 'Q6 preflight: unexpected database';
 END IF;
 SELECT max(version_id) INTO version FROM
  (SELECT DISTINCT ON(version_id) version_id,is_applied FROM public.goose_db_version ORDER BY version_id,id DESC) v
  WHERE is_applied;
 IF version IS DISTINCT FROM 65 THEN RAISE EXCEPTION 'Q6 preflight: expected schema65, found %',version; END IF;
 FOREACH target IN ARRAY ARRAY[
  'plugin_legacy_wasm_plugins','plugin_legacy_wasm_deliveries','plugin_legacy_wasm_runs',
  'plugin_legacy_wasm_repositories','plugin_legacy_wasm_jobs','plugin_legacy_wasm_service_ticks',
  'site_accounts','notification_channels','bootstrap_imports','media_user_legacy_grants'] LOOP
  EXECUTE format('SELECT EXISTS(SELECT FROM public.%I LIMIT 1)',target) INTO populated;
  IF populated THEN RAISE EXCEPTION 'Q6 preflight: legacy table % is not empty',target; END IF;
 END LOOP;
 IF EXISTS(SELECT FROM public.download_tasks WHERE site_account_id IS NOT NULL)
 OR EXISTS(SELECT FROM public.notification_deliveries WHERE channel_id IS NOT NULL)
 OR EXISTS(SELECT FROM public.site_definitions WHERE local_overrides IS DISTINCT FROM '{}'::jsonb) THEN
  RAISE EXCEPTION 'Q6 preflight: legacy reference/override contains data';
 END IF;
 IF EXISTS(SELECT FROM public.plugin_v2_state_tasks t LEFT JOIN public.plugin_v2_state_snapshots s ON s.id=t.snapshot_id
  WHERE t.revision<=0 OR t.fingerprint !~ '^[a-f0-9]{64}$' OR jsonb_typeof(t.result)<>'object'
   OR s.id IS NULL OR s.instance_id<>t.instance_id) THEN
  RAISE EXCEPTION 'Q6 preflight: state task revision/fingerprint/JSON/ownership invalid';
 END IF;
 IF EXISTS(SELECT FROM public.search_runs WHERE jsonb_typeof(account_ids)<>'array')
 OR EXISTS(SELECT FROM public.download_tasks WHERE jsonb_typeof(details)<>'object')
 OR EXISTS(SELECT FROM public.subscriptions WHERE jsonb_typeof(identity)<>'object' OR jsonb_typeof(rules)<>'object' OR jsonb_typeof(coverage)<>'object') THEN
  RAISE EXCEPTION 'Q6 preflight: JSON shape violates migration67';
 END IF;
END
$preflight$;
-- Only counts/boolean observations; never ciphertext, credentials, paths or URL values.
SELECT current_database() AS database_name,
 (SELECT count(*) FROM public.plugin_v2_state_tasks) AS state_tasks,
 (SELECT count(*) FROM public.plugin_v2_state_tasks t JOIN public.plugin_v2_state_snapshots s ON s.id=t.snapshot_id AND s.instance_id=t.instance_id) AS owned_state_tasks,
 current_setting('transaction_read_only') AS read_only;
ROLLBACK;
