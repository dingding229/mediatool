package library

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/storage"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
)

const scanBatchSize = 128
const scanBatchBytes = 256 << 10

// Fast path is deliberately ONLY for new paths/provider IDs. Existing files,
// moves, replacements and ambiguous identities retain indexFile's full checks.
// The scan and connection SHARE locks fence every COPY against pause/generation
// or credential changes; no network/file side effect is inside the transaction.
func (e Engine) indexNewBatch(ctx context.Context, scan string, generation int64, connection string, revision int64, files []storage.File) (bool, error) {
	if len(files) == 0 {
		return true, nil
	}
	paths, providers := []string{}, []string{}
	seenPath, seenProvider := map[string]bool{}, map[string]bool{}
	for _, f := range files {
		if seenPath[f.Path] || f.ID != "" && seenProvider[f.ID] {
			return false, nil
		}
		seenPath[f.Path] = true
		paths = append(paths, f.Path)
		if f.ID != "" {
			seenProvider[f.ID] = true
			providers = append(providers, f.ID)
		}
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer deferutil.Rollback(ctx, tx)
	var state string
	var gen, current int64
	var enabled bool
	if err = tx.QueryRow(ctx, `SELECT s.state,s.generation,i.revision,i.enabled FROM library_scans s JOIN integrations i ON i.id=s.integration_id WHERE s.id=$1 FOR SHARE OF s,i`, scan).Scan(&state, &gen, &current, &enabled); err != nil {
		return false, err
	}
	if state != "running" || gen != generation {
		return false, jobs.ErrOperatorRequired
	}
	if current != revision || !enabled {
		return false, errors.New("scan connection revision changed; restart with fresh coverage")
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_files WHERE storage_id=$1 AND (full_path=ANY($2::text[]) OR provider_file_id<>'' AND md5(provider_file_id)=ANY(ARRAY(SELECT md5(v)FROM unnest($3::text[])v)) AND provider_file_id=ANY($3::text[])))`, connection, paths, providers).Scan(&exists); err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	var storageID, scanID pgtype.UUID
	if err = storageID.Scan(connection); err != nil {
		return false, err
	}
	if err = scanID.Scan(scan); err != nil {
		return false, err
	}
	rows, seen, candidates := [][]any{}, [][]any{}, [][]any{}
	for _, f := range files {
		var id pgtype.UUID
		if err = id.Scan(jobs.UUID()); err != nil {
			return false, err
		}
		raw, err := json.Marshal(f.Hashes)
		if err != nil {
			return false, err
		}
		rows = append(rows, []any{id, storageID, f.ID, f.Path, f.Size, raw})
		seen = append(seen, []any{scanID, id})
		if metadata.IsVideo(f.Name) && !metadata.IsSample(f.Path) {
			var candidate pgtype.UUID
			if err = candidate.Scan(jobs.UUID()); err != nil {
				return false, err
			}
			candidates = append(candidates, []any{candidate, id})
		}
	}
	if _, err = tx.CopyFrom(ctx, pgx.Identifier{"cloud_files"}, []string{"id", "storage_id", "provider_file_id", "full_path", "size_bytes", "metadata"}, pgx.CopyFromRows(rows)); err != nil {
		// Another writer may have committed one of these paths after our read.
		var conflict *pgconn.PgError
		if errors.As(err, &conflict) && conflict.Code == "23505" {
			return false, nil
		}
		return false, err
	}
	if _, err = tx.CopyFrom(ctx, pgx.Identifier{"library_scan_seen"}, []string{"scan_id", "file_id"}, pgx.CopyFromRows(seen)); err != nil {
		return false, err
	}
	if len(candidates) > 0 {
		if _, err = tx.CopyFrom(ctx, pgx.Identifier{"library_candidates"}, []string{"id", "file_id"}, pgx.CopyFromRows(candidates)); err != nil {
			return false, err
		}
	}
	if err = dbutil.Commit(ctx, tx); err != nil {
		return false, err
	}
	return true, nil
}

func scanMetadataBytes(file storage.File) int {
	n := len(file.ID) + len(file.Name) + len(file.Path) + 128
	for _, value := range file.Hashes {
		n += len(value) + 16
	}
	return n
}
