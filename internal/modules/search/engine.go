package search

import (
	"context"
	"encoding/json"
	"errors"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/sites"
	"media-tool/internal/platform/secrets"
	"media-tool/internal/platform/trace"
)

type Engine struct {
	Redis       *redis.Client
	DB          *pgxpool.Pool
	Vault       *secrets.Vault
	Connections *integrations.Manager
}
type Run struct {
	Media      json.RawMessage `json:"media"`
	ID         string          `json:"id"`
	Keyword    string          `json:"keyword"`
	State      string          `json:"state"`
	CreatedAt  time.Time       `json:"createdAt"`
	FinishedAt *time.Time      `json:"finishedAt,omitempty"`
	Sites      []SiteResult    `json:"sites"`
	Resources  []Resource      `json:"resources"`
	Total      int             `json:"total"`
	Excluded   int             `json:"excluded"`
}
type SiteResult struct {
	AccountID string `json:"accountId"`
	Name      string `json:"name"`
	State     string `json:"state"`
	Message   string `json:"message"`
	Count     int    `json:"count"`
}
type Resource struct {
	ID             string         `json:"id"`
	AccountID      string         `json:"accountId"`
	SiteName       string         `json:"siteName"`
	TorrentID      string         `json:"torrentId"`
	Title          string         `json:"title"`
	Size           int64          `json:"size"`
	Seeders        int            `json:"seeders"`
	Leechers       int            `json:"leechers"`
	DownloadFactor *float64       `json:"downloadFactor"`
	Metadata       map[string]any `json:"metadata"`
}

const maxSearchAccounts = 200
const searchConcurrency = 3

func validateAccountIDs(accountIDs []string) error {
	if len(accountIDs) == 0 || len(accountIDs) > maxSearchAccounts {
		return errors.New("select 1-200 enabled PT accounts")
	}
	seen := make(map[string]bool, len(accountIDs))
	for _, id := range accountIDs {
		if seen[id] {
			return errors.New("duplicate PT account")
		}
		seen[id] = true
	}
	return nil
}

func forEachAccount(ctx context.Context, accountIDs []string, visit func(string)) {
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, searchConcurrency)
	for _, account := range accountIDs {
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return
		}
		if ctx.Err() != nil {
			<-semaphore
			break
		}
		wg.Add(1)
		go func(account string) {
			defer wg.Done()
			defer func() { <-semaphore }()
			visit(account)
		}(account)
	}
	wg.Wait()
}

func (e Engine) Start(ctx context.Context, keyword string, accountIDs []string) (string, error) {
	return e.start(ctx, keyword, accountIDs, true, "")
}
func (e Engine) StartInline(ctx context.Context, keyword string, accountIDs []string) (string, error) {
	return e.start(ctx, keyword, accountIDs, false, "")
}
func (e Engine) start(ctx context.Context, keyword string, accountIDs []string, enqueue bool, cacheKey string, media ...metadata.Item) (string, error) {
	if keyword == "" || len(keyword) > 200 {
		return "", errors.New("enter a keyword up to 200 characters")
	}
	if err := validateAccountIDs(accountIDs); err != nil {
		return "", err
	}
	for _, id := range accountIDs {
		c, err := e.Connections.Repository.Get(ctx, id)
		if err != nil || c.Record.Kind != "ptsite" || !c.Record.Enabled {
			return "", errors.New("select enabled PT accounts")
		}
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer deferutil.Rollback(ctx, tx)
	id := jobs.UUID()
	accounts, _ := json.Marshal(accountIDs)
	mediaJSON := []byte("null")
	if len(media) > 0 {
		mediaJSON, _ = json.Marshal(media[0])
	}
	if _, err = tx.Exec(ctx, `INSERT INTO search_runs(id,keyword,account_ids,cache_key,media_context)VALUES($1,$2,$3,$4,$5)`, id, keyword, accounts, cacheKey, mediaJSON); err != nil {
		return "", err
	}
	for _, account := range accountIDs {
		if _, err = tx.Exec(ctx, `INSERT INTO search_site_results(run_id,account_id)VALUES($1,$2)`, id, account); err != nil {
			return "", err
		}
	}
	payload, _ := json.Marshal(map[string]string{"runId": id})
	if enqueue {
		if _, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,payload,idempotency_key)VALUES($1,'search.run',$2,$3)`, jobs.UUID(), payload, "search:"+id); err != nil {
			return "", err
		}
	}
	return id, dbutil.Commit(ctx, tx)
}
func (e Engine) List(ctx context.Context) ([]Run, error) { return e.Page(ctx, 0, 50, "", "") }
func (e Engine) Page(ctx context.Context, start, limit int, query, id string) ([]Run, error) {
	if start < 0 || start > 200000 || limit < 1 || limit > 200 || len(query) > 800 {
		return nil, errors.New("invalid page")
	}
	rows, err := e.DB.Query(ctx, `SELECT id::text,keyword,state,created_at,finished_at,media_context,(SELECT count(*) FROM search_run_resources WHERE run_id=r.id) FROM search_runs r WHERE NOT cache_hidden AND ($3='' OR keyword ILIKE '%'||$3||'%') AND ($4='' OR r.id::text=$4) ORDER BY created_at DESC,id LIMIT $1 OFFSET $2`, limit, start, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		var run Run
		if err = rows.Scan(&run.ID, &run.Keyword, &run.State, &run.CreatedAt, &run.FinishedAt, &run.Media, &run.Total); err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}
func (e Engine) Get(ctx context.Context, id string, page, limit int) (Run, error) {
	var run Run
	err := e.DB.QueryRow(ctx, `SELECT id::text,keyword,state,created_at,finished_at,media_context FROM search_runs WHERE id=$1`, id).Scan(&run.ID, &run.Keyword, &run.State, &run.CreatedAt, &run.FinishedAt, &run.Media)
	if err != nil {
		return run, errors.New("search run unavailable")
	}
	run.Sites = []SiteResult{}
	run.Resources = []Resource{}
	rows, err := e.DB.Query(ctx, `SELECT s.account_id::text,i.name,s.state,s.message,s.result_count FROM search_site_results s JOIN integrations i ON i.id=s.account_id WHERE run_id=$1 ORDER BY i.name`, id)
	if err != nil {
		return run, err
	}
	for rows.Next() {
		var site SiteResult
		if err = rows.Scan(&site.AccountID, &site.Name, &site.State, &site.Message, &site.Count); err != nil {
			rows.Close()
			return run, err
		}
		run.Sites = append(run.Sites, site)
	}
	rows.Close()
	if err = e.DB.QueryRow(ctx, `SELECT count(*) FROM search_run_resources WHERE run_id=$1`, id).Scan(&run.Total); err != nil {
		return run, err
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}
	limit = min(limit, 200)
	rows, err = e.DB.Query(ctx, `SELECT link.public_snapshot FROM search_run_resources link WHERE link.run_id=$1 ORDER BY (link.public_snapshot->>'seeders')::integer DESC,link.public_snapshot->>'title',link.resource_id LIMIT $2 OFFSET $3`, id, limit, (page-1)*limit)
	if err != nil {
		return run, err
	}
	defer rows.Close()
	for rows.Next() {
		var resource Resource
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return run, err
		}
		if len(raw) == 0 || json.Unmarshal(raw, &resource) != nil {
			return run, ErrSnapshotUnavailable
		}
		run.Resources = append(run.Resources, resource)
	}
	return run, rows.Err()
}
func (e Engine) Process(ctx context.Context, job jobs.Job) error {
	var payload struct {
		RunID string `json:"runId"`
	}
	if json.Unmarshal(job.Payload, &payload) != nil {
		return errors.New("invalid search job")
	}
	var frozen bool
	if err := e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM subscription_runs WHERE search_run_id=$1 AND collection_state<>'collecting') OR EXISTS(SELECT 1 FROM download_tasks WHERE details->>'searchRunId'=$1::text)`, payload.RunID).Scan(&frozen); err != nil {
		return err
	}
	if frozen {
		return ErrSnapshotChanged
	}
	var keyword string
	var raw []byte
	if err := e.DB.QueryRow(ctx, `SELECT keyword,account_ids FROM search_runs WHERE id=$1`, payload.RunID).Scan(&keyword, &raw); err != nil {
		return err
	}
	var accountIDs []string
	if json.Unmarshal(raw, &accountIDs) != nil {
		return errors.New("invalid account list")
	}
	if _, err := e.DB.Exec(ctx, `UPDATE search_runs SET state='running' WHERE id=$1`, payload.RunID); err != nil {
		return err
	}
	forEachAccount(ctx, accountIDs, func(account string) {
		e.runSite(ctx, payload.RunID, keyword, account)
	})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var successful int
	if err := e.DB.QueryRow(ctx, `SELECT count(*) FROM search_site_results WHERE run_id=$1 AND state IN('succeeded','partial')`, payload.RunID).Scan(&successful); err != nil {
		return err
	}
	var incomplete int
	if err := e.DB.QueryRow(ctx, `SELECT count(*) FROM search_site_results WHERE run_id=$1 AND state='partial'`, payload.RunID).Scan(&incomplete); err != nil {
		return err
	}
	state := "succeeded"
	if successful == 0 {
		state = "failed"
	} else if successful < len(accountIDs) || incomplete > 0 {
		state = "partial"
	}
	_, err := e.DB.Exec(ctx, `UPDATE search_runs SET state=$2,finished_at=now() WHERE id=$1`, payload.RunID, state)
	if err == nil {
		e.rememberCache(ctx, payload.RunID)
	}
	return err
}
func (e Engine) runSite(ctx context.Context, runID, keyword, accountID string) {
	if err := e.beginSiteAttempt(ctx, runID, accountID); err != nil {
		return
	}
	client, err := e.Connections.Site(ctx, accountID)
	result := sites.SearchResult{Resources: []sites.Resource{}}
	if err == nil {
		bounded, cancel := context.WithTimeout(ctx, 45*time.Second)
		result, err = client.SearchAll(bounded, sites.SearchQuery{Keyword: keyword, Page: 1})
		cancel()
	}
	if err == nil {
		err = e.SaveResourcesWithClient(ctx, runID, accountID, result.Resources, client)
	}
	state, message := "succeeded", ""
	if err != nil {
		state = "failed"
		message = "站点请求或解析失败，请检查配置兼容性、登录凭据和网络"
		if errors.Is(err, sites.ErrAuthentication) {
			message = "站点登录状态未验证，请更新 Cookie/API Key"
		}
		if errors.Is(err, sites.ErrConfig) {
			message = "站点配置存在未支持或无效的请求/解析规则"
		}
		if errors.Is(err, sites.ErrSearchResponse) {
			message = "站点返回无效的声明结果列表；不将其视为成功空搜索"
		}
	} else if !result.Complete {
		state = "partial"
		switch result.Reason {
		case "invalid_next_page":
			message = "下一页链接不满足同源同查询边界；只保留已取得结果"
		case "single_page_contract":
			message = "站点定义仅声明单页请求；本次只查询该页，未声明其他页面已检索"
		case "page_limit":
			message = "达到5页查询上限；仅显示已取得的部分结果，不代表站点无更多资源"
		case "resource_limit":
			message = "达到500项资源上限；仅显示已取得的部分结果"
		case "repeated_page":
			message = "站点返回重复页面或忽略页码；已停止重复请求，仅显示部分结果"
		case "later_page_failed":
			message = "后续页面请求或解析失败；保留已验证页面，不声明结果完整"
		default:
			message = "搜索结果未完整取得，请核对站点分页与接口版本"
		}
	}
	trace.Info(ctx, "search", "站点搜索结果", "searchRunId", runID, "accountId", accountID, "state", state, "pages", result.Pages, "complete", result.Complete, "reason", result.Reason, "message", message)
	count := len(result.Resources)
	if err != nil {
		count = 0
	}
	if _, checkedErr := e.DB.Exec(ctx, `UPDATE search_site_results SET state=$3,message=$4,result_count=$5 WHERE run_id=$1 AND account_id=$2`, runID, accountID, state, message, count); checkedErr != nil {
		deferutil.Report(checkedErr, "internal/modules/search/engine.go:runSite")
		return
	}
}

// A retry replaces this account's attempt, never unions a previous attempt's
// removed rows with new pages. It cannot mutate a frozen subscription run.
func (e Engine) beginSiteAttempt(ctx context.Context, runID, accountID string) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM search_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&locked); err != nil {
		return err
	}
	var frozen bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM subscription_runs WHERE search_run_id=$1 AND collection_state<>'collecting') OR EXISTS(SELECT 1 FROM download_tasks WHERE details->>'searchRunId'=$1::text)`, runID).Scan(&frozen); err != nil {
		return err
	}
	if frozen {
		return ErrSnapshotChanged
	}
	if _, err = tx.Exec(ctx, `DELETE FROM search_run_resources l USING resources r WHERE l.resource_id=r.id AND l.run_id=$1 AND r.account_id=$2`, runID, accountID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE search_site_results SET state='running',message='',result_count=0,source_revision=NULL,definition_fingerprint=NULL,observed_at=NULL WHERE run_id=$1 AND account_id=$2`, runID, accountID); err != nil {
		return err
	}
	return dbutil.Commit(ctx, tx)
}

func (e Engine) saveResources(ctx context.Context, runID, accountID string, resources []sites.Resource) error {
	return e.saveResourcesWithSource(ctx, runID, accountID, resources, 0, "")
}

// Source proof comes from the client used for HTTP, not from a later account read.
func (e Engine) SaveResourcesWithClient(ctx context.Context, runID, accountID string, resources []sites.Resource, client sites.Client) error {
	if client.SourceRevision < 1 || client.DefinitionFingerprint == "" {
		return ErrSnapshotUnavailable
	}
	return e.saveResourcesWithSource(ctx, runID, accountID, resources, client.SourceRevision, client.DefinitionFingerprint)
}

func (e Engine) saveResourcesWithSource(ctx context.Context, runID, accountID string, resources []sites.Resource, expectedRevision int64, expectedFingerprint string) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var lockedRun string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM search_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&lockedRun); err != nil {
		return err
	}
	var frozen bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM subscription_runs WHERE search_run_id=$1 AND collection_state<>'collecting') OR EXISTS(SELECT 1 FROM download_tasks WHERE details->>'searchRunId'=$1::text)`, runID).Scan(&frozen); err != nil {
		return err
	}
	if frozen {
		return ErrSnapshotChanged
	}
	if expectedRevision != 0 {
		revision, fingerprint, err := sourceProof(ctx, tx, accountID)
		if err != nil || revision != expectedRevision || fingerprint != expectedFingerprint {
			return ErrSnapshotChanged
		}
		var priorRevision *int64
		var priorFingerprint *string
		var priorObserved *time.Time
		err = tx.QueryRow(ctx, `SELECT source_revision,definition_fingerprint,observed_at FROM search_site_results WHERE run_id=$1 AND account_id=$2 FOR UPDATE`, runID, accountID).Scan(&priorRevision, &priorFingerprint, &priorObserved)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSnapshotUnavailable
		}
		if err != nil {
			return err
		}
		if priorRevision != nil || priorFingerprint != nil || priorObserved != nil {
			if priorRevision == nil || priorFingerprint == nil || priorObserved == nil {
				return ErrSnapshotUnavailable
			}
			// Search and RSS share one site proof. Never replace earlier empty
			// evidence with a later account/definition, even without resource rows.
			if *priorRevision != revision || *priorFingerprint != fingerprint {
				return ErrSnapshotChanged
			}
		}
		// Same-origin RSS must not refresh the age of an earlier search proof.
		if _, err = tx.Exec(ctx, `UPDATE search_site_results SET source_revision=$3,definition_fingerprint=$4,observed_at=coalesce(observed_at,now()) WHERE run_id=$1 AND account_id=$2`, runID, accountID, revision, fingerprint); err != nil {
			return err
		}
	}
	for _, item := range resources {
		if err = e.saveResourceTx(ctx, tx, runID, accountID, item); err != nil {
			return err
		}
	}
	return dbutil.Commit(ctx, tx)
}

func publicURL(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil {
		return ""
	}
	for key := range parsed.Query() {
		k := strings.ToLower(key)
		if strings.Contains(k, "token") || strings.Contains(k, "key") || strings.Contains(k, "auth") || strings.Contains(k, "sign") {
			return ""
		}
	}
	return parsed.String()
}

func (e Engine) Torrent(ctx context.Context, resourceID string) ([]byte, error) {
	var accountID, torrentID string
	var encrypted []byte
	if err := e.DB.QueryRow(ctx, `SELECT account_id::text,torrent_id,download_hint_ciphertext FROM resources WHERE id=$1`, resourceID).Scan(&accountID, &torrentID, &encrypted); err != nil {
		return nil, errors.New("resource unavailable")
	}
	hint, err := e.Vault.Open(resourceID, encrypted)
	if err != nil {
		return nil, err
	}
	client, err := e.Connections.Site(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return client.Torrent(ctx, torrentID, hint["url"])
}

func (e Engine) SaveResources(ctx context.Context, runID, accountID string, items []sites.Resource) error {
	return e.saveResources(ctx, runID, accountID, items)
}
