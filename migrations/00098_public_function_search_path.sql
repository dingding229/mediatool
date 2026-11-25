-- +goose Up
-- pg_restore intentionally emits an empty session search_path. Row triggers
-- must resolve application objects during a data-only import regardless of it.
ALTER FUNCTION public.native_queue_asset_assignment(uuid) SET search_path = public, pg_catalog;
ALTER FUNCTION public.native_queue_binding_assignment() SET search_path = public, pg_catalog;
ALTER FUNCTION public.native_queue_strm_assignment() SET search_path = public, pg_catalog;
ALTER FUNCTION public.native_queue_cloud_assignment() SET search_path = public, pg_catalog;
ALTER FUNCTION public.native_probe_fingerprint(bigint, text, jsonb) SET search_path = public, pg_catalog;

-- +goose Down
ALTER FUNCTION public.native_queue_asset_assignment(uuid) RESET search_path;
ALTER FUNCTION public.native_queue_binding_assignment() RESET search_path;
ALTER FUNCTION public.native_queue_strm_assignment() RESET search_path;
ALTER FUNCTION public.native_queue_cloud_assignment() RESET search_path;
ALTER FUNCTION public.native_probe_fingerprint(bigint, text, jsonb) RESET search_path;
