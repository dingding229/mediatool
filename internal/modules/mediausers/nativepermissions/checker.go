// Package nativepermissions owns request-scoped native media authorization.
// New must be called once per HTTP request; no snapshot crosses requests.
package nativepermissions

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/mediausers/contracts"
)

type grant struct {
	ID             string
	Play, Download bool
}
type snapshot struct {
	User   contracts.User
	Grants []grant
}
type entry struct {
	once  sync.Once
	value snapshot
	err   error
}
type Checker struct {
	load  func(context.Context, contracts.User) (snapshot, error)
	count func(context.Context, contracts.User) (int, error)
	now   func() time.Time
	mu    sync.Mutex // Per request only. Never held during SQL or external work.
	users map[string]*entry
}

var _ contracts.PermissionChecker = (*Checker)(nil)

func New(db *pgxpool.Pool) *Checker {
	c := &Checker{now: time.Now, users: map[string]*entry{}}
	c.load = func(ctx context.Context, u contracts.User) (snapshot, error) {
		var out snapshot
		if db == nil {
			return out, contracts.ErrUnavailable
		}
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		var raw []byte
		err := db.QueryRow(ctx, `SELECT replace(u.id::text,'-',''),u.enabled,u.must_change_password,u.credential_version,u.max_streams,u.expires_at,
   coalesce((SELECT jsonb_agg(row_to_json(g)) FROM (SELECT l.id AS "ID",g.can_play AS "Play",g.can_download AS "Download" FROM media_user_library_grants g JOIN native_libraries l ON l.id=g.library_id WHERE g.user_id=u.id AND NOT l.hidden ORDER BY l.id LIMIT 201) g),'[]'::jsonb)
   FROM media_users u WHERE u.id=$1::uuid`, u.ID).Scan(&out.User.ID, &out.User.Enabled, &out.User.MustChangePassword, &out.User.CredentialVersion, &out.User.MaxStreams, &out.User.ExpiresAt, &raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return out, contracts.ErrForbidden
		}
		if err != nil {
			return out, contracts.ErrUnavailable
		}
		if err = json.Unmarshal(raw, &out.Grants); err != nil || len(out.Grants) > 200 {
			return out, contracts.ErrUnavailable
		}
		return out, nil
	}
	c.count = func(ctx context.Context, u contracts.User) (int, error) {
		if db == nil {
			return 0, contracts.ErrUnavailable
		}
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		var n int
		err := db.QueryRow(ctx, `SELECT count(*) FROM native_playback_sessions WHERE user_id=$1::uuid AND state<>'stopped' AND last_progress_at>=now()-interval '5 minutes'`, u.ID).Scan(&n)
		if err != nil {
			return 0, contracts.ErrUnavailable
		}
		return n, nil
	}
	return c
}
func active(u contracts.User, now time.Time) bool {
	return u.ID != "" && u.Enabled && !u.MustChangePassword && (u.ExpiresAt == nil || u.ExpiresAt.After(now))
}
func (c *Checker) get(ctx context.Context, u contracts.User) (snapshot, error) {
	if !active(u, c.now()) {
		return snapshot{}, contracts.ErrForbidden
	}
	c.mu.Lock()
	e := c.users[u.ID]
	if e == nil {
		e = &entry{}
		c.users[u.ID] = e
	}
	c.mu.Unlock()
	e.once.Do(func() { e.value, e.err = c.load(ctx, u) })
	if e.err != nil {
		return snapshot{}, e.err
	}
	if !active(e.value.User, c.now()) || e.value.User.CredentialVersion != u.CredentialVersion {
		return snapshot{}, contracts.ErrForbidden
	}
	return e.value, nil
}
func (c *Checker) VisibleLibraries(ctx context.Context, u contracts.User) ([]string, error) {
	s, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(s.Grants))
	for _, g := range s.Grants {
		ids = append(ids, g.ID)
	}
	return ids, nil
}
func (c *Checker) allowed(ctx context.Context, u contracts.User, id string, mode int) error {
	s, err := c.get(ctx, u)
	if err != nil {
		return err
	}
	for _, g := range s.Grants {
		if g.ID == id && (mode == 0 || g.Play && (mode == 1 || g.Download)) {
			return nil
		}
	}
	return contracts.ErrForbidden
}
func (c *Checker) CanBrowse(ctx context.Context, u contracts.User, id string) error {
	return c.allowed(ctx, u, id, 0)
}
func (c *Checker) CanPlay(ctx context.Context, u contracts.User, id string) error {
	return c.allowed(ctx, u, id, 1)
}
func (c *Checker) CanDownload(ctx context.Context, u contracts.User, id string) error {
	return c.allowed(ctx, u, id, 2)
}
func (c *Checker) ItemAllowed(ctx context.Context, u contracts.User, item contracts.ItemAccess) error {
	s, err := c.get(ctx, u)
	if err != nil {
		return err
	}
	for _, g := range s.Grants {
		if g.ID == item.LibraryID {
			return nil
		}
		for _, id := range item.LibraryIDs {
			if g.ID == id {
				return nil
			}
		}
	}
	return contracts.ErrForbidden
}
func (c *Checker) StreamSlots(ctx context.Context, u contracts.User) (contracts.StreamCapacity, error) {
	s, err := c.get(ctx, u)
	if err != nil {
		return contracts.StreamCapacity{}, err
	}
	n, err := c.count(ctx, s.User)
	if err != nil {
		return contracts.StreamCapacity{}, err
	}
	out := contracts.StreamCapacity{Limit: s.User.MaxStreams, InUse: n}
	if n >= out.Limit {
		return out, contracts.ErrStreamLimit
	}
	return out, nil
}
