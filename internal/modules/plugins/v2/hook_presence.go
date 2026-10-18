package v2

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"sync"
	"time"
)

const hookAbsenceTTL = 2 * time.Second

// A negative hint only: neither descriptors nor authorization are cached. The
// bounded pool-keyed slots isolate test schemas and retain at most 32 pools.
var hookAbsence struct {
	sync.Mutex
	slots [32]struct {
		db    *pgxpool.Pool
		epoch uint64
		until time.Time
	}
	next  int
	epoch uint64
}

func absentHooks(db *pgxpool.Pool) (uint64, bool) {
	hookAbsence.Lock()
	defer hookAbsence.Unlock()
	for i := range hookAbsence.slots {
		v := &hookAbsence.slots[i]
		if v.db == db {
			return v.epoch, time.Now().Before(v.until)
		}
	}
	i := hookAbsence.next
	hookAbsence.next = (i + 1) % len(hookAbsence.slots)
	hookAbsence.epoch++
	hookAbsence.slots[i].db = db
	hookAbsence.slots[i].epoch = hookAbsence.epoch
	hookAbsence.slots[i].until = time.Time{}
	return hookAbsence.epoch, false
}
func rememberAbsentHooks(db *pgxpool.Pool, epoch uint64, until time.Time) {
	hookAbsence.Lock()
	defer hookAbsence.Unlock()
	for i := range hookAbsence.slots {
		v := &hookAbsence.slots[i]
		if v.db == db && v.epoch == epoch {
			v.until = until
			return
		}
	}
}

// Called after lifecycle/publication/config changes (including uncertain commits).
// Epoch fencing prevents an older SELECT from refilling after invalidation.
func invalidateNameHooks(db *pgxpool.Pool) {
	invalidatePolicyAbsence(db)
	hookAbsence.Lock()
	defer hookAbsence.Unlock()
	for i := range hookAbsence.slots {
		v := &hookAbsence.slots[i]
		if v.db == db {
			hookAbsence.epoch++
			v.epoch = hookAbsence.epoch
			v.until = time.Time{}
			return
		}
	}
}

func (s Store) nameHooks(ctx context.Context) (map[string][]hook, error) {
	out := map[string][]hook{"chain": {}, "module": {}}
	if s.DB == nil {
		return out, ErrUnavailable
	}
	observed := time.Now()
	epoch, absent := absentHooks(s.DB)
	if absent {
		return out, nil
	}
	// One bounded query for both name extension points. Positive rows still take
	// the ordinary live Descriptor/grant/fence checks before every invocation.
	rows, e := s.DB.Query(ctx, `/* plugins:name-hooks */ SELECT i.id::text,d FROM plugin_v2_extensions x JOIN plugin_v2_snapshots p ON p.id=x.snapshot_id JOIN plugin_v2_instances i ON i.id=x.instance_id JOIN plugin_v2_definitions f ON f.id=i.plugin_id,jsonb_array_elements(p.descriptors->'descriptors') d WHERE i.desired_state='enabled' AND i.runtime_state='running' AND i.generation=p.generation AND i.grant_revision=p.grant_revision AND i.lease_until>clock_timestamp() AND i.approved_fingerprint=f.fingerprint AND f.availability='available' AND ((d->>'kind'='chain' AND d->>'id'='name.recognize') OR (d->>'kind'='module' AND d->>'id'='metadata.name_recognize')) ORDER BY coalesce((d->>'priority')::int,0) DESC,i.id,d->>'handler' LIMIT 17`)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var h hook
		var raw []byte
		if e = rows.Scan(&h.instance, &raw); e != nil {
			return out, e
		}
		if json.Unmarshal(raw, &h.descriptor) != nil {
			return out, ErrInvalid
		}
		kind := h.descriptor.Kind
		out[kind] = append(out[kind], h)
		if len(out[kind]) > 8 {
			return out, ErrUnavailable
		}
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out["chain"])+len(out["module"]) == 0 {
		rememberAbsentHooks(s.DB, epoch, observed.Add(hookAbsenceTTL))
	}
	return out, nil
}
