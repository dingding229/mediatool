package admin

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/mediaprobe/probequeue"
	"media-tool/internal/modules/mediaprobe/stillqueue"
	"strconv"
	"time"
)

type Repository struct {
	DB       *pgxpool.Pool
	Stills   *stillqueue.Repository
	MaxBytes int64
}

func (r Repository) Summary(parent context.Context) (out Summary, err error) {
	if r.DB == nil || r.Stills == nil {
		return out, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	if err = r.DB.QueryRow(ctx, `SELECT enabled,probe_enabled,stills_enabled,revision FROM native_server_settings WHERE singleton`).Scan(&out.Settings.Enabled, &out.Settings.ProbeEnabled, &out.Settings.StillsEnabled, &out.Settings.Revision); err != nil {
		return out, err
	}
	counts := map[string]int64{}
	rows, err := r.DB.Query(ctx, `SELECT 'probe:'||state,count(*) FROM media_probe_results GROUP BY state UNION ALL SELECT 'still:'||state,count(*) FROM media_stills GROUP BY state`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var key string
		var n int64
		if err = rows.Scan(&key, &n); err != nil {
			break
		}
		counts[key] = n
	}
	err = errors.Join(err, rows.Err())
	rows.Close()
	if err != nil {
		return out, err
	}
	out.Probe = []StateCount{}
	out.Still = []StateCount{}
	for _, s := range States("probe") {
		out.Probe = append(out.Probe, StateCount{s, counts["probe:"+s]})
	}
	for _, s := range States("still") {
		out.Still = append(out.Still, StateCount{s, counts["still:"+s]})
	}
	u, err := r.Stills.Usage(ctx)
	out.Capacity = Capacity{u.SnapshotBytes, u.ReservedBytes, r.MaxBytes, u.UnknownSizes}
	if out.Capacity.LimitBytes == 0 {
		out.Capacity.LimitBytes = stillqueue.DefaultMaxBytes
	}
	return out, err
}
func (r Repository) Settings(parent context.Context, in Settings) (out Settings, err error) {
	if r.DB == nil || in.Revision < 1 {
		return out, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer func() {
		if e := tx.Rollback(ctx); err == nil && e != nil && !errors.Is(e, pgx.ErrTxClosed) {
			err = e
		}
	}()
	err = tx.QueryRow(ctx, `UPDATE native_server_settings SET enabled=$1,probe_enabled=$2,stills_enabled=$3,revision=revision+1,updated_at=now() WHERE singleton AND revision=$4 RETURNING revision`, in.Enabled, in.ProbeEnabled, in.StillsEnabled, in.Revision).Scan(&in.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrConflict
	}
	if err != nil {
		return out, err
	}
	if err = audit(ctx, tx, "native.processing.settings", in); err != nil {
		return out, err
	}
	err = tx.Commit(ctx)
	return in, err
}

type executor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func audit(ctx context.Context, q executor, action string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tag, err := q.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator',$1,$2)`, action, raw)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return err
}
func (r Repository) Audit(ctx context.Context, action string, value any) error {
	if r.DB == nil {
		return ErrInvalid
	}
	return audit(ctx, r.DB, action, value)
}

// Fixed SQL only; request values are always bind parameters. Title lookup is
// evaluated after pagination, not repeated over every result in the count.
const probeRows = `SELECT p.cloud_file_id::text key,p.state,p.attempts,p.next_attempt_at,coalesce(p.last_error_reason,'') reason FROM media_probe_results p`
const stillRows = `SELECT s.item_id::text key,s.state,s.attempts,s.next_attempt_at,coalesce(s.last_error_reason,'') reason FROM media_stills s`
const probeDisplay = `SELECT coalesce(a.id::text,'') id,coalesce(nullif(m.title,''),'未绑定媒体资产')||CASE WHEN m.media_type='series' THEN coalesce(' · S'||b.season||'E'||b.episode,'') ELSE '' END name FROM media_assets a LEFT JOIN media_asset_bindings b ON b.asset_id=a.id LEFT JOIN media_items m ON m.id=b.media_id WHERE a.cloud_file_id=p.key::uuid ORDER BY a.created_at DESC,a.id LIMIT 1`
const stillDisplay = `SELECT p.key id,coalesce(nullif(m.title,''),'单集 '||p.key)||coalesce(' · S'||b.season||'E'||b.episode,'') name FROM native_item_ids n LEFT JOIN media_stills s ON s.item_id=n.id LEFT JOIN media_asset_bindings b ON b.asset_id=s.asset_id LEFT JOIN media_items m ON m.id=b.media_id WHERE n.id=p.key::bigint LIMIT 1`

func (r Repository) List(parent context.Context, f Filter) (out Page, err error) {
	out.Items = []Task{}
	if r.DB == nil || !f.Valid() {
		return out, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	base, display := probeRows, probeDisplay
	if f.Kind == "still" {
		base, display = stillRows, stillDisplay
	}
	var raw []byte
	err = r.DB.QueryRow(ctx, `WITH filtered AS (`+base+` WHERE ($1::text='' OR state=$1)), page AS(SELECT * FROM filtered ORDER BY next_attempt_at NULLS LAST,key LIMIT $2 OFFSET $3)
 SELECT (SELECT count(*) FROM filtered),coalesce((SELECT jsonb_agg(jsonb_build_object('id',coalesce(d.id,''),'name',left(coalesce(d.name,'未绑定媒体资产'),512),'state',p.state,'attempts',p.attempts,'errorReason',p.reason,'nextAttemptAt',p.next_attempt_at) ORDER BY p.next_attempt_at NULLS LAST,p.key) FROM page p LEFT JOIN LATERAL (`+display+`) d ON true),'[]'::jsonb)`, f.State, f.PageSize, (f.Page-1)*f.PageSize).Scan(&out.Total, &raw)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(raw, &out.Items)
	out.Page = f.Page
	out.PageSize = f.PageSize
	return out, err
}
func (r Repository) Retry(parent context.Context, in Retry) (out Retried, err error) {
	f := Filter{Kind: in.Kind, State: in.State}
	if r.DB == nil || r.Stills == nil || !f.Valid() || in.Bulk && (!in.Confirmed || in.ID != "") || !in.Bulk && !validTarget(in.Kind, in.ID) {
		return out, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()
	var enabled bool
	err = r.DB.QueryRow(ctx, `SELECT enabled AND probe_enabled AND ($1::text='probe' OR stills_enabled) FROM native_server_settings WHERE singleton`, in.Kind).Scan(&enabled)
	if err != nil {
		return out, err
	}
	if !enabled {
		return out, probequeue.ErrDisabled
	}
	ids := []string{in.ID}
	if in.Bulk {
		base, display := probeRows, probeDisplay
		if in.Kind == "still" {
			base, display = stillRows, stillDisplay
		}
		rows, e := r.DB.Query(ctx, `WITH chosen AS(SELECT * FROM (`+base+`) x WHERE ($1::text='' OR state=$1) ORDER BY next_attempt_at NULLS LAST,key LIMIT 501) SELECT coalesce(d.id,'') FROM chosen p LEFT JOIN LATERAL (`+display+`) d ON true ORDER BY p.next_attempt_at NULLS LAST,p.key`, in.State)
		if e != nil {
			return out, e
		}
		ids = nil
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				break
			}
			ids = append(ids, id)
		}
		e = errors.Join(e, rows.Err())
		rows.Close()
		if e != nil {
			return out, e
		}
		out.LimitReached = len(ids) > MaxRetry
		if out.LimitReached {
			ids = ids[:MaxRetry]
		}
	}
	// Record intent before any partial work; each successful retry also writes its
	// existing transactional per-item audit. A batch never holds one long tx.
	if err = r.Audit(ctx, "native.processing.retry", in); err != nil {
		return out, err
	}
	out.Selected = len(ids)
	for _, id := range ids {
		var queued bool
		var e error
		if in.Kind == "probe" {
			queued, e = (probequeue.Repository{DB: r.DB}).Retry(ctx, id)
		} else {
			n, parse := strconv.ParseInt(id, 10, 64)
			if parse != nil {
				if in.Bulk {
					out.Skipped++
					continue
				}
				return out, ErrInvalid
			}
			queued, e = r.Stills.Retry(ctx, n)
		}
		if !in.Bulk && e != nil {
			return out, e
		}
		if e != nil {
			out.Failed++
		} else if queued {
			out.Queued++
		} else {
			out.Skipped++
		}
	}
	return out, nil
}
