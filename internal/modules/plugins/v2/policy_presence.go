package v2

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"sync"
	"time"
)

// Negative hints only; never cache a descriptor, grant, lease or accepted result.
// This prevents the new conversion hooks from adding two SQL calls per file in
// zero-plugin scans. Existing lifecycle invalidation also fences older readers.
var policyAbsence struct {
	sync.Mutex
	slots [128]struct {
		db    *pgxpool.Pool
		key   string
		epoch uint64
		until time.Time
	}
	next  int
	epoch uint64
}

func absentPolicy(db *pgxpool.Pool, key string) (uint64, bool) {
	policyAbsence.Lock()
	defer policyAbsence.Unlock()
	for i := range policyAbsence.slots {
		v := &policyAbsence.slots[i]
		if v.db == db && v.key == key {
			return v.epoch, time.Now().Before(v.until)
		}
	}
	i := policyAbsence.next
	policyAbsence.next = (i + 1) % len(policyAbsence.slots)
	policyAbsence.epoch++
	v := &policyAbsence.slots[i]
	v.db = db
	v.key = key
	v.epoch = policyAbsence.epoch
	v.until = time.Time{}
	return v.epoch, false
}
func rememberAbsentPolicy(db *pgxpool.Pool, key string, epoch uint64, until time.Time) {
	policyAbsence.Lock()
	defer policyAbsence.Unlock()
	for i := range policyAbsence.slots {
		v := &policyAbsence.slots[i]
		if v.db == db && v.key == key && v.epoch == epoch {
			v.until = until
			return
		}
	}
}
func invalidatePolicyAbsence(db *pgxpool.Pool) {
	policyAbsence.Lock()
	defer policyAbsence.Unlock()
	for i := range policyAbsence.slots {
		v := &policyAbsence.slots[i]
		if v.db == db {
			policyAbsence.epoch++
			v.epoch = policyAbsence.epoch
			v.until = time.Time{}
		}
	}
}
