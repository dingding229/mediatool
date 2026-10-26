package externalapi

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"media-tool/internal/modules/maintenance"
	deferutil "media-tool/internal/platform/cleanup"
	"sync/atomic"
	"time"
)

const CallRetentionDays = 30
const CallsPerKey = 1000

type Observation struct {
	KeyID      string
	CallID     int64
	IP, Source string
	Status     int
}
type Recorder struct {
	DB      *pgxpool.Pool
	Log     *slog.Logger
	queue   chan Observation
	dropped atomic.Uint64
}

func NewRecorder(db *pgxpool.Pool, log *slog.Logger) *Recorder {
	if log == nil {
		log = slog.Default()
	}
	return &Recorder{DB: db, Log: log, queue: make(chan Observation, 2048)}
}
func (r *Recorder) Submit(v Observation) {
	if r == nil {
		return
	}
	select {
	case r.queue <- v:
	default:
		r.dropped.Add(1)
	}
}

// Flush only updates rows admitted synchronously for authorization/rate limiting.
// A delete/revoke cannot be undone by a late async observation. No token enters this queue.
func (r *Recorder) Flush(ctx context.Context, items []Observation) error {
	if len(items) == 0 {
		return nil
	}
	release, e := maintenance.BeginWork(ctx, r.DB)
	if e != nil {
		return e
	}
	defer release()
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer deferutil.Rollback(ctx, tx)
	batch := &pgx.Batch{}
	for _, v := range items {
		batch.Queue(`UPDATE external_api_keys k SET last_ip=nullif($3,'')::inet,last_ip_source=$4,last_ip_at=c.created_at FROM external_api_calls c WHERE k.id=$1::uuid AND c.key_id=k.id AND c.id=$2 AND (k.last_ip_at IS NULL OR k.last_ip_at<=c.created_at)`, v.KeyID, v.CallID, v.IP, v.Source)
		batch.Queue(`UPDATE external_api_calls SET status=$3,client_ip=nullif($4,'')::inet,ip_source=$5 WHERE id=$1 AND key_id=$2::uuid`, v.CallID, v.KeyID, v.Status, v.IP, v.Source)
	}
	result := tx.SendBatch(ctx, batch)
	for range items {
		for range 2 {
			if _, e = result.Exec(); e != nil {
				_ = result.Close()
				return e
			}
		}
	}
	if e = result.Close(); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (r *Recorder) Prune(ctx context.Context) error {
	release, e := maintenance.BeginWork(ctx, r.DB)
	if e != nil {
		return e
	}
	defer release()
	// Never remove the rolling one-minute rate window. Batch deletion is bounded.
	_, e = r.DB.Exec(ctx, `DELETE FROM external_api_calls WHERE id IN(SELECT id FROM(SELECT id,created_at,row_number()OVER(PARTITION BY key_id ORDER BY created_at DESC,id DESC) AS ordinal FROM external_api_calls)q WHERE created_at<now()-interval '1 minute' AND(created_at<now()-interval '30 days' OR ordinal>1000)LIMIT 1000)`)
	if e != nil {
		return e
	}
	_, e = r.DB.Exec(ctx, `UPDATE external_api_keys SET last_ip=NULL,last_ip_source='',last_ip_at=NULL WHERE last_ip_at<now()-interval '30 days'`)
	return e
}
func (r *Recorder) Run(ctx context.Context) {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	pending := make([]Observation, 0, 256)
	ticks := 0
	flush := func(parent context.Context) {
		if len(pending) == 0 {
			return
		}
		c, cancel := context.WithTimeout(parent, 5*time.Second)
		defer cancel()
		if e := r.Flush(c, pending); e != nil {
			if errors.Is(e, maintenance.ErrConflict) {
				return
			}
			r.Log.Warn("外部API调用IP记录暂缓，将重试", "module", "externalapi")
			return
		}
		pending = pending[:0]
	}
	for {
		select {
		case v := <-r.queue:
			if len(pending) < 256 {
				pending = append(pending, v)
			} else {
				r.dropped.Add(1)
			}
			if len(pending) >= 256 {
				flush(ctx)
			}
		case <-timer.C:
			flush(ctx)
			ticks++
			if n := r.dropped.Swap(0); n > 0 {
				r.Log.Warn("外部API调用记录队列已满，部分记录未写入", "module", "externalapi", "dropped", n)
			}
			if ticks%60 == 0 {
				c, cancel := context.WithTimeout(ctx, 5*time.Second)
				if e := r.Prune(c); e != nil {
					r.Log.Warn("外部API调用记录清理暂缓", "module", "externalapi")
				}
				cancel()
			}
		case <-ctx.Done():
			for len(pending) < 256 {
				select {
				case v := <-r.queue:
					pending = append(pending, v)
				default:
					flush(context.WithoutCancel(ctx))
					return
				}
			}
			flush(context.WithoutCancel(ctx))
			return
		}
	}
}
