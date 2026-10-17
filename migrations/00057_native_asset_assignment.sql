-- +goose Up
-- T12 fallback libraries are reserved IDs, never derived from user group IDs.
ALTER TABLE native_libraries ALTER COLUMN group_id DROP NOT NULL;
ALTER TABLE native_libraries DROP CONSTRAINT IF EXISTS native_libraries_id_check;
ALTER TABLE native_libraries ADD CONSTRAINT native_libraries_id_check CHECK (
 (id IN ('f:movie','f:series') AND group_id IS NULL AND rule_id IS NULL)
 OR (group_id IS NOT NULL AND id=CASE WHEN rule_id IS NULL THEN 'g:'||group_id ELSE 'r:'||group_id||':'||rule_id END)
);
INSERT INTO native_item_ids(entity_type,internal_key) VALUES ('library','f:movie'),('library','f:series') ON CONFLICT(entity_type,internal_key) DO NOTHING;
INSERT INTO native_libraries(id,item_id,name,directory,collection_type,hidden,sort_order,policy_revision)
 SELECT 'f:movie',id,'未分类电影','电影','movies',true,2147483000,1 FROM native_item_ids WHERE entity_type='library' AND internal_key='f:movie'
 ON CONFLICT(id) DO NOTHING;
INSERT INTO native_libraries(id,item_id,name,directory,collection_type,hidden,sort_order,policy_revision)
 SELECT 'f:series',id,'未分类剧集','电视剧','tvshows',true,2147483001,1 FROM native_item_ids WHERE entity_type='library' AND internal_key='f:series'
 ON CONFLICT(id) DO NOTHING;
ALTER TABLE native_asset_libraries ADD COLUMN diagnostic jsonb NOT NULL DEFAULT '{}';

-- BEFORE DELETE is intentional: the parent cloud row may cascade-delete its media asset;
-- the trigger must read the asset association before that cascade completes.
-- +goose StatementBegin
CREATE FUNCTION native_queue_asset_assignment(asset_key uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE asset_revision bigint;
BEGIN
 SELECT revision INTO asset_revision FROM media_assets WHERE id=asset_key;
 IF asset_revision IS NULL THEN RETURN; END IF;
 INSERT INTO jobs(id,kind,queue,payload,idempotency_key,max_attempts)
 VALUES(gen_random_uuid(),'native.assign','library',jsonb_build_object('assetId',asset_key::text,'assetRevision',asset_revision), 'native.assign:'||asset_key::text,5)
 ON CONFLICT(idempotency_key) WHERE idempotency_key IS NOT NULL DO UPDATE
 SET payload=CASE WHEN jobs.state='running'
                  THEN jsonb_set(jsonb_set(jobs.payload,'{assetRevision}',to_jsonb(asset_revision)),'{rerun}','true'::jsonb)
                  ELSE EXCLUDED.payload END,
     state=CASE WHEN jobs.state IN('succeeded','failed','retry') THEN 'pending' ELSE jobs.state END,
     attempts=CASE WHEN jobs.state IN('succeeded','failed','retry') THEN 0 ELSE jobs.attempts END,
     available_at=CASE WHEN jobs.state IN('succeeded','failed','retry') THEN now() ELSE jobs.available_at END,
     last_error=CASE WHEN jobs.state IN('succeeded','failed','retry') THEN NULL ELSE jobs.last_error END;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION native_queue_binding_assignment() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN PERFORM native_queue_asset_assignment(OLD.asset_id); RETURN OLD; END IF;
 PERFORM native_queue_asset_assignment(NEW.asset_id); RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION native_queue_strm_assignment() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN PERFORM native_queue_asset_assignment(OLD.asset_id); RETURN OLD; END IF;
 PERFORM native_queue_asset_assignment(NEW.asset_id); RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION native_queue_cloud_assignment() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE asset_key uuid;
BEGIN
 SELECT id INTO asset_key FROM media_assets WHERE cloud_file_id=CASE WHEN TG_OP='DELETE' THEN OLD.id ELSE NEW.id END LIMIT 1;
 IF asset_key IS NOT NULL THEN PERFORM native_queue_asset_assignment(asset_key); END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF; RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER native_asset_binding_assignment AFTER INSERT OR UPDATE OR DELETE ON media_asset_bindings FOR EACH ROW EXECUTE FUNCTION native_queue_binding_assignment();
CREATE TRIGGER native_strm_entry_assignment AFTER INSERT OR UPDATE OF output_path OR DELETE ON strm_entries FOR EACH ROW EXECUTE FUNCTION native_queue_strm_assignment();
CREATE TRIGGER native_cloud_file_assignment BEFORE UPDATE OF state OR DELETE ON cloud_files FOR EACH ROW EXECUTE FUNCTION native_queue_cloud_assignment();

-- +goose Down
DROP TRIGGER IF EXISTS native_cloud_file_assignment ON cloud_files;
DROP TRIGGER IF EXISTS native_strm_entry_assignment ON strm_entries;
DROP TRIGGER IF EXISTS native_asset_binding_assignment ON media_asset_bindings;
DROP FUNCTION IF EXISTS native_queue_cloud_assignment();
DROP FUNCTION IF EXISTS native_queue_strm_assignment();
DROP FUNCTION IF EXISTS native_queue_binding_assignment();
DROP FUNCTION IF EXISTS native_queue_asset_assignment(uuid);
DELETE FROM native_asset_libraries WHERE library_id IN ('f:movie','f:series');
DELETE FROM native_libraries WHERE id IN ('f:movie','f:series');
DELETE FROM native_item_ids WHERE entity_type='library' AND internal_key IN ('f:movie','f:series');
ALTER TABLE native_asset_libraries DROP COLUMN diagnostic;
ALTER TABLE native_libraries DROP CONSTRAINT IF EXISTS native_libraries_id_check;
ALTER TABLE native_libraries ADD CONSTRAINT native_libraries_id_check CHECK (id=CASE WHEN rule_id IS NULL THEN 'g:'||group_id ELSE 'r:'||group_id||':'||rule_id END);
ALTER TABLE native_libraries ALTER COLUMN group_id SET NOT NULL;
