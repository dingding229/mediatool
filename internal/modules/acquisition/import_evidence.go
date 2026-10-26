package acquisition

// ImportEvidenceSQL is evaluated with a download_tasks alias d. File/import flags
// alone are insufficient: every media file needs the owned native STRM artifact.
const ImportEvidenceSQL = `EXISTS (SELECT 1 FROM download_files f WHERE f.task_id=d.id AND f.media_state='imported')
 AND NOT EXISTS (SELECT 1 FROM download_files f WHERE f.task_id=d.id AND f.media_state NOT IN ('imported','non_media','sample'))
 AND NOT EXISTS (SELECT 1 FROM download_files f WHERE f.task_id=d.id AND f.media_state='imported'
  AND NOT EXISTS (SELECT 1 FROM transfer_tasks t
   JOIN media_asset_bindings b ON b.download_file_id=f.id
   JOIN media_assets a ON a.id=b.asset_id
   JOIN cloud_files cf ON cf.id=a.cloud_file_id
   JOIN strm_entries s ON s.asset_id=a.id
   JOIN managed_media_artifacts m ON m.asset_id=a.id AND m.relative_path=s.output_path AND m.artifact_kind='strm' AND m.owner_scope='asset:'||a.id::text
   WHERE t.download_file_id=f.id AND t.state='imported' AND a.artifacts_state='ready' AND cf.state='present'))`
