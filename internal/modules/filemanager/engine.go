package filemanager

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
)

type Engine struct {
	DB          *pgxpool.Pool
	Connections *integrations.Manager
	Open        func(context.Context, Root) (Backend, error)
}

func (e Engine) Roots(ctx context.Context) ([]Root, error) {
	records, err := e.Connections.Repository.List(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	out := []Root{}
	for _, c := range records {
		if !c.Enabled {
			continue
		}
		root := Root{ID: c.ID, Name: c.Name, Revision: c.Revision}
		switch c.Kind {
		case "clouddrive2":
			root.Kind = "cloud"
			root.Path, _ = c.Settings["cloudRoot"].(string)
			if root.Path == "" && len(e.Connections.Config.STRMAllowedRoots) > 0 {
				root.Path = e.Connections.Config.STRMAllowedRoots[0]
			}
		case "local":
			root.Kind = "local"
			root.Path, _ = c.Settings["rootPath"].(string)
		default:
			continue
		}
		if root.Path == "" || root.Path == "/" {
			continue
		}
		root.Origin = fingerprint(map[string]any{"kind": root.Kind, "root": root.Path, "endpoint": c.Settings["endpoint"]})
		out = append(out, root)
	}
	return out, nil
}
func (e Engine) root(ctx context.Context, id string) (Root, error) {
	roots, err := e.Roots(ctx)
	if err != nil {
		return Root{}, err
	}
	for _, root := range roots {
		if root.ID == id {
			return root, nil
		}
	}
	return Root{}, ErrMissing
}
func (e Engine) backend(ctx context.Context, root Root) (Backend, error) {
	if e.Open != nil {
		return e.Open(ctx, root)
	}
	if root.Kind == "local" {
		return OpenLocal(root.Path)
	}
	client, err := e.Connections.CloudByID(ctx, root.ID)
	if err != nil {
		return nil, ErrUnavailable
	}
	return &Cloud{Client: client, Root: root.Path}, nil
}
func (e Engine) List(ctx context.Context, id, relative string, limit, offset int) (map[string]any, error) {
	if limit < 1 || limit > 200 || offset < 0 || offset > 100000 {
		return nil, ErrInvalid
	}
	p, err := Relative(relative)
	if err != nil {
		return nil, err
	}
	root, err := e.root(ctx, id)
	if err != nil {
		return nil, err
	}
	b, err := e.backend(ctx, root)
	if err != nil {
		return nil, err
	}
	defer b.Close()
	items, err := b.List(ctx, p)
	if err != nil {
		return nil, err
	}
	total := len(items)
	end := offset + limit
	if offset > total {
		offset = total
	}
	if end > total {
		end = total
	}
	return map[string]any{"root": root, "path": p, "items": items[offset:end], "total": total, "offset": offset, "limit": limit}, nil
}
func (e Engine) protected(ctx context.Context, root Root, relative string) error {
	if root.Kind != "local" {
		return nil
	}
	full := filepath.Join(root.Path, filepath.FromSlash(relative))
	if actual, err := filepath.EvalSymlinks(full); err == nil {
		full = actual
	}
	output, err := filepath.Abs(e.Connections.Config.STRMOutputDir)
	if e.Connections.Config.STRMOutputDir != "" && err == nil && output != "" && overlaps(full, output) {
		return ErrProtected
	}
	rows, err := e.DB.Query(ctx, `SELECT settings->>'localPath' FROM integrations WHERE kind='qbittorrent' AND coalesce(settings->>'localPath','')<>''`)
	if err != nil {
		return ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var seed string
		if rows.Scan(&seed) != nil {
			return ErrUnavailable
		}
		if actual, err := filepath.EvalSymlinks(seed); err == nil {
			seed = actual
		}
		if overlaps(full, seed) {
			return ErrProtected
		}
	}
	return rows.Err()
}
func overlaps(a, b string) bool {
	return a == b || strings.HasPrefix(a, strings.TrimRight(b, string(filepath.Separator))+string(filepath.Separator)) || strings.HasPrefix(b, strings.TrimRight(a, string(filepath.Separator))+string(filepath.Separator))
}
func (e Engine) Preview(ctx context.Context, in Input) (Plan, error) {
	root, err := e.root(ctx, in.ConnectionID)
	if err != nil {
		return Plan{}, err
	}
	b, err := e.backend(ctx, root)
	if err != nil {
		return Plan{}, err
	}
	defer b.Close()
	p, err := BuildPlan(ctx, b, root, in, jobs.UUID())
	if err != nil {
		return p, err
	}
	if err = e.validateIndexed(ctx, root, b, p); err != nil {
		return p, err
	}
	if len(p.Steps) > 160 {
		return p, ErrLimit
	}
	for _, step := range p.Steps {
		if step.Action != "mkdir" {
			if err = e.protected(ctx, root, step.Source.Path); err != nil {
				return p, err
			}
		}
		if err = e.protected(ctx, root, step.Target); err != nil {
			return p, err
		}
	}
	if root.Kind == "cloud" {
		for _, item := range p.Items {
			if item.Source.ID == "" {
				continue
			}
			var exists bool
			target := path.Join(root.Path, item.Target)
			source := path.Join(root.Path, item.Source.Path)
			err = e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_files WHERE storage_id=$1 AND (full_path=$2 OR left(full_path,length($2)+1)=$2||'/') AND NOT(full_path=$3 OR left(full_path,length($3)+1)=$3||'/'))`, root.ID, target, source).Scan(&exists)
			if err != nil {
				return p, ErrUnavailable
			}
			if exists {
				return p, ErrConflict
			}
		}
	}
	raw, _ := json.Marshal(p)
	_, err = e.DB.Exec(ctx, `INSERT INTO file_operations(id,integration_id,integration_revision,backend_kind,operation,plan,plan_digest,expires_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, p.ID, p.ConnectionID, p.Revision, p.BackendKind, p.Operation, raw, p.Digest, p.ExpiresAt)
	if err != nil {
		return p, ErrUnavailable
	}
	return p, nil
}
func (e Engine) Get(ctx context.Context, id string) (Result, error) {
	var out Result
	var raw []byte
	err := e.DB.QueryRow(ctx, `SELECT id::text,state,completed_steps,message,plan FROM file_operations WHERE id=$1`, id).Scan(&out.ID, &out.State, &out.CompletedSteps, &out.Message, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrMissing
	}
	if err != nil || json.Unmarshal(raw, &out.Plan) != nil {
		return out, ErrUnavailable
	}
	return out, nil
}
func (e Engine) Operations(ctx context.Context) ([]Result, error) {
	rows, err := e.DB.Query(ctx, `SELECT id::text,state,completed_steps,message,plan FROM file_operations ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	out := []Result{}
	for rows.Next() {
		var item Result
		var raw []byte
		if rows.Scan(&item.ID, &item.State, &item.CompletedSteps, &item.Message, &raw) != nil || json.Unmarshal(raw, &item.Plan) != nil {
			return nil, ErrUnavailable
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func (e Engine) lock(ctx context.Context, id string) (func(), error) {
	conn, err := e.DB.Acquire(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	var locked bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,0))`, "files:"+id).Scan(&locked); err != nil || !locked {
		conn.Release()
		return nil, ErrBusy
	}
	return func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var released bool
		if conn.QueryRow(c, `SELECT pg_advisory_unlock(hashtextextended($1,0))`, "files:"+id).Scan(&released) != nil || !released {
			conn.Conn().Close(c)
		}
		conn.Release()
	}, nil
}
func (e Engine) claim(ctx context.Context, p Plan) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer tx.Rollback(ctx)
	var rev int64
	var enabled bool
	if err = tx.QueryRow(ctx, `SELECT revision,enabled FROM integrations WHERE id=$1 FOR UPDATE`, p.ConnectionID).Scan(&rev, &enabled); err != nil || rev != p.Revision || !enabled {
		return ErrChanged
	}
	var busy bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM library_scans WHERE integration_id=$1 AND state IN ('pending','running')) OR EXISTS(SELECT 1 FROM transfer_tasks WHERE integration_id=$1 AND state NOT IN ('succeeded','completed','imported','cancelled','failed')) OR EXISTS(SELECT 1 FROM file_operations WHERE integration_id=$1 AND id<>$2 AND state IN ('running','needs_reconcile'))`, p.ConnectionID, p.ID).Scan(&busy); err != nil {
		return ErrUnavailable
	}
	if busy {
		return ErrBusy
	}
	tag, err := tx.Exec(ctx, `UPDATE file_operations SET state='running',updated_at=now() WHERE id=$1 AND state='preview' AND expires_at>now() AND plan_digest=$2`, p.ID, p.Digest)
	if err != nil {
		return ErrUnavailable
	}
	if tag.RowsAffected() != 1 {
		return ErrChanged
	}
	audit, _ := json.Marshal(map[string]string{"operationId": p.ID, "operation": p.Operation, "connectionId": p.ConnectionID})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','files.execute',$1)`, audit); err != nil {
		return ErrUnavailable
	}
	return tx.Commit(ctx)
}
func (e Engine) finish(ctx context.Context, id, state, message string, completed int) error {
	_, err := e.DB.Exec(ctx, `UPDATE file_operations SET state=$2,message=$3,completed_steps=$4,updated_at=now() WHERE id=$1`, id, state, message, completed)
	return err
}
func (e Engine) Execute(ctx context.Context, id, digest string, confirm bool) (Result, error) {
	if !confirm || digest == "" {
		return Result{}, ErrInvalid
	}
	out, err := e.Get(ctx, id)
	if err != nil {
		return out, err
	}
	if out.Plan.Digest != digest {
		return out, ErrChanged
	}
	if out.State == "succeeded" {
		return out, nil
	}
	if out.State != "preview" {
		return out, ErrBusy
	}
	root, err := e.root(ctx, out.Plan.ConnectionID)
	if err != nil || root.Revision != out.Plan.Revision || root.Path != out.Plan.Root {
		return out, ErrChanged
	}
	unlock, err := e.lock(ctx, root.ID)
	if err != nil {
		return out, err
	}
	defer unlock()
	b, err := e.backend(ctx, root)
	if err != nil {
		return out, err
	}
	defer b.Close()
	for _, item := range out.Plan.Items {
		if item.Source.ID == "" {
			continue
		}
		actual, err := b.Find(ctx, item.Source.Path)
		if err != nil || !Same(item.Source, actual) {
			return out, ErrChanged
		}
		if err = e.protected(ctx, root, item.Source.Path); err != nil {
			return out, err
		}
		if err = absent(ctx, b, item.Target); err != nil {
			return out, err
		}
	}
	for _, step := range out.Plan.Steps {
		if err = e.protected(ctx, root, step.Target); err != nil {
			return out, err
		}
	}
	if err = e.validateIndexed(ctx, root, b, out.Plan); err != nil {
		return out, err
	}
	if err = e.claim(ctx, out.Plan); err != nil {
		return out, err
	}
	completed := 0

	for _, step := range out.Plan.Steps {
		if ctx.Err() != nil {
			err = ctx.Err()
			break
		}
		if step.Action == "mkdir" {
			err = absent(ctx, b, step.Target)
			if err == nil {
				err = b.Mkdir(ctx, step.Target)
			}
		} else {
			current, findErr := b.Find(ctx, step.Source.Path)
			if findErr != nil || !Same(step.Source, current) {
				err = ErrChanged
			} else if err = absent(ctx, b, step.Target); err == nil {
				err = b.Relocate(ctx, step.Source.Path, step.Target)
			}
		}
		if err != nil {
			break
		}
		target, findErr := b.Find(ctx, step.Target)
		if findErr != nil || step.Action == "mkdir" && !target.Directory || step.Action != "mkdir" && !Same(step.Source, target) {
			err = ErrChanged
			break
		}
		if step.Action != "mkdir" {
			if _, sourceErr := b.Find(ctx, step.Source.Path); !errors.Is(sourceErr, ErrMissing) {
				err = ErrChanged
				break
			}
		}
		if err = e.recordStep(ctx, out.Plan, step, completed+1); err != nil {
			break
		}
		completed++
	}
	doneCtx, doneCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer doneCancel()
	state, message := "succeeded", "已完成并核对文件身份；原播放Key和STRM正文不变"
	if err != nil {
		state = "needs_reconcile"
		message = "操作结果需要核对；不要重复提交，先检查当前源路径和目标路径"
	}
	if finishErr := e.finish(doneCtx, id, state, message, completed); finishErr != nil {
		return out, ErrUnavailable
	}
	return e.Get(doneCtx, id)
}
func (e Engine) recordStep(ctx context.Context, p Plan, step Step, completed int) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if p.BackendKind == "cloud" && step.Action != "mkdir" {
		source, target := path.Join(p.Root, step.Source.Path), path.Join(p.Root, step.Target)
		state := "present"
		if step.Target == TrashDirectory || strings.HasPrefix(step.Target, TrashDirectory+"/") {
			state = "trashed"
		}
		_, err = tx.Exec(ctx, `UPDATE cloud_files SET full_path=$3||substring(full_path FROM length($2)+1),metadata=jsonb_set(jsonb_set(metadata,'{path}',to_jsonb($3||substring(full_path FROM length($2)+1))),'{name}',to_jsonb(regexp_replace($3||substring(full_path FROM length($2)+1),'^.*/','','g'))),state=$4,missing_scan_count=0,missing_since=NULL,updated_at=now() WHERE storage_id=$1 AND (full_path=$2 OR left(full_path,length($2)+1)=$2||'/')`, p.ConnectionID, source, target, state)
		if err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE file_operations SET completed_steps=$2,updated_at=now() WHERE id=$1`, p.ID, completed); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
