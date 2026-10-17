package catalog

// ExposableAssetsSQL is the T12 evidence definition shared by catalog queries:
// confirmed binding, present cloud file and at least one valid STRM entry.
// It deliberately does not depend on user grants (those are applied separately).
const ExposableAssetsSQL = `SELECT a.id AS asset_id,a.cloud_file_id,a.revision,a.created_at,a.media_identity AS asset_identity,
 m.id AS media_id,m.tmdb_id,m.media_type,m.title,m.release_year,m.metadata,b.season,b.episode,b.episodes
 FROM media_assets a JOIN media_asset_bindings b ON b.asset_id=a.id AND b.state='ready'
 JOIN cloud_files f ON f.id=a.cloud_file_id AND f.state='present'
 JOIN media_items m ON m.id=b.media_id
 WHERE EXISTS(SELECT 1 FROM strm_entries s WHERE s.asset_id=a.id AND s.output_path<>'' AND lower(s.output_path) LIKE '%.strm')`
