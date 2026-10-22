package library

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/trace"
)

type Engine struct {
	DB          *pgxpool.Pool
	Connections *integrations.Manager
	Observe     func(context.Context, string, string) (storage.File, error)
}

type scanCountQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func scanCounts(ctx context.Context, q scanCountQueryer, scanID string) (int, int, error) {
	var dirs, files int
	if err := q.QueryRow(ctx, `SELECT count(*)FROM library_scan_directories WHERE scan_id=$1`, scanID).Scan(&dirs); err != nil {
		return 0, 0, err
	}
	if err := q.QueryRow(ctx, `SELECT count(*)FROM library_scan_seen WHERE scan_id=$1`, scanID).Scan(&files); err != nil {
		return 0, 0, err
	}
	return dirs, files, nil
}

type Scan struct {
	ID                   string    `json:"id"`
	ConnectionID         string    `json:"connectionId"`
	Root                 string    `json:"root"`
	State                string    `json:"state"`
	Files                int       `json:"files"`
	Directories          int       `json:"directories"`
	Message              string    `json:"message"`
	CompletedDirectories int       `json:"completedDirectories"`
	CurrentDirectory     string    `json:"currentDirectory"`
	Revision             int64     `json:"revision"`
	Generation           int64     `json:"generation"`
	UpdatedAt            time.Time `json:"updatedAt"`
	CreatedAt            time.Time `json:"createdAt"`
}

func inside(root, p string) bool {
	return p == root || strings.HasPrefix(p, strings.TrimRight(root, "/")+"/")
}
func (e Engine) Start(ctx context.Context, root string) (string, error) {
	connection, err := e.Connections.Repository.Default(ctx, "clouddrive2")
	if err != nil {
		return "", errors.New("configure a primary CloudDrive2 connection")
	}
	allowed := connection.String("cloudRoot")
	if allowed == "" {
		if len(e.Connections.Config.STRMAllowedRoots) == 0 {
			return "", errors.New("configure an allowed scan root")
		}
		allowed = e.Connections.Config.STRMAllowedRoots[0]
	}
	allowed = path.Clean(allowed)
	if root == "" {
		root = allowed
	}
	if !strings.HasPrefix(root, "/") || path.Clean(root) != root || strings.Contains(root, "\\") || !inside(allowed, root) {
		return "", errors.New("scan root must be canonical and within the configured cloud directory")
	}
	ctx, selected, err := (pluginv2.Store{DB: e.DB, Vault: e.Connections.Repository.Vault}).SelectStorage(ctx, contracts.StorageSelection{Handles: []string{connection.Record.ID}, Selected: connection.Record.ID})
	if err != nil || selected != connection.Record.ID {
		return "", errors.New("存储选择策略无效")
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer deferutil.Rollback(ctx, tx)
	id, _, err := startScanTx(ctx, tx, connection.Record.ID, root)
	if err != nil {
		return "", err
	}
	return id, dbutil.Commit(ctx, tx)
}

func startScanTx(ctx context.Context, tx pgx.Tx, connectionID, root string) (string, bool, error) {
	if strings.Contains("/"+root+"/", "/.media-tool-trash/") {
		return "", false, errors.New("the file-management recycle directory is excluded from library scans")
	}
	// Serialize scan admission with file-operation intent, not the remote RPC.
	var locked string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM integrations WHERE id=$1 FOR SHARE`, connectionID).Scan(&locked); err != nil {
		return "", false, err
	}
	var fileBusy bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_operations WHERE integration_id=$1 AND state IN ('running','needs_reconcile'))`, connectionID).Scan(&fileBusy); err != nil {
		return "", false, err
	}
	if fileBusy {
		return "", false, errors.New("reconcile the active file operation before scanning")
	}

	id := jobs.UUID()
	tag, err := tx.Exec(ctx, `INSERT INTO library_scans(id,integration_id,root,integration_revision)SELECT $1,$2,$3,revision FROM integrations WHERE id=$2 ON CONFLICT(integration_id,root)WHERE state IN ('pending','running','paused')DO NOTHING`, id, connectionID, root)
	if err != nil {
		return "", false, err
	}
	if tag.RowsAffected() == 0 {
		if err = tx.QueryRow(ctx, `SELECT id::text FROM library_scans WHERE integration_id=$1 AND root=$2 AND state IN ('pending','running','paused')`, connectionID, root).Scan(&id); err != nil {
			return "", false, err
		}
		return id, false, nil
	}
	if _, err = tx.Exec(ctx, `INSERT INTO library_scan_directories(scan_id,path)VALUES($1,$2)`, id, root); err != nil {
		return "", false, err
	}
	body, _ := json.Marshal(map[string]string{"scanId": id})
	if _, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,payload,queue,idempotency_key)VALUES($1,'library.scan',$2,'library',$3)`, jobs.UUID(), body, "library.scan:"+id); err != nil {
		return "", false, err
	}
	return id, true, nil
}
func (e Engine) Scans(ctx context.Context) ([]Scan, error) { return e.ScansPage(ctx, 0, 100) }
func (e Engine) ScansPage(ctx context.Context, start, limit int) ([]Scan, error) {
	rows, err := e.DB.Query(ctx, `SELECT id::text,integration_id::text,root,state,(SELECT count(*) FROM library_scan_seen s WHERE s.scan_id=library_scans.id),(SELECT count(*) FROM library_scan_directories d WHERE d.scan_id=library_scans.id),message,created_at,updated_at,integration_revision,generation,current_directory,(SELECT count(*) FROM library_scan_directories d WHERE d.scan_id=library_scans.id AND d.state='completed') FROM library_scans ORDER BY created_at DESC,id LIMIT $1 OFFSET $2`, limit, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Scan{}
	for rows.Next() {
		var item Scan
		if err = rows.Scan(&item.ID, &item.ConnectionID, &item.Root, &item.State, &item.Files, &item.Directories, &item.Message, &item.CreatedAt, &item.UpdatedAt, &item.Revision, &item.Generation, &item.CurrentDirectory, &item.CompletedDirectories); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func (e Engine) Process(ctx context.Context, job jobs.Job) (result error) {
	var body struct {
		ScanID     string `json:"scanId"`
		Generation int64  `json:"generation"`
	}
	if json.Unmarshal(job.Payload, &body) != nil {
		return errors.New("invalid scan job")
	}
	var connectionID, root, state string
	var expectedRevision, generation int64
	// A dedicated session lock prevents two retry generations from clearing or
	// extending the same directory evidence concurrently.
	lock, err := e.DB.Acquire(ctx)
	if err != nil {
		return err
	}
	defer lock.Release()
	var acquired bool
	if err = lock.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,842))`, body.ScanID).Scan(&acquired); err != nil {
		return err
	}
	if !acquired {
		return errors.New("scan is already being processed")
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var unlocked bool
		if lock.QueryRow(c, `SELECT pg_advisory_unlock(hashtextextended($1,842))`, body.ScanID).Scan(&unlocked) != nil || !unlocked {
			deferutil.CloseContext(c, lock.Conn(), "Read/descriptor teardown only; primary operations handle their own I/O result")
		}
	}()
	if err := e.DB.QueryRow(ctx, `SELECT integration_id::text,root,state,integration_revision,generation FROM library_scans WHERE id=$1`, body.ScanID).Scan(&connectionID, &root, &state, &expectedRevision, &generation); err != nil {
		return err
	}
	var rootLocked bool
	if err = lock.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,0))`, "files:"+connectionID).Scan(&rootLocked); err != nil {
		return err
	}
	if !rootLocked {
		return errors.New("file operation is active; scan will retry without missing votes")
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var released bool
		if lock.QueryRow(c, `SELECT pg_advisory_unlock(hashtextextended($1,0))`, "files:"+connectionID).Scan(&released) != nil || !released {
			deferutil.CloseContext(c, lock.Conn(), "Read/descriptor teardown only; primary operations handle their own I/O result")
		}
	}()
	if body.Generation == 0 {
		body.Generation = 1
	}
	if state == "paused" || state == "cancelled" || generation != body.Generation {
		return jobs.ErrOperatorRequired
	}
	if state == "succeeded" {
		return nil
	}
	tag, err := e.DB.Exec(ctx, `UPDATE library_scans SET state='running',message='',updated_at=now(),coverage_started_at=now() WHERE id=$1 AND generation=$2 AND state IN ('pending','running')`, body.ScanID, generation)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return jobs.ErrOperatorRequired
	}
	defer func() {
		if result != nil {
			status := "running"
			if job.Attempts >= job.MaxAttempts {
				status = "failed"
			}
			if _, checkedErr := e.DB.Exec(context.WithoutCancel(ctx), `UPDATE library_scans SET state=$2,updated_at=now(),message=$4 WHERE id=$1 AND generation=$3 AND state='running'`, body.ScanID, status, generation, "目录扫描未完成；不会据此标记缺失或清理输出："+result.Error()); checkedErr != nil {
				result = errors.Join(result, checkedErr)
				return
			}
		}
	}()
	if job.Attempts > 1 || generation > 1 {
		tx, err := e.DB.Begin(ctx)
		if err != nil {
			return err
		}
		defer deferutil.Rollback(ctx, tx)
		var state string
		var current int64
		if err = tx.QueryRow(ctx, `SELECT state,generation FROM library_scans WHERE id=$1 FOR UPDATE`, body.ScanID).Scan(&state, &current); err != nil {
			return err
		}
		if state != "running" || current != generation {
			return jobs.ErrOperatorRequired
		}
		if _, err = tx.Exec(ctx, `DELETE FROM library_scan_seen WHERE scan_id=$1`, body.ScanID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM library_scan_directories WHERE scan_id=$1`, body.ScanID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO library_scan_directories(scan_id,path)VALUES($1,$2)`, body.ScanID, root); err != nil {
			return err
		}
		if err = dbutil.Commit(ctx, tx); err != nil {
			return err
		}
	}
	connection, err := e.Connections.Repository.Get(ctx, connectionID)
	if err != nil {
		return err
	}
	allowed := connection.String("cloudRoot")
	if allowed == "" && len(e.Connections.Config.STRMAllowedRoots) > 0 {
		allowed = e.Connections.Config.STRMAllowedRoots[0]
	}
	if connection.Record.Revision != expectedRevision {
		return errors.New("scan connection revision changed; restart with fresh coverage")
	}
	if !connection.Record.Enabled || allowed == "" || !inside(allowed, root) || strings.Contains("/"+root+"/", "/.media-tool-trash/") {
		return errors.New("scan connection or allowed directory changed")
	}

	client, err := e.Connections.CloudByID(ctx, connectionID)
	if err != nil {
		return err
	}
	settings, _ := json.Marshal(connection.Record.Settings)
	if _, err = e.DB.Exec(ctx, `INSERT INTO storage_connections(id,name,provider,settings,enabled)VALUES($1,$2,'clouddrive2',$3,true)ON CONFLICT(id)DO NOTHING`, connectionID, connection.Record.Name, settings); err != nil {
		return err
	}
	trace.Info(ctx, "library", "scan coverage started", "scanId", body.ScanID, "root", root, "generation", generation, "revision", expectedRevision)
	rootInfo, err := client.Find(ctx, root)
	if err != nil || !rootInfo.IsDirectory {
		return errors.New("scan root is not a readable cloud directory")
	}
	check := func() error {
		var s string
		var g int64
		if err := e.DB.QueryRow(ctx, `SELECT state,generation FROM library_scans WHERE id=$1`, body.ScanID).Scan(&s, &g); err != nil {
			return err
		}
		if s != "running" || g != generation {
			return jobs.ErrOperatorRequired
		}
		return nil
	}
	for {
		if err := check(); err != nil {
			return err
		}
		var directory string
		err = e.DB.QueryRow(ctx, `SELECT path FROM library_scan_directories WHERE scan_id=$1 AND state<>'completed'ORDER BY path LIMIT 1`, body.ScanID).Scan(&directory)
		if errors.Is(err, pgx.ErrNoRows) {
			break
		}
		if err != nil {
			return err
		}
		if _, err = e.DB.Exec(ctx, `UPDATE library_scans SET current_directory=$2,updated_at=now() WHERE id=$1 AND generation=$3 AND state='running'`, body.ScanID, directory, generation); err != nil {
			return err
		}
		if strings.Count(strings.TrimPrefix(directory, root), "/") > 32 {
			return errors.New("cloud tree exceeds depth limit")
		}
		buffer := make([]storage.File, 0, scanBatchSize)
		bufferBytes := 0
		flush := func() error {
			if len(buffer) == 0 {
				return nil
			}
			fast, err := e.indexNewBatch(ctx, body.ScanID, generation, connectionID, expectedRevision, buffer)
			if err != nil {
				return err
			}
			if !fast {
				for _, file := range buffer {
					if err := check(); err != nil {
						return err
					}
					id, err := e.indexFile(ctx, connectionID, file)
					if err != nil {
						return err
					}
					if _, err = e.DB.Exec(ctx, `INSERT INTO library_scan_seen(scan_id,file_id)VALUES($1,$2)ON CONFLICT DO NOTHING`, body.ScanID, id); err != nil {
						return err
					}
					if metadata.IsVideo(file.Name) && !metadata.IsSample(file.Path) {
						if _, err = e.DB.Exec(ctx, `INSERT INTO library_candidates(id,file_id)SELECT $1,$2 WHERE NOT EXISTS(SELECT 1 FROM media_assets WHERE cloud_file_id=$2)ON CONFLICT(file_id)DO NOTHING`, jobs.UUID(), id); err != nil {
							return err
						}
					}
				}
			}
			clear(buffer)
			buffer = buffer[:0]
			bufferBytes = 0
			return nil
		}
		if err = client.Iterate(ctx, directory, func(file storage.File) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if file.Path == "" || path.Clean(file.Path) != file.Path || path.Dir(file.Path) != directory || !inside(root, file.Path) || file.Size < 0 {
				return errors.New("cloud listing contains an unsafe path")
			}
			if file.IsDirectory {
				if file.Name == ".media-tool-trash" {
					return nil
				}
				if err := flush(); err != nil {
					return err
				}
				if err := check(); err != nil {
					return err
				}
				_, err := e.DB.Exec(ctx, `INSERT INTO library_scan_directories(scan_id,path)VALUES($1,$2)ON CONFLICT DO NOTHING`, body.ScanID, file.Path)
				return err
			}
			weight := scanMetadataBytes(file)
			if len(buffer) > 0 && bufferBytes+weight > scanBatchBytes {
				if err := flush(); err != nil {
					return err
				}
			}
			buffer = append(buffer, file)
			bufferBytes += weight
			if len(buffer) == scanBatchSize || bufferBytes >= scanBatchBytes {
				return flush()
			}
			return nil
		}); err != nil {
			return err
		}
		if err = flush(); err != nil {
			return err
		}

		if _, err = e.DB.Exec(ctx, `UPDATE library_scan_directories SET state='completed'WHERE scan_id=$1 AND path=$2`, body.ScanID, directory); err != nil {
			return err
		}
		dirs, files, err := scanCounts(ctx, e.DB, body.ScanID)
		if err != nil {
			return err
		}
		if dirs > 20000 || files > 500000 {
			return errors.New("scan safety limit exceeded")
		}
		if _, err = e.DB.Exec(ctx, `UPDATE library_scans SET files_seen=$2,directories_seen=$3,updated_at=now() WHERE id=$1 AND generation=$4 AND state='running'`, body.ScanID, files, dirs, generation); err != nil {
			return err
		}
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var finalState string
	var finalGeneration int64
	if err = tx.QueryRow(ctx, `SELECT state,generation FROM library_scans WHERE id=$1 FOR UPDATE`, body.ScanID).Scan(&finalState, &finalGeneration); err != nil {
		return err
	}
	if finalState != "running" || finalGeneration != generation {
		return jobs.ErrOperatorRequired
	}
	var currentRevision int64
	var stillEnabled bool
	if err = tx.QueryRow(ctx, `SELECT revision,enabled FROM integrations WHERE id=$1 FOR SHARE`, connectionID).Scan(&currentRevision, &stillEnabled); err != nil {
		return err
	}
	if !stillEnabled || currentRevision != connection.Record.Revision {
		return errors.New("connection changed during scan; missing-file votes were not applied")
	}
	var changedDuringScan bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM library_scan_schedules s JOIN library_scans scan ON scan.id=$2 WHERE s.integration_id=$1 AND s.last_change_at>scan.coverage_started_at)`, connectionID, body.ScanID).Scan(&changedDuringScan); err != nil {
		return err
	}
	if changedDuringScan {
		return errors.New("cloud changes observed during coverage; a new complete scan is required before missing votes")
	}
	if _, err = tx.Exec(ctx, `UPDATE cloud_files SET missing_scan_count=missing_scan_count+1,state=CASE WHEN missing_scan_count+1>=2 THEN 'missing'ELSE 'suspected_missing'END,missing_since=coalesce(missing_since,now()) WHERE state<>'trashed' AND storage_id=$1 AND (full_path=$2 OR left(full_path,length($2)+1)=$2||'/')AND NOT EXISTS(SELECT 1 FROM library_scan_seen WHERE scan_id=$3 AND file_id=cloud_files.id)`, connectionID, root, body.ScanID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE library_scans SET state='succeeded',message='完整扫描完成；缺失需两次完整扫描确认，默认不删除STRM',completed_at=now(),updated_at=now(),current_directory='' WHERE id=$1`, body.ScanID); err != nil {
		return err
	}
	trace.Info(ctx, "library", "complete scan coverage verified", "scanId", body.ScanID, "root", root, "generation", generation)
	return dbutil.Commit(ctx, tx)
}
func (e Engine) indexFile(ctx context.Context, connectionID string, file storage.File) (string, error) {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer deferutil.Rollback(ctx, tx)
	var id string
	err = tx.QueryRow(ctx, `SELECT id::text FROM cloud_files WHERE storage_id=$1 AND full_path=$2 FOR UPDATE`, connectionID, file.Path).Scan(&id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if id == "" && file.ID != "" && file.Hashes[2] != "" {
		rows, err := tx.Query(ctx, `SELECT id::text,size_bytes,metadata FROM cloud_files WHERE storage_id=$1 AND provider_file_id=$2 AND provider_file_id<>'' AND md5(provider_file_id)=md5($2) FOR UPDATE`, connectionID, file.ID)
		if err != nil {
			return "", err
		}
		matches := []string{}
		for rows.Next() {
			var existing string
			var size int64
			var raw []byte
			if rows.Scan(&existing, &size, &raw) != nil {
				continue
			}
			var hashes map[uint32]string
			if checkedErr := json.Unmarshal(raw, &hashes); checkedErr != nil {
				return "", checkedErr
			}
			if size == file.Size && hashes[2] != "" && strings.EqualFold(hashes[2], file.Hashes[2]) {
				matches = append(matches, existing)
			}
		}
		rows.Close()
		if len(matches) == 1 {
			id = matches[0]
		}
	}
	if id == "" {
		id = jobs.UUID()
	}
	raw, _ := json.Marshal(file.Hashes)
	var bound, changed, replaced bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_assets WHERE cloud_file_id=$1), EXISTS(SELECT 1 FROM cloud_files WHERE id=$1 AND (provider_file_id IS DISTINCT FROM $2 OR full_path<>$3 OR size_bytes<>$4 OR metadata<>$5::jsonb)), EXISTS(SELECT 1 FROM cloud_files WHERE id=$1 AND (provider_file_id IS DISTINCT FROM $2 OR size_bytes<>$4 OR (coalesce(metadata->>'2','')<>'' AND coalesce($5::jsonb->>'2','')<>'' AND lower(metadata->>'2')<>lower($5::jsonb->>'2'))))`, id, file.ID, file.Path, file.Size, raw).Scan(&bound, &changed, &replaced); err != nil {
		return "", err
	}
	if bound && replaced {
		return "", errors.New("indexed media identity changed at this path; old playback grant was not rebound")
	}
	if changed {
		if _, err = tx.Exec(ctx, `UPDATE library_candidates SET revision=revision+1,updated_at=now() WHERE file_id=$1`, id); err != nil {
			return "", err
		}
	}

	_, err = tx.Exec(ctx, `INSERT INTO cloud_files(id,storage_id,provider_file_id,full_path,size_bytes,metadata)VALUES($1,$2,$3,$4,$5,$6)ON CONFLICT(id)DO UPDATE SET provider_file_id=excluded.provider_file_id,full_path=excluded.full_path,size_bytes=excluded.size_bytes,metadata=excluded.metadata,state='present',missing_scan_count=0,missing_since=NULL,updated_at=now()`, id, connectionID, file.ID, file.Path, file.Size, raw)
	if err != nil {
		return "", err
	}
	if err = dbutil.Commit(ctx, tx); err != nil {
		return "", err
	}
	return id, nil
}
