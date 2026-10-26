-- +goose Up
-- No disk/media/STRM mutation; retire only external Emby integration data.
ALTER TABLE plugin_v2_instances DROP CONSTRAINT plugin_v2_instances_reason_check;
ALTER TABLE plugin_v2_instances ADD CONSTRAINT plugin_v2_instances_reason_check CHECK(reason IN('','worker_failed','resource_limit','permission_review_required','definition_missing','definition_unsupported','data_unavailable','external_media_removed'));
CREATE TABLE external_emby_retirement (
 task_id uuid PRIMARY KEY REFERENCES download_tasks(id) ON DELETE CASCADE, previous_state text NOT NULL,
 completed boolean NOT NULL, retired_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO external_emby_retirement(task_id,previous_state,completed)
 SELECT d.id,d.download_state,(EXISTS (SELECT 1 FROM download_files f WHERE f.task_id=d.id AND f.media_state='imported')
 AND NOT EXISTS (SELECT 1 FROM download_files f WHERE f.task_id=d.id AND f.media_state NOT IN ('imported','non_media','sample'))
 AND NOT EXISTS (SELECT 1 FROM download_files f WHERE f.task_id=d.id AND f.media_state='imported'
  AND NOT EXISTS (SELECT 1 FROM transfer_tasks t
   JOIN media_asset_bindings b ON b.download_file_id=f.id
   JOIN media_assets a ON a.id=b.asset_id
   JOIN cloud_files cf ON cf.id=a.cloud_file_id
   JOIN strm_entries s ON s.asset_id=a.id
   JOIN managed_media_artifacts m ON m.asset_id=a.id AND m.relative_path=s.output_path AND m.artifact_kind='strm' AND m.owner_scope='asset:'||a.id::text
   WHERE t.download_file_id=f.id AND t.state='imported' AND a.artifacts_state='ready' AND cf.state='present'))) FROM download_tasks d
 WHERE d.download_state IN ('strm_ready','refresh_requested');
UPDATE download_tasks d SET download_state='imported',updated_at=now()
 FROM external_emby_retirement r WHERE r.task_id=d.id AND r.completed;
UPDATE jobs SET state='cancelled',lease_token=NULL,lease_until=NULL,locked_by=NULL,
 last_error='外部 Emby 对接已移除；未完成入库需核对本程序产物',updated_at=now()
 WHERE kind IN ('emby.refresh','emby.confirm','library.emby.confirm','library.emby.refresh')
 AND state NOT IN ('succeeded','cancelled');
-- library.refresh formerly refreshed the external server. Do not replay old jobs.
UPDATE jobs SET state='cancelled',lease_token=NULL,lease_until=NULL,locked_by=NULL,
 last_error='外部 Emby 刷新已移除',updated_at=now()
 WHERE kind='library.refresh' AND state NOT IN ('succeeded','cancelled');
DROP TABLE library_emby_checks;
DROP TABLE emby_item_mappings;
DELETE FROM integrations WHERE kind='emby';
ALTER TABLE integrations DROP CONSTRAINT integrations_kind_check;
ALTER TABLE integrations ADD CONSTRAINT integrations_kind_check CHECK(kind IN ('clouddrive2','open115','local','qbittorrent','tmdb','telegram','ptsite'));
UPDATE application_settings SET value=(value-'embyUserId') #- '{access,embyUrl}',revision=revision+1,updated_at=now()
 WHERE value ? 'embyUserId' OR value#>'{access,embyUrl}' IS NOT NULL;
ALTER TABLE player_access_events DROP COLUMN emby_user_id,DROP COLUMN emby_user_name,DROP COLUMN emby_verified;

-- +goose Down
UPDATE plugin_v2_instances SET reason='definition_unsupported' WHERE reason='external_media_removed';
ALTER TABLE plugin_v2_instances DROP CONSTRAINT plugin_v2_instances_reason_check;
ALTER TABLE plugin_v2_instances ADD CONSTRAINT plugin_v2_instances_reason_check CHECK(reason IN('','worker_failed','resource_limit','permission_review_required','definition_missing','definition_unsupported','data_unavailable'));
-- Schema rollback only: erased external credentials/identities are never restored.
ALTER TABLE integrations DROP CONSTRAINT integrations_kind_check;
ALTER TABLE integrations ADD CONSTRAINT integrations_kind_check CHECK(kind IN ('clouddrive2','open115','local','qbittorrent','emby','tmdb','telegram','ptsite'));
CREATE TABLE emby_item_mappings (
 emby_instance text NOT NULL,item_id text NOT NULL,media_source_id text NOT NULL,
 asset_id uuid NOT NULL REFERENCES media_assets(id),PRIMARY KEY(emby_instance,item_id,media_source_id)
);
CREATE TABLE library_emby_checks (
 asset_id uuid PRIMARY KEY REFERENCES media_assets(id) ON DELETE CASCADE,
 integration_id uuid NOT NULL REFERENCES integrations(id),asset_revision bigint NOT NULL,
 state text NOT NULL,message text NOT NULL,paths jsonb NOT NULL,checked_at timestamptz NOT NULL DEFAULT now(),
 integration_revision bigint NOT NULL DEFAULT 0,user_id text NOT NULL DEFAULT ''
);
ALTER TABLE player_access_events ADD COLUMN emby_user_id text NOT NULL DEFAULT '',ADD COLUMN emby_user_name text NOT NULL DEFAULT '',ADD COLUMN emby_verified boolean NOT NULL DEFAULT false;
CREATE INDEX player_access_events_user ON player_access_events(emby_user_id,created_at DESC) WHERE emby_verified;
DROP TABLE external_emby_retirement;
