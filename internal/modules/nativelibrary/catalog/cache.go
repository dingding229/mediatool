package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
)

const BrowseCacheTTL = 10 * time.Second
const browseCacheBytes = 2 << 20
const browseCacheBudget = 75 * time.Millisecond

// CachedRepository caches only projections, never PlaybackInfo, temporary URLs,
// credentials or permission decisions. Every request loads a fresh T42 scope.
// MVCC snapshot fencing invalidates on ANY committed DB write, including direct
// SQL and background probe/still writes, without relying on a best-effort Redis
// event. This is deliberately conservative (unrelated writes also miss).
type CachedRepository struct {
	BrowseRepository
	redis   *redis.Client
	flights singleflight.Group
	slots   chan struct{}
}

func NewCachedBrowse(base BrowseRepository, client *redis.Client) *CachedRepository {
	r := &CachedRepository{BrowseRepository: base, slots: make(chan struct{}, 4)}
	// A client that discards request deadlines is not allowed on the cache path.
	// Production cache.Open configures this; an injected legacy client bypasses.
	if client != nil && client.Options().ContextTimeoutEnabled {
		r.redis = client.WithTimeout(browseCacheBudget)
	}
	return r
}

var _ nativelibrary.BrowseService = (*CachedRepository)(nil)
var _ nativelibrary.DiscoveryService = (*CachedRepository)(nil)
var _ nativelibrary.PersonalService = (*CachedRepository)(nil)

func (r *CachedRepository) fence(ctx context.Context) (string, error) {
	if r.DB == nil {
		return "", contracts.ErrUnavailable
	}
	bounded, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	var database, schema, snapshot string
	// pg_current_snapshot does not allocate an xid for a read-only request. Its
	// xmin/xmax/in-progress set distinguishes late commits as well as new xids.
	if err := r.DB.QueryRow(bounded, `/* native.browse.fence */ SELECT current_database(),current_schema(),pg_current_snapshot()::text`).Scan(&database, &schema, &snapshot); err != nil {
		return "", contracts.ErrUnavailable
	}
	c := r.DB.Config().ConnConfig
	// xmin is only a conservative vacuum horizon; it can advance when a
	// read-only transaction ends without any visibility change. xmax plus the
	// in-progress xid set is the complete visibility fence, without that noise.
	_, visibility, ok := strings.Cut(snapshot, ":")
	if !ok {
		return "", contracts.ErrUnavailable
	}
	return fmt.Sprintf("%s:%d/%s/%s/%s", c.Host, c.Port, database, schema, visibility), nil
}
func (r *CachedRepository) key(ctx context.Context, u contracts.User, p contracts.PermissionChecker, request any) (string, string, error) {
	scope, err := scope(ctx, u, p)
	if err != nil {
		return "", "", err
	}
	stamp, err := r.fence(ctx)
	if err != nil {
		return "", "", err
	}
	raw, err := json.Marshal(struct {
		Version            int
		User               string
		Credential         int64
		Libraries, Ratings []string
		Unrated            bool
		Fence              string
		Request            any
	}{1, u.ID, u.CredentialVersion, scope.ids, scope.codes, scope.unrated, stamp, request})
	if err != nil {
		return "", "", nativelibrary.ErrInvalidQuery
	}
	digest := sha256.Sum256(raw)
	return "native:browse:v1:" + hex.EncodeToString(digest[:]), stamp, nil
}

const readBrowseCache = `if redis.call('TYPE',KEYS[1]).ok~='string' then return false end if redis.call('STRLEN',KEYS[1])>tonumber(ARGV[1]) then return false end return redis.call('GET',KEYS[1])`

func cachedRead[T any](ctx context.Context, r *CachedRepository, key string) (T, bool) {
	var out T
	bounded, cancel := context.WithTimeout(ctx, browseCacheBudget)
	defer cancel()
	text, err := r.redis.Eval(bounded, readBrowseCache, []string{key}, browseCacheBytes).Text()
	if err != nil || len(text) > browseCacheBytes {
		return out, false
	}
	d := json.NewDecoder(bytes.NewBufferString(text))
	d.DisallowUnknownFields()
	if err = d.Decode(&out); err != nil {
		return out, false
	}
	return out, true
}
func cachedRun[T any](ctx context.Context, r *CachedRepository, u contracts.User, p contracts.PermissionChecker, request any, load func(context.Context) (T, error)) (T, error) {
	if r.redis == nil {
		return load(ctx)
	}
	var zero T
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	key, stamp, err := r.key(ctx, u, p, request)
	if err != nil {
		return zero, err
	}
	if value, ok := cachedRead[T](ctx, r, key); ok {
		// Do not return a cached page if a write committed during the cache read.
		after, err := r.fence(ctx)
		if err != nil {
			return zero, err
		}
		if after == stamp {
			return value, nil
		}
	}
	wait := r.flights.DoChan(key, func() (any, error) {
		select {
		case r.slots <- struct{}{}:
			defer func() { <-r.slots }()
		default:
			return nil, contracts.ErrUnavailable
		}
		value, err := load(ctx)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, contracts.ErrUnavailable
		}
		// Do not label an old projection with a new epoch. A mutation or page-local
		// ID repair while loading means this result is returned but never cached.
		after, err := r.fence(ctx)
		if err == nil && after == stamp && len(raw) <= browseCacheBytes {
			bounded, done := context.WithTimeout(ctx, browseCacheBudget)
			// Cache failure is a supported degradation, never a failed browse request.
			if err := r.redis.Set(bounded, key, raw, BrowseCacheTTL).Err(); err != nil {
				if r.Logger != nil {
					r.Logger.Debug("Native browse cache write bypassed")
				}
			}
			done()
		}
		return &cacheFlight[T]{value: value, raw: raw}, nil
	})
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case result := <-wait:
		if result.Err != nil {
			return zero, result.Err
		}
		flight, ok := result.Val.(*cacheFlight[T])
		if !ok {
			return zero, contracts.ErrUnavailable
		}
		// Exactly one waiter may take the original graph. Others decode immutable
		// bytes; no shared maps/slices are returned, and a miss avoids a pointless
		// encode/decode roundtrip for the caller that already has the source DTO.
		if flight.taken.CompareAndSwap(false, true) {
			return flight.value, nil
		}
		var out T
		if err := json.Unmarshal(flight.raw, &out); err != nil {
			return zero, contracts.ErrUnavailable
		}
		return out, nil
	}
}

type cachedPage struct {
	Items []dto.BaseItemDto
	Total int
}

func (r *CachedRepository) Query(ctx context.Context, u contracts.User, p contracts.PermissionChecker, q nativelibrary.ItemQuery) ([]dto.BaseItemDto, int, error) {
	if err := validateBrowseQuery(&q); err != nil {
		return nil, 0, err
	}
	out, err := cachedRun(ctx, r, u, p, struct {
		Mode  string
		Query nativelibrary.ItemQuery
	}{"items", q}, func(ctx context.Context) (cachedPage, error) {
		items, total, err := r.BrowseRepository.Query(ctx, u, p, q)
		return cachedPage{items, total}, err
	})
	return out.Items, out.Total, err
}
func (r *CachedRepository) Views(ctx context.Context, u contracts.User, p contracts.PermissionChecker) ([]dto.BaseItemDto, error) {
	return cachedRun(ctx, r, u, p, "views", func(ctx context.Context) ([]dto.BaseItemDto, error) { return r.BrowseRepository.Views(ctx, u, p) })
}
func (r *CachedRepository) Counts(ctx context.Context, u contracts.User, p contracts.PermissionChecker, filters ...nativelibrary.ItemQuery) (nativelibrary.ItemCounts, error) {
	return cachedRun(ctx, r, u, p, struct {
		Mode    string
		Filters []nativelibrary.ItemQuery
	}{"counts", filters}, func(ctx context.Context) (nativelibrary.ItemCounts, error) {
		return r.BrowseRepository.Counts(ctx, u, p, filters...)
	})
}
func (r *CachedRepository) Discover(ctx context.Context, u contracts.User, p contracts.PermissionChecker, q nativelibrary.DiscoveryQuery) (nativelibrary.DiscoveryResult, error) {
	return cachedRun(ctx, r, u, p, q, func(ctx context.Context) (nativelibrary.DiscoveryResult, error) {
		return r.BrowseRepository.Discover(ctx, u, p, q)
	})
}

type cacheFlight[T any] struct {
	value T
	raw   []byte
	taken atomic.Bool
}
