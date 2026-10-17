package settings

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/singleflight"
	"media-tool/internal/modules/nativelibrary"
	"strings"
	"sync/atomic"
	"time"
)

const NativeServerCacheTTL = 250 * time.Millisecond
const nativeSettingsTimeout = 1500 * time.Millisecond

type nativeSnapshot struct {
	value   nativelibrary.ServerSettings
	expires time.Time
}

// A per-repository short read cache; no global lock or transaction on hot reads.
// Expired entries are never served after a read failure (including enabled=true).
type NativeServerRepository struct {
	read    func(context.Context) (nativelibrary.ServerSettings, error)
	now     func() time.Time
	cached  atomic.Pointer[nativeSnapshot]
	loading singleflight.Group
}

func NewNativeServerRepository(db *pgxpool.Pool) *NativeServerRepository {
	r := &NativeServerRepository{now: time.Now}
	r.read = func(ctx context.Context) (nativelibrary.ServerSettings, error) {
		var out nativelibrary.ServerSettings
		if db == nil {
			return out, nativelibrary.ErrSettingsUnavailable
		}
		err := db.QueryRow(ctx, `SELECT n.server_id,n.server_name,n.version,n.enabled,n.public_user_list,n.library_granularity,n.revision,
   coalesce(a.value#>>'{access,memberUrl}','') FROM native_server_settings n
   LEFT JOIN application_settings a ON a.singleton WHERE n.singleton`).Scan(&out.ServerID, &out.ServerName, &out.Version, &out.Enabled, &out.PublicUserList, &out.LibraryGranularity, &out.Revision, &out.PublicURL)
		if err != nil {
			return nativelibrary.ServerSettings{}, nativelibrary.ErrSettingsUnavailable
		}
		out.ServerID = strings.TrimSpace(out.ServerID)
		return out, nil
	}
	return r
}
func (r *NativeServerRepository) Get(ctx context.Context) (nativelibrary.ServerSettings, error) {
	if r == nil || r.read == nil || r.now == nil {
		return nativelibrary.ServerSettings{}, nativelibrary.ErrSettingsUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nativelibrary.ServerSettings{}, err
	}
	if cached := r.cached.Load(); cached != nil && r.now().Before(cached.expires) {
		return cached.value, nil
	}
	result := r.loading.DoChan("settings", func() (any, error) {
		if cached := r.cached.Load(); cached != nil && r.now().Before(cached.expires) {
			return cached.value, nil
		}
		bounded, cancel := context.WithTimeout(ctx, nativeSettingsTimeout)
		defer cancel()
		value, err := r.read(bounded)
		if err != nil {
			return nil, nativelibrary.ErrSettingsUnavailable
		}
		r.cached.Store(&nativeSnapshot{value, r.now().Add(NativeServerCacheTTL)})
		return value, nil
	})
	select {
	case <-ctx.Done():
		return nativelibrary.ServerSettings{}, ctx.Err()
	case response := <-result:
		if response.Err != nil {
			return nativelibrary.ServerSettings{}, response.Err
		}
		value, ok := response.Val.(nativelibrary.ServerSettings)
		if !ok {
			return nativelibrary.ServerSettings{}, nativelibrary.ErrSettingsUnavailable
		}
		return value, nil
	}
}
