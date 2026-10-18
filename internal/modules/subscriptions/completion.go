package subscriptions

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"media-tool/internal/modules/nativelibrary"
	nativecatalog "media-tool/internal/modules/nativelibrary/catalog"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/outbox"
	"media-tool/internal/platform/trace"
	"slices"
	"strconv"
	"time"
)

// CoverageObserver derives coverage of the subscription's configured season, not
// a downloader collection or a persistent MP "finished" state. Future/undated
// episodes remain expected. Oversized, empty or unavailable lists are unknown.
// No migration is needed: only bounded plugin-owned observations use Redis TTL.
type CoverageObserver struct {
	Engine Engine
	Cache  *redis.Client
}
type coverageQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (o CoverageObserver) ExpectedEpisodes(ctx context.Context, r Record) ([]int, error) {
	if r.Media.MediaType != "series" || r.Media.ID < 1 || r.Rules.Season < 0 || r.Rules.Season > 999 || o.Engine.Connections == nil {
		return nil, ErrCoverageUnknown
	}
	client, e := o.Engine.Connections.TMDB(ctx)
	if e != nil {
		return nil, ErrCoverageUnknown
	}
	season := r.Rules.Season
	// NativeDetails validates all episode tuples and retains future/undated
	// episodes; SeasonEpisodes intentionally filters aired episodes for downloads.
	details, e := client.NativeDetails(ctx, nativelibrary.TMDBTypeSeason, int64(r.Media.ID), &season, "CN")
	if e != nil || len(details.Episodes) == 0 || len(details.Episodes) > 200 {
		return nil, ErrCoverageUnknown
	}
	expected := []int{}
	for _, v := range details.Episodes {
		expected = append(expected, v.Number)
	}
	slices.Sort(expected)
	return expected, nil
}

var ErrCoverageUnknown = fmt.Errorf("subscription coverage unknown")

func ownedCoverage(ctx context.Context, db coverageQuery, r Record) ([]int, string, error) {
	// Aggregate in SQL, not a 50k asset scan in Go. Identity/revision digest fences
	// changes even if the same episode set is still exposed by another asset.
	var raw []byte
	var digest string
	e := db.QueryRow(ctx, `WITH live AS (`+nativecatalog.ExposableAssetsSQL+`), selected AS (SELECT * FROM live WHERE media_type='series' AND tmdb_id=$1 AND season=$2), eps AS (SELECT DISTINCT n::integer n FROM selected CROSS JOIN LATERAL jsonb_array_elements_text(CASE WHEN jsonb_array_length(episodes)>0 THEN episodes ELSE jsonb_build_array(episode) END) ep(n) WHERE n::integer>0) SELECT coalesce((SELECT jsonb_agg(n ORDER BY n) FROM (SELECT n FROM eps ORDER BY n LIMIT 201) x),'[]'::jsonb),coalesce((SELECT md5(string_agg(asset_id::text||':'||revision::text||':'||coalesce(episodes::text,'[]')||':'||coalesce(episode::text,''),',' ORDER BY asset_id)) FROM selected),'')`, r.Media.ID, r.Rules.Season).Scan(&raw, &digest)
	if e != nil {
		return nil, "", e
	}
	var owned []int
	if json.Unmarshal(raw, &owned) != nil || len(owned) > 200 {
		return nil, "", ErrCoverageUnknown
	}
	return owned, digest, nil
}
func unknownCoverage(r Record) contracts.CoverageObservation {
	return contracts.CoverageObservation{CompletionCheck: contracts.CompletionCheck{EpisodeExpectation: contracts.EpisodeExpectation{SubscriptionID: r.ID, Revision: r.Revision, TMDBID: r.Media.ID, Season: r.Rules.Season, Expected: []int{}}, Owned: []int{}}, State: "unknown", Evidence: "tmdb_season_exposable_assets", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}
}
func (o CoverageObserver) prepare(ctx context.Context, id string) (context.Context, Record, contracts.CoverageObservation, string, error) {
	r, e := o.Engine.Get(ctx, id)
	v := unknownCoverage(r)
	if e != nil {
		return ctx, r, v, "", e
	}
	if r.ArchivedAt != nil {
		return ctx, r, v, "", nil
	}
	expected, e := o.ExpectedEpisodes(ctx, r)
	if e != nil {
		return ctx, r, v, "", nil
	}
	v.Expected = expected
	store := pluginv2.Store{DB: o.Engine.DB, Vault: o.Engine.Connections.Repository.Vault}
	ctx, v.Expected, e = store.SupplementEpisodes(ctx, v.EpisodeExpectation)
	if e != nil {
		return ctx, r, unknownCoverage(r), "", e
	}
	owned, digest, e := ownedCoverage(ctx, o.Engine.DB, r)
	v.Owned = owned
	if e != nil {
		return ctx, r, unknownCoverage(r), "", e
	}
	v.Complete = len(v.Expected) > 0
	for _, n := range v.Expected {
		v.Complete = v.Complete && slices.Contains(v.Owned, n)
	}
	ctx, v.Complete, e = store.CheckCoverage(ctx, v.CompletionCheck)
	if e != nil {
		return ctx, r, unknownCoverage(r), "", e
	}
	slices.Sort(v.Expected)
	v.State = "partial"
	if v.Complete {
		v.State = "complete"
	}
	return ctx, r, v, digest, nil
}

// Check is read-only: no outbox, subscription update, enable/disable or invented
// MP state. Host authorization is checked live before and after this facade.
func (o CoverageObserver) Check(ctx context.Context, id string) (contracts.CoverageObservation, error) {
	prepared, _, v, _, e := o.prepare(ctx, id)
	if e == nil {
		e = dbutil.CheckPrepared(prepared, o.Engine.DB)
	}
	return v, e
}
func (o CoverageObserver) Observe(ctx context.Context, id string) error {
	ctx, r, v, digest, e := o.prepare(ctx, id)
	if e != nil {
		return e
	}
	tx, e := o.Engine.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer deferutil.Rollback(ctx, tx)
	var revision int64
	if e = tx.QueryRow(ctx, `SELECT revision FROM subscriptions WHERE id=$1 AND archived_at IS NULL FOR SHARE`, r.ID).Scan(&revision); e != nil {
		return e
	}
	if revision != r.Revision {
		return ErrCoverageUnknown
	}
	if v.Complete {
		// Lock the actual exposure rows only for the final evidence check/commit,
		// never over TMDB or RPC. A concurrent hide/missing/unbind must wait or
		// be observed before the completed fact can commit.
		if _, e = tx.Exec(ctx, `SELECT a.id FROM media_assets a JOIN media_asset_bindings b ON b.asset_id=a.id JOIN cloud_files f ON f.id=a.cloud_file_id JOIN media_items m ON m.id=b.media_id JOIN strm_entries s ON s.asset_id=a.id WHERE b.state='ready' AND f.state='present' AND m.media_type='series' AND m.tmdb_id=$1 AND b.season=$2 AND s.output_path<>'' AND lower(s.output_path) LIKE '%.strm' FOR SHARE OF a,b,f,m,s`, r.Media.ID, r.Rules.Season); e != nil {
			return e
		}
	}
	currentOwned, now, e := ownedCoverage(ctx, tx, r)
	if e != nil {
		return e
	}
	if (now != digest || !slices.Equal(currentOwned, v.Owned)) && v.State != "unknown" {
		return ErrCoverageUnknown
	}
	if v.Complete {
		raw, _ := json.Marshal(v.CompletionCheck)
		sum := sha256.Sum256(append(raw, []byte(digest)...))
		if e = outbox.EmitVersionTx(ctx, tx, "subscription.coverage.completed", r.ID, strconv.FormatInt(r.Revision, 10)+fmt.Sprintf(":%x", sum), v); e != nil {
			return e
		}
	}
	if e = dbutil.Commit(ctx, tx); e != nil {
		return e
	}
	if o.Cache != nil {
		raw, _ := json.Marshal(v)
		return o.Cache.Set(ctx, "plugins:v2:coverage:"+id, raw, 10*time.Minute).Err()
	}
	return nil
}

// Reconcile uses an expiring cursor and one lease per database/Redis deployment;
// each tick observes at most ten subscriptions and never stalls browsing.
func (o CoverageObserver) Reconcile(ctx context.Context) error {
	if o.Cache == nil {
		return nil
	}
	key := "plugins:v2:coverage-scan"
	token := strconv.FormatInt(time.Now().UnixNano(), 10)
	ok, e := o.Cache.SetNX(ctx, key+":lease", token, 50*time.Second).Result()
	if e != nil || !ok {
		return e
	}
	// Lease deletion is best-effort: TTL releases it after a cache outage.
	defer func() {
		_ = o.Cache.Eval(context.WithoutCancel(ctx), `if redis.call('get',KEYS[1])==ARGV[1] then return redis.call('del',KEYS[1]) end return 0`, []string{key + ":lease"}, token).Err()
	}()
	cursor, _ := o.Cache.Get(ctx, key+":cursor").Result()
	rows, e := o.Engine.DB.Query(ctx, `SELECT id::text FROM subscriptions WHERE archived_at IS NULL AND media_type='series' AND id::text>$1 ORDER BY id::text LIMIT 10`, cursor)
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			break
		}
		ids = append(ids, id)
	}
	other := rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if other != nil {
		return other
	}
	last := ""
	for _, id := range ids {
		bounded, cancel := context.WithTimeout(ctx, 4*time.Second)
		e = o.Observe(bounded, id)
		cancel()
		last = id // one unavailable identity must not starve the rest of the scan.
		if e != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			trace.Warn(ctx, "plugins", "覆盖观测暂不可用，将于后续扫描重试", "reason", "observation_deferred")
		}
	}
	if len(ids) < 10 {
		last = ""
	}
	return o.Cache.Set(ctx, key+":cursor", last, time.Hour).Err()
}
