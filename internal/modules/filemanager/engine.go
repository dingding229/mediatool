package filemanager

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/outbox"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/releasename"
	"media-tool/internal/modules/settings"
	"media-tool/internal/platform/trace"
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
		root := Root{ID: c.ID, Name: c.Name, Revision: c.Revision, ConnectionState: c.LastCheckState}
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
		if root.Path == "" || root.Path == "/" || root.Kind == "local" && !safeLocalRoot(root.Path) {
			continue
		}
		root.Origin = fingerprint(map[string]any{"kind": root.Kind, "root": root.Path, "endpoint": c.Settings["endpoint"]})
		root.Capabilities = Capabilities{Read: true, Write: true, Move: false, Recycle: false, Role: "cloud-target"}
		if root.Kind == "local" {
			b, openErr := e.backend(ctx, root)
			if openErr != nil {
				root.ConnectionState = "unavailable"
				root.Capabilities = Capabilities{ReadOnly: true, Role: "workspace", Reason: "目录不可读取"}
			} else {
				root.Capabilities, err = e.capability(ctx, root, ".", b, nil)
				deferutil.Close(b, "Read/descriptor teardown only; primary operations handle their own I/O result")
				if err != nil {
					return nil, err
				}
				root.ConnectionState = "accessible"
			}
		}
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
	defer deferutil.Close(b, "Read/descriptor teardown only; primary operations handle their own I/O result")
	capability, err := e.capability(ctx, root, p, b, nil)
	if err != nil {
		return nil, err
	}
	if !capability.Read {
		return nil, ErrProtected
	}
	items, err := b.List(ctx, p)
	if err != nil {
		return nil, err
	}
	visible := make([]File, 0, len(items))
	for _, item := range items {
		if root.Kind == "local" && e.privateBrowsePath(root, item.Path) {
			continue
		}
		visible = append(visible, item)
	}
	items = visible
	total := len(items)
	end := offset + limit
	if offset > total {
		offset = total
	}
	if end > total {
		end = total
	}
	pageItems := items[offset:end]
	for i := range pageItems {
		c, capErr := e.capability(ctx, root, pageItems[i].Path, b, &pageItems[i])
		if capErr != nil {
			return nil, capErr
		}
		pageItems[i].Capabilities = &c
	}
	return map[string]any{"root": root, "capabilities": capability, "path": p, "items": pageItems, "total": total, "offset": offset, "limit": limit}, nil
}
func (e Engine) protected(ctx context.Context, root Root, relative string) error {
	if root.Kind != "local" {
		return nil
	}
	full := filepath.Join(root.Path, filepath.FromSlash(relative))
	if actual, err := filepath.EvalSymlinks(full); err == nil {
		full = actual
	}
	for _, private := range []string{e.Connections.Config.LogDir, e.Connections.Config.MaintenanceDir, e.Connections.Config.PasswordResetFile} {
		if private == "" {
			continue
		}
		p, err := filepath.Abs(private)
		if err == nil && overlaps(full, p) {
			return ErrProtected
		}
	}
	if privateLocalPath(relative) {
		return ErrProtected
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
	ctx, selected, err := (pluginv2.Store{DB: e.DB, Vault: e.Connections.Repository.Vault}).SelectStorage(ctx, contracts.StorageSelection{Handles: []string{root.ID}, Selected: root.ID})
	if err != nil || selected != root.ID {
		return Plan{}, ErrChanged
	}
	b, err := e.backend(ctx, root)
	if err != nil {
		return Plan{}, err
	}
	defer deferutil.Close(b, "Read/descriptor teardown only; primary operations handle their own I/O result")
	if in.ExpectedFileID != "" || in.ExpectedSize != nil || in.ExpectedModified != nil {
		source, err := b.Find(ctx, in.Source)
		if err != nil {
			return Plan{}, err
		}
		if in.ExpectedFileID != "" && source.ID != in.ExpectedFileID || in.ExpectedSize != nil && source.Size != *in.ExpectedSize || in.ExpectedModified != nil && source.Modified != *in.ExpectedModified {
			return Plan{}, ErrChanged
		}
	}
	if (in.Operation == "organize" || in.Operation == "rename") && in.Name == "" && in.Template == "" {
		source, findErr := b.Find(ctx, in.Source)
		if findErr != nil {
			return Plan{}, findErr
		}
		if !source.Directory {
			hint := releasename.Parse(source.Name)
			if len(hint.Episodes) > 1 {
				return Plan{}, ErrInvalid
			} // multi-episode files need an explicit complete custom name, not a single-episode template
			if in.Title != "" {
				hint.Title = in.Title
			}
			if in.Year != "" {
				hint.Year = in.Year
			}
			if in.MediaType != "" {
				hint.MediaType = in.MediaType
			}
			if in.Season != 0 || in.MediaType == "series" {
				hint.Season = in.Season
			}
			if in.Episode != 0 {
				hint.Episode = in.Episode
			}
			if in.Resolution != "" {
				hint.Resolution = in.Resolution
			}
			in.Title, in.Year, in.MediaType, in.Season, in.Episode = hint.Title, hint.Year, hint.MediaType, hint.Season, hint.Episode
			prefs, prefErr := (settings.Repository{DB: e.DB}).Get(ctx)
			if prefErr != nil {
				return Plan{}, ErrUnavailable
			}
			template := prefs.Value.MovieTemplate
			if hint.MediaType == "series" {
				template = prefs.Value.SeriesTemplate
			}
			if hint.MediaType == "unknown" {
				return Plan{}, ErrInvalid
			}
			rendered, renderErr := releasename.RenderPath(template, hint, source.Name)
			err = renderErr
			in.Name = path.Base(rendered)
			if in.Operation == "organize" && strings.Contains(rendered, "/") {
				base := in.Destination
				if base == "" || base == "." {
					base = "电影"
					if hint.MediaType == "series" {
						base = "电视剧"
					}
					if in.Category != "" {
						if Name(in.Category) != nil {
							return Plan{}, ErrInvalid
						}
						base = path.Join(base, in.Category)
					}
				}
				in.Destination = path.Join(base, path.Dir(rendered))
			}
			if err != nil {
				return Plan{}, ErrInvalid
			}
		}
	}
	if in.Template != "" && strings.Contains(in.Template, "/") {
		source, findErr := b.Find(ctx, in.Source)
		if findErr != nil {
			return Plan{}, findErr
		}
		hint := releasename.Parse(source.Name)
		hint.Title, hint.Year, hint.MediaType, hint.Season, hint.Episode = in.Title, in.Year, in.MediaType, in.Season, in.Episode
		if in.Resolution != "" {
			hint.Resolution = in.Resolution
		}
		rendered, err := releasename.RenderPath(in.Template, hint, source.Name)
		if err != nil {
			return Plan{}, ErrInvalid
		}
		in.Name = path.Base(rendered)
		in.Template = ""
		if in.Operation == "organize" {
			base := in.Destination
			if base == "" || base == "." {
				base = "电影"
				if hint.MediaType == "series" {
					base = "电视剧"
				}
			}
			in.Destination = path.Join(base, path.Dir(rendered))
		} else if in.Operation != "rename" {
			return Plan{}, ErrInvalid
		}
	}
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
		if err = e.requireWritable(ctx, root, b, step); err != nil {
			return p, err
		}
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
	_, err = dbutil.Exec(ctx, e.DB, `INSERT INTO file_operations(id,integration_id,integration_revision,backend_kind,operation,plan,plan_digest,expires_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, p.ID, p.ConnectionID, p.Revision, p.BackendKind, p.Operation, raw, p.Digest, p.ExpiresAt)
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
			deferutil.CloseContext(c, conn.Conn(), "Read/descriptor teardown only; primary operations handle their own I/O result")
		}
		conn.Release()
	}, nil
}
func (e Engine) claim(ctx context.Context, p Plan) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer deferutil.Rollback(ctx, tx)
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
	return dbutil.Commit(ctx, tx)
}
func (e Engine) finish(ctx context.Context, id, state, message string, completed int) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var oldState string
	var oldCompleted int
	if err = tx.QueryRow(ctx, `SELECT state,completed_steps FROM file_operations WHERE id=$1 FOR UPDATE`, id).Scan(&oldState, &oldCompleted); err != nil {
		return err
	}
	if oldState == state && oldCompleted == completed && (state == "failed" || state == "succeeded" || state == "cancelled") {
		return dbutil.Commit(ctx, tx)
	}
	var version time.Time
	if err = tx.QueryRow(ctx, `UPDATE file_operations SET state=$2,message=$3,completed_steps=$4,updated_at=now() WHERE id=$1 RETURNING updated_at`, id, state, message, completed).Scan(&version); err != nil {
		return err
	}
	if err = outbox.EmitVersionTx(ctx, tx, "files."+state, id, version.UTC().Format(time.RFC3339Nano), map[string]string{"operationId": id, "state": state, "message": message}); err != nil {
		return err
	}
	trace.Info(ctx, "filemanager", "file operation result", "operationId", id, "state", state, "completedSteps", completed, "message", message)
	// Plugin deliveries and notifications independently consume the committed files.* fact.

	return dbutil.Commit(ctx, tx)
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
	defer deferutil.Close(b, "Read/descriptor teardown only; primary operations handle their own I/O result")
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
		if err = e.requireWritable(ctx, root, b, step); err != nil {
			return out, err
		}
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
	trace.Info(ctx, "filemanager", "approved file operation started", "operationId", id, "root", out.Plan.Root, "items", out.Plan.Items)

	for _, step := range out.Plan.Steps {
		if ctx.Err() != nil {
			err = ctx.Err()
			break
		}
		if err = dbutil.CheckEffect(ctx); err != nil {
			break
		}
		if step.Action == "mkdir" {
			err = absent(ctx, b, step.Target)
			if errors.Is(err, ErrConflict) {
				err = e.sharedParent(ctx, b, out.Plan, step.Target)
			}
			if err == nil {
				if _, foundErr := b.Find(ctx, step.Target); errors.Is(foundErr, ErrMissing) {
					err = b.Mkdir(ctx, step.Target)
				}
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
		if err = e.recordStep(ctx, out.Plan, step, completed+1, target); err != nil {
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
func (e Engine) recordStep(ctx context.Context, p Plan, step Step, completed int, observed ...File) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	if step.Action == "mkdir" && len(observed) == 1 {
		raw, _ := json.Marshal(observed[0])
		if _, err = tx.Exec(ctx, `INSERT INTO file_batch_parents(batch_id,integration_id,path,identity) SELECT batch_id,$2,$3,$4 FROM file_batch_items WHERE operation_id=$1 ON CONFLICT DO NOTHING`, p.ID, p.ConnectionID, step.Target, raw); err != nil {
			return err
		}
	}
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
	return dbutil.Commit(ctx, tx)
}
