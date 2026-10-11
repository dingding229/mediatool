package library

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/storage"
)

type Engine struct {
	DB          *pgxpool.Pool
	Connections *integrations.Manager
}
type Scan struct {
	ID           string    `json:"id"`
	ConnectionID string    `json:"connectionId"`
	Root         string    `json:"root"`
	State        string    `json:"state"`
	Files        int       `json:"files"`
	Directories  int       `json:"directories"`
	Message      string    `json:"message"`
	CreatedAt    time.Time `json:"createdAt"`
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
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	id, _, err := startScanTx(ctx, tx, connection.Record.ID, root)
	if err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
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
	tag, err := tx.Exec(ctx, `INSERT INTO library_scans(id,integration_id,root)VALUES($1,$2,$3)ON CONFLICT(integration_id,root)WHERE state IN ('pending','running')DO NOTHING`, id, connectionID, root)
	if err != nil {
		return "", false, err
	}
	if tag.RowsAffected() == 0 {
		if err = tx.QueryRow(ctx, `SELECT id::text FROM library_scans WHERE integration_id=$1 AND root=$2 AND state IN ('pending','running')`, connectionID, root).Scan(&id); err != nil {
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
func (e Engine) Scans(ctx context.Context) ([]Scan, error) {
	rows, err := e.DB.Query(ctx, `SELECT id::text,integration_id::text,root,state,files_seen,directories_seen,message,created_at FROM library_scans ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Scan{}
	for rows.Next() {
		var item Scan
		if err = rows.Scan(&item.ID, &item.ConnectionID, &item.Root, &item.State, &item.Files, &item.Directories, &item.Message, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func (e Engine) Process(ctx context.Context, job jobs.Job) (result error) {
	var body struct {
		ScanID string `json:"scanId"`
	}
	if json.Unmarshal(job.Payload, &body) != nil {
		return errors.New("invalid scan job")
	}
	var connectionID, root, state string
	if err := e.DB.QueryRow(ctx, `SELECT integration_id::text,root,state FROM library_scans WHERE id=$1`, body.ScanID).Scan(&connectionID, &root, &state); err != nil {
		return err
	}
	if state == "succeeded" {
		return nil
	}
	if _, err := e.DB.Exec(ctx, `UPDATE library_scans SET state='running',message=''WHERE id=$1`, body.ScanID); err != nil {
		return err
	}
	defer func() {
		if result != nil {
			status := "running"
			if job.Attempts >= job.MaxAttempts {
				status = "failed"
			}
			e.DB.Exec(context.WithoutCancel(ctx), `UPDATE library_scans SET state=$2,message='目录扫描未完成；不会据此标记缺失或清理输出'WHERE id=$1`, body.ScanID, status)
		}
	}()
	if job.Attempts > 1 {
		tx, err := e.DB.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `DELETE FROM library_scan_seen WHERE scan_id=$1`, body.ScanID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM library_scan_directories WHERE scan_id=$1`, body.ScanID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO library_scan_directories(scan_id,path)VALUES($1,$2)`, body.ScanID, root); err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
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
	rootInfo, err := client.Find(ctx, root)
	if err != nil || !rootInfo.IsDirectory {
		return errors.New("scan root is not a readable cloud directory")
	}
	for {
		var directory string
		err = e.DB.QueryRow(ctx, `SELECT path FROM library_scan_directories WHERE scan_id=$1 AND state<>'completed'ORDER BY path LIMIT 1`, body.ScanID).Scan(&directory)
		if errors.Is(err, pgx.ErrNoRows) {
			break
		}
		if err != nil {
			return err
		}
		if strings.Count(strings.TrimPrefix(directory, root), "/") > 32 {
			return errors.New("cloud tree exceeds depth limit")
		}
		if err = client.Iterate(ctx, directory, func(file storage.File) error {
			if file.Path == "" || path.Clean(file.Path) != file.Path || path.Dir(file.Path) != directory || !inside(root, file.Path) || file.Size < 0 {
				return errors.New("cloud listing contains an unsafe path")
			}
			if file.IsDirectory {
				if file.Name == ".media-tool-trash" {
					return nil
				}
				_, err := e.DB.Exec(ctx, `INSERT INTO library_scan_directories(scan_id,path)VALUES($1,$2)ON CONFLICT DO NOTHING`, body.ScanID, file.Path)
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
				_, err = e.DB.Exec(ctx, `INSERT INTO library_candidates(id,file_id)SELECT $1,$2 WHERE NOT EXISTS(SELECT 1 FROM media_assets WHERE cloud_file_id=$2)ON CONFLICT(file_id)DO NOTHING`, jobs.UUID(), id)
			}
			return err
		}); err != nil {
			return err
		}
		if _, err = e.DB.Exec(ctx, `UPDATE library_scan_directories SET state='completed'WHERE scan_id=$1 AND path=$2`, body.ScanID, directory); err != nil {
			return err
		}
		var dirs, files int
		e.DB.QueryRow(ctx, `SELECT count(*)FROM library_scan_directories WHERE scan_id=$1`, body.ScanID).Scan(&dirs)
		e.DB.QueryRow(ctx, `SELECT count(*)FROM library_scan_seen WHERE scan_id=$1`, body.ScanID).Scan(&files)
		if dirs > 20000 || files > 500000 {
			return errors.New("scan safety limit exceeded")
		}
		if _, err = e.DB.Exec(ctx, `UPDATE library_scans SET files_seen=$2,directories_seen=$3 WHERE id=$1`, body.ScanID, files, dirs); err != nil {
			return err
		}
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var currentRevision int64
	var stillEnabled bool
	if err = tx.QueryRow(ctx, `SELECT revision,enabled FROM integrations WHERE id=$1 FOR SHARE`, connectionID).Scan(&currentRevision, &stillEnabled); err != nil {
		return err
	}
	if !stillEnabled || currentRevision != connection.Record.Revision {
		return errors.New("connection changed during scan; missing-file votes were not applied")
	}
	if _, err = tx.Exec(ctx, `UPDATE cloud_files SET missing_scan_count=missing_scan_count+1,state=CASE WHEN missing_scan_count+1>=2 THEN 'missing'ELSE 'suspected_missing'END,missing_since=coalesce(missing_since,now()) WHERE state<>'trashed' AND storage_id=$1 AND (full_path=$2 OR left(full_path,length($2)+1)=$2||'/')AND NOT EXISTS(SELECT 1 FROM library_scan_seen WHERE scan_id=$3 AND file_id=cloud_files.id)`, connectionID, root, body.ScanID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE library_scans SET state='succeeded',message='完整扫描完成；缺失需两次完整扫描确认，默认不删除STRM',completed_at=now()WHERE id=$1`, body.ScanID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (e Engine) indexFile(ctx context.Context, connectionID string, file storage.File) (string, error) {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `SELECT id::text FROM cloud_files WHERE storage_id=$1 AND full_path=$2 FOR UPDATE`, connectionID, file.Path).Scan(&id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if id == "" && file.ID != "" && file.Hashes[2] != "" {
		rows, err := tx.Query(ctx, `SELECT id::text,size_bytes,metadata FROM cloud_files WHERE storage_id=$1 AND provider_file_id=$2 FOR UPDATE`, connectionID, file.ID)
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
			json.Unmarshal(raw, &hashes)
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
	_, err = tx.Exec(ctx, `INSERT INTO cloud_files(id,storage_id,provider_file_id,full_path,size_bytes,metadata)VALUES($1,$2,$3,$4,$5,$6)ON CONFLICT(id)DO UPDATE SET provider_file_id=excluded.provider_file_id,full_path=excluded.full_path,size_bytes=excluded.size_bytes,metadata=excluded.metadata,state='present',missing_scan_count=0,missing_since=NULL,updated_at=now()`, id, connectionID, file.ID, file.Path, file.Size, raw)
	if err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}
