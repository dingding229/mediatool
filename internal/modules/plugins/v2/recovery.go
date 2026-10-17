package v2

import (
	"context"
	"media-tool/internal/modules/plugins/v2/sandbox"
	"slices"
)

// References survive a crash/expired owner. A new generation cannot claim until
// the old process group has disappeared; database fencing alone cannot stop exec.
func (s Store) recoverProcesses(ctx context.Context, active ...string) error {
	rows, e := s.DB.Query(ctx, `SELECT id::text,worker_pid,worker_start FROM plugin_v2_instances WHERE worker_pid>0 AND (generation IS NULL OR lease_until<=now()) ORDER BY id LIMIT 200`)
	if e != nil {
		return e
	}
	type ref struct {
		id    string
		pid   int
		start string
	}
	refs := []ref{}
	for rows.Next() {
		var r ref
		if e = rows.Scan(&r.id, &r.pid, &r.start); e != nil {
			break
		}
		refs = append(refs, r)
	}
	other := rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if other != nil {
		return other
	}
	if len(refs) == 0 {
		return nil
	}
	all, e := sandbox.Snapshot()
	if e != nil {
		return e
	}
	for _, r := range refs {
		if e = ctx.Err(); e != nil {
			return e
		}
		if slices.Contains(active, r.id) {
			continue
		}
		rootMatches := false
		for _, p := range all {
			if p.PID == r.pid && p.Start == r.start {
				rootMatches = true
			}
		}
		// Do not signal a recycled PID. A live old group is attributable by its root
		// start time or the retained non-secret instance label on adopted descendants.
		owned := sandbox.Tagged(r.id, all)
		attributed := rootMatches
		for _, p := range owned {
			if p.Group == r.pid {
				attributed = true
			}
		}
		if attributed {
			if e = sandbox.KillGroup(r.pid); e != nil {
				return e
			}
		}
		for _, p := range owned {
			if e = sandbox.KillKnown(p); e != nil {
				return e
			}

		}
		if e = sandbox.ReapOwned(owned); e != nil {
			return e
		}
		remaining, e := sandbox.Snapshot()
		if e != nil {
			return e
		}
		alive := false
		for _, p := range remaining {
			if p.PID == r.pid && p.Start == r.start {
				alive = true
			}
			if attributed && p.Group == r.pid {
				alive = true
				if p.PID != r.pid {
					if e = sandbox.KillKnown(p); e != nil {
						return e
					}
					if e = sandbox.ReapKnown(p); e != nil {
						return e
					}
				}
			}
		}
		if alive {
			continue
		}
		_, e = s.DB.Exec(ctx, `UPDATE plugin_v2_instances SET worker_pid=0,worker_start='' WHERE id=$1 AND worker_pid=$2 AND worker_start=$3 AND (generation IS NULL OR lease_until<=now())`, r.id, r.pid, r.start)
		if e != nil {
			return e
		}
	}
	return nil
}
