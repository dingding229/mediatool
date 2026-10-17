package subscriptions

import (
	"context"
	"encoding/json"
	"errors"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"sort"
	"strconv"

	"media-tool/internal/modules/acquisition"
	"media-tool/internal/modules/downloads"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/search"
	"media-tool/internal/modules/sites"
	"media-tool/internal/modules/subscriptionpolicy"
	"media-tool/internal/platform/trace"
)

type DecisionRecord struct {
	Checks         []subscriptionpolicy.RuleCheck `json:"checks"`
	ID             string                         `json:"id"`
	ResourceID     *string                        `json:"resourceId,omitempty"`
	Title          string                         `json:"title"`
	Action         string                         `json:"action"`
	Reasons        []string                       `json:"reasons"`
	Episodes       []int                          `json:"episodes"`
	DownloadTaskID *string                        `json:"downloadTaskId,omitempty"`
}
type RunRecord struct {
	ID             string           `json:"id"`
	SubscriptionID string           `json:"subscriptionId"`
	State          string           `json:"state"`
	Preview        bool             `json:"preview"`
	Decisions      []DecisionRecord `json:"decisions"`
}

func (e Engine) Run(ctx context.Context, id string) (RunRecord, error) {
	var run RunRecord
	if err := e.DB.QueryRow(ctx, `SELECT id::text,subscription_id::text,state,preview FROM subscription_runs WHERE id=$1`, id).Scan(&run.ID, &run.SubscriptionID, &run.State, &run.Preview); err != nil {
		return run, err
	}
	run.Decisions = []DecisionRecord{}
	rows, err := e.DB.Query(ctx, `SELECT id::text,resource_id::text,title,action,reasons,covered_episodes,download_task_id::text,rule_checks FROM subscription_decisions WHERE run_id=$1 ORDER BY created_at,id`, id)
	if err != nil {
		return run, err
	}
	defer rows.Close()
	for rows.Next() {
		var decision DecisionRecord
		var reasons, episodes, checks []byte
		if err = rows.Scan(&decision.ID, &decision.ResourceID, &decision.Title, &decision.Action, &reasons, &episodes, &decision.DownloadTaskID, &checks); err != nil {
			return run, err
		}
		if checkedErr := json.Unmarshal(reasons, &decision.Reasons); checkedErr != nil {
			return RunRecord{}, checkedErr
		}
		if checkedErr := json.Unmarshal(episodes, &decision.Episodes); checkedErr != nil {
			return RunRecord{}, checkedErr
		}
		if checkedErr := json.Unmarshal(checks, &decision.Checks); checkedErr != nil {
			return RunRecord{}, checkedErr
		}
		run.Decisions = append(run.Decisions, decision)
	}
	return run, rows.Err()
}
func (e Engine) Process(ctx context.Context, job jobs.Job) (result error) {
	var payload struct {
		RunID string `json:"runId"`
	}
	if json.Unmarshal(job.Payload, &payload) != nil {
		return errors.New("invalid subscription job")
	}
	run, err := e.Run(ctx, payload.RunID)
	if err != nil {
		return err
	}
	if run.State == "succeeded" || run.State == "cancelled" {
		return nil
	}
	record, err := e.Get(ctx, run.SubscriptionID)
	if err != nil {
		return err
	}
	lockConn, err := e.DB.Acquire(ctx)
	if err != nil {
		return err
	}
	defer lockConn.Release()
	trace.Info(ctx, "subscriptions", "订阅评估开始", "subscriptionId", record.ID, "subscriptionRunId", run.ID, "title", record.Title, "preview", run.Preview, "rules", record.Rules)
	lockKey := record.Media.MediaType + ":" + strconv.Itoa(record.Media.ID)
	if _, err = lockConn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1,0))`, "subscription-media:"+lockKey); err != nil {
		return err
	}
	defer dbutil.Unlock(context.Background(), lockConn, `SELECT pg_advisory_unlock(hashtextextended($1,0))`, "subscription-media:"+lockKey)
	// Another worker/editor may have finished or changed this run while we waited.
	run, err = e.Run(ctx, payload.RunID)
	if err != nil {
		return err
	}
	if run.State == "succeeded" || run.State == "cancelled" {
		return nil
	}
	if run.State == "needs_refresh" {
		return errors.Join(jobs.ErrOperatorRequired, ErrEvaluationRefresh)
	}
	record, err = e.Get(ctx, run.SubscriptionID)
	if err != nil {
		return err
	}
	var revision int64
	if err = e.DB.QueryRow(ctx, `SELECT revision FROM subscription_runs WHERE id=$1`, run.ID).Scan(&revision); err != nil {
		return err
	}
	if revision != record.Revision || !record.Enabled && !run.Preview {
		_, err = e.DB.Exec(ctx, `UPDATE subscription_runs SET state='cancelled',finished_at=now()WHERE id=$1`, run.ID)
		return err
	}
	if _, err = e.DB.Exec(ctx, `UPDATE subscription_runs SET state='running'WHERE id=$1`, run.ID); err != nil {
		return err
	}
	defer func() {
		if result != nil {
			message := "订阅评估失败，请检查站点、TMDB、规则和后台任务"
			if errors.Is(result, sites.ErrFeed) {
				message = "RSS响应无效或种子标识不支持，请检查站点登录和RSS链接；未更新订阅基线"
			}
			if errors.Is(result, jobs.ErrOperatorRequired) {
				message = ErrEvaluationRefresh.Error()
			}
			if _, checkedErr := e.DB.Exec(context.WithoutCancel(ctx), `UPDATE subscriptions SET last_state='failed',last_message=$2 WHERE id=$1`, record.ID, message); checkedErr != nil {
				result = errors.Join(result, checkedErr)
				return
			}
			if job.Attempts >= job.MaxAttempts && !errors.Is(result, jobs.ErrOperatorRequired) {
				if _, checkedErr := e.DB.Exec(context.WithoutCancel(ctx), `UPDATE subscription_runs SET state='failed',finished_at=now()WHERE id=$1`, run.ID); checkedErr != nil {
					result = errors.Join(result, checkedErr)
					return
				}
			}
		}
	}()
	searcher := search.Engine{DB: e.DB, Vault: e.Connections.Repository.Vault, Connections: e.Connections}
	var searchRunID string
	if checkedErr := e.DB.QueryRow(ctx, `SELECT coalesce(search_run_id::text,'')FROM subscription_runs WHERE id=$1`, run.ID).Scan(&searchRunID); checkedErr != nil {
		return checkedErr
	}
	if searchRunID == "" {
		searchRunID, err = searcher.StartInline(ctx, record.Rules.Keyword, record.Rules.AccountIDs)
		if err != nil {
			return err
		}
		if _, err = e.DB.Exec(ctx, `UPDATE subscription_runs SET search_run_id=$2 WHERE id=$1`, run.ID, searchRunID); err != nil {
			return err
		}
	}
	collect, err := e.beginCollection(ctx, record, run.ID, searchRunID)
	if err != nil {
		return err
	}
	if collect {
		if record.Source != "rss" {
			body, _ := json.Marshal(map[string]string{"runId": searchRunID})
			if err = searcher.Process(ctx, jobs.Job{Payload: body}); err != nil {
				return err
			}
			if record.Source == "search" {
				var successful int
				if err = e.DB.QueryRow(ctx, `SELECT count(*) FROM search_site_results WHERE run_id=$1 AND state IN('succeeded','partial')`, searchRunID).Scan(&successful); err != nil {
					return err
				}
				if successful == 0 {
					return errors.New("所有站点搜索失败；未执行订阅决策")
				}
			}
		}
		if record.Source != "search" {
			if err = e.feeds(ctx, record, run, searchRunID, searcher); err != nil {
				return err
			}
		}
		if err = e.completeCollection(ctx, record, run.ID, searchRunID); err != nil {
			if errors.Is(err, ErrEvaluationRefresh) || snapshotRequiresRefresh(err) {
				return e.needsRefresh(ctx, run.ID)
			}
			return err
		}
	}
	if err = e.validateEvaluation(ctx, record, run); err != nil {
		return err
	}
	if err = searcher.ValidateRunSources(ctx, searchRunID); err != nil {
		if snapshotRequiresRefresh(err) {
			return e.needsRefresh(ctx, run.ID)
		}
		return err
	}
	target := map[int]bool{}
	if record.Media.MediaType == "series" {
		episodes := record.Rules.Episodes
		if len(episodes) == 0 {
			client, err := e.Connections.TMDB(ctx)
			if err != nil {
				return err
			}
			episodes, err = client.SeasonEpisodes(ctx, record.Media.ID, record.Rules.Season)
			if err != nil {
				return err
			}
		}
		for _, ep := range episodes {
			target[ep] = true
		}
		if len(target) == 0 {
			return errors.New("no explicit or aired target episodes yet")
		}
	}
	covered, movieCovered, err := e.coverage(ctx, record)
	if err != nil {
		return err
	}
	var candidates []search.Resource
	for page := 1; ; page++ {
		results, err := searcher.Get(ctx, searchRunID, page, 200)
		if err != nil {
			return err
		}
		candidates = append(candidates, results.Resources...)
		if page*200 >= results.Total {
			break
		}
	}
	rank := map[string]int{}
	for i, id := range record.Rules.AccountIDs {
		rank[id] = i
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if rank[candidates[i].AccountID] != rank[candidates[j].AccountID] {
			return rank[candidates[i].AccountID] < rank[candidates[j].AccountID]
		}
		return candidates[i].Seeders > candidates[j].Seeders
	})
	acquirer := acquisition.Engine{DB: e.DB, Connections: e.Connections, Search: searcher}
	for _, resource := range candidates {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err = e.validateEvaluation(ctx, record, run); err != nil {
			return err
		}
		if err = searcher.ValidateRunResource(ctx, searchRunID, resource.ID); err != nil {
			if snapshotRequiresRefresh(err) {
				return e.needsRefresh(ctx, run.ID)
			}
			return err
		}
		var oldAction string
		if e.DB.QueryRow(ctx, `SELECT action FROM subscription_decisions WHERE run_id=$1 AND resource_id=$2`, run.ID, resource.ID).Scan(&oldAction) == nil {
			continue
		}
		action := "skipped"
		matched, reasons, checks := subscriptionpolicy.Evaluate(resource, record.Media, record.Rules)
		episodes := []int{}
		hash := ""
		taskID := ""
		if matched && record.Media.MediaType == "movie" && movieCovered {
			matched = false
			reasons = append(reasons, "媒体已入库或有未完成下载，不创建重复任务")
		}
		if matched {
			torrent, err := searcher.TorrentFromRun(ctx, searchRunID, resource.ID)
			if err != nil {
				if snapshotRequiresRefresh(err) {
					return e.needsRefresh(ctx, run.ID)
				}
				matched = false
				reasons = append(reasons, "种子预检失败，未提交下载器")
			} else {
				meta, err := downloads.ParseTorrent(torrent)
				if err != nil {
					matched = false
					reasons = append(reasons, "种子格式或文件列表不支持，需人工确认")
				} else {
					hash = meta.InfoHash
					if record.Media.MediaType == "series" {
						names := []string{}
						for _, file := range meta.Files {
							names = append(names, file.Name)
						}
						episodes, err = ManifestCoverage(names, record.Rules.Season)
						if err != nil {
							matched = false
							action = "needs_confirmation"
							reasons = append(reasons, err.Error())
						} else {
							missing := 0
							for _, ep := range episodes {
								if target[ep] && !covered[ep] {
									missing++
								}
							}
							if missing == 0 {
								matched = false
								reasons = append(reasons, "未覆盖当前缺集，或季集已在库/下载中")
							}
						}
					}
				}
			}
		}
		if matched {
			if run.Preview {
				action = "would_download"
				reasons = append(reasons, "规则和种子预检通过；仅预演，不创建下载任务")
			} else {
				var season *int
				if record.Media.MediaType == "series" {
					season = &record.Rules.Season
				}
				taskID, err = acquirer.StartExpectedFromRun(ctx, searchRunID, resource.ID, record.Media.MediaType, record.Media.ID, hash, season, episodes, run.ID, record.Revision)
				if err != nil {
					if snapshotRequiresRefresh(err) {
						return e.needsRefresh(ctx, run.ID)
					}
					action = "download_failed"
					reasons = append(reasons, "下载任务创建失败，请检查下载器、目录和媒体配置")
				} else {
					action = "download_queued"
					reasons = append(reasons, "匹配缺集/媒体，已提交持久化下载任务")
				}
			}
			if (run.Preview || action == "download_queued") && record.Media.MediaType == "movie" {
				movieCovered = true
			} else if run.Preview || action == "download_queued" {
				for _, ep := range episodes {
					covered[ep] = true
				}
			}
		}
		trace.Info(ctx, "subscriptions", "订阅资源决策", "subscriptionId", record.ID, "subscriptionRunId", run.ID, "resourceId", resource.ID, "title", resource.Title, "action", action, "reasons", reasons, "checks", checks, "preview", run.Preview)
		if err = e.decision(ctx, run.ID, resource, action, reasons, episodes, taskID, checks); err != nil {
			return err
		}
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var currentRevision int64
	var currentEnabled bool
	if err = tx.QueryRow(ctx, `SELECT revision,enabled FROM subscriptions WHERE id=$1 FOR SHARE`, record.ID).Scan(&currentRevision, &currentEnabled); err != nil {
		return err
	}
	if currentRevision != record.Revision || (!run.Preview && !currentEnabled) {
		return e.needsRefresh(ctx, run.ID)
	}
	// Coverage, metadata and decisions may take time. Revalidate source proofs
	// in this committing transaction, including successful empty RSS feeds,
	// and retain source locks until baseline/seen facts have committed.
	if err = searcher.ValidateRunSourcesTx(ctx, tx, searchRunID); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return rollbackErr
		}
		if snapshotRequiresRefresh(err) {
			return e.needsRefresh(ctx, run.ID)
		}
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE subscription_runs SET state='succeeded',finished_at=now()WHERE id=$1`, run.ID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE search_runs SET state='succeeded',finished_at=now()WHERE id=$1 AND state='pending'`, searchRunID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE subscriptions SET last_state='succeeded',last_message=$2 WHERE id=$1`, record.ID, "评估完成："+strconv.Itoa(len(candidates))+" 个候选，详见决策记录"); err != nil {
		return err
	}
	if !run.Preview {
		if _, err = tx.Exec(ctx, `INSERT INTO subscription_rss_seen(subscription_id,account_id,guid_digest)SELECT $1::uuid,account_id,guid_digest FROM subscription_rss_run_items WHERE run_id=$2 ON CONFLICT DO NOTHING`, record.ID, run.ID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO subscription_rss_accounts(subscription_id,account_id)SELECT DISTINCT $1::uuid,account_id FROM subscription_rss_run_accounts WHERE run_id=$2 ON CONFLICT DO NOTHING`, record.ID, run.ID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (e Engine) decision(ctx context.Context, runID string, resource search.Resource, action string, reasons []string, episodes []int, taskID string, checks []subscriptionpolicy.RuleCheck) error {
	raw, _ := json.Marshal(reasons)
	coverage, _ := json.Marshal(episodes)
	var task any
	if taskID != "" {
		task = taskID
	}
	checkRaw, _ := json.Marshal(checks)
	_, err := e.DB.Exec(ctx, `INSERT INTO subscription_decisions(id,run_id,resource_id,title,action,reasons,covered_episodes,download_task_id,rule_checks)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)ON CONFLICT(run_id,resource_id)DO NOTHING`, jobs.UUID(), runID, resource.ID, resource.Title, action, raw, coverage, task, checkRaw)
	return err
}
func (e Engine) coverage(ctx context.Context, record Record) (map[int]bool, bool, error) {
	out := map[int]bool{}
	movie := false
	rows, err := e.DB.Query(ctx, `SELECT b.season,b.episode,b.episodes FROM media_asset_bindings b JOIN media_items m ON m.id=b.media_id WHERE m.media_type=$1 AND m.tmdb_id=$2`, record.Media.MediaType, record.Media.ID)
	if err != nil {
		return out, false, err
	}
	for rows.Next() {
		var season, episode *int
		var raw []byte
		if err = rows.Scan(&season, &episode, &raw); err != nil {
			rows.Close()
			return out, false, err
		}
		movie = true
		if season != nil && *season == record.Rules.Season {
			if episode != nil {
				out[*episode] = true
			}
			var episodes []int
			if checkedErr := json.Unmarshal(raw, &episodes); checkedErr != nil {
				return *new(map[int]bool), false, checkedErr
			}
			for _, ep := range episodes {
				out[ep] = true
			}
		}
	}
	rows.Close()
	rows, err = e.DB.Query(ctx, `SELECT details FROM download_tasks WHERE media_type=$1 AND tmdb_id=$2 AND download_state NOT IN ('failed','cancelled')`, record.Media.MediaType, record.Media.ID)
	if err != nil {
		return out, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if checkedErr := rows.Scan(&raw); checkedErr != nil {
			return *new(map[int]bool), false, checkedErr
		}
		movie = true
		var details struct {
			Season   *int  `json:"expectedSeason"`
			Episodes []int `json:"expectedEpisodes"`
		}
		if checkedErr := json.Unmarshal(raw, &details); checkedErr != nil {
			return *new(map[int]bool), false, checkedErr
		}
		if details.Season != nil && *details.Season == record.Rules.Season {
			for _, ep := range details.Episodes {
				out[ep] = true
			}
		}
	}
	return out, movie, rows.Err()
}
func (e Engine) feeds(ctx context.Context, record Record, run RunRecord, searchRunID string, searcher search.Engine) error {
	for _, accountID := range record.Rules.AccountIDs {
		c, err := e.Connections.Repository.Get(ctx, accountID)
		if err != nil {
			return err
		}
		client, err := e.Connections.Site(ctx, accountID)
		if err != nil {
			return err
		}
		items, err := client.RSS(ctx, c.Values["rssUrl"])
		if err != nil {
			// Fixed text only: the URL can hold a passkey in its query or path.
			message := "RSS请求失败，请检查站点登录、RSS链接和网络；未更新订阅基线"
			if errors.Is(err, sites.ErrFeed) {
				message = "RSS响应无效或种子标识不支持；未更新订阅基线"
			}
			if _, checkedErr := e.DB.Exec(context.WithoutCancel(ctx), `UPDATE search_site_results SET state='failed',message=$3 WHERE run_id=$1 AND account_id=$2`, searchRunID, accountID, message); checkedErr != nil {
				return checkedErr
			}
			return err
		}
		if !run.Preview {
			if _, err = e.DB.Exec(ctx, `INSERT INTO subscription_rss_run_accounts(run_id,account_id)VALUES($1,$2)ON CONFLICT DO NOTHING`, run.ID, accountID); err != nil {
				return err
			}
		}
		var initialized bool
		if err = e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM subscription_rss_accounts WHERE subscription_id=$1 AND account_id=$2)`, record.ID, accountID).Scan(&initialized); err != nil {
			return err
		}
		eligible := []sites.Resource{}
		for _, item := range items {
			var seen bool
			if err = e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM subscription_rss_seen WHERE subscription_id=$1 AND account_id=$2 AND guid_digest=$3)`, record.ID, accountID, item.Digest).Scan(&seen); err != nil {
				return err
			}
			if run.Preview || (!seen && (initialized || record.Rules.IncludeHistory)) {
				eligible = append(eligible, item.Resource)
			}
			if !run.Preview {
				if _, err = e.DB.Exec(ctx, `INSERT INTO subscription_rss_run_items(run_id,account_id,guid_digest)VALUES($1,$2,$3)ON CONFLICT DO NOTHING`, run.ID, accountID, item.Digest); err != nil {
					return err
				}
			}
		}
		if err = searcher.SaveResourcesWithClient(ctx, searchRunID, accountID, eligible, client); err != nil {
			return err
		}
		if _, err = e.DB.Exec(ctx, `UPDATE search_site_results SET state=CASE WHEN $3 AND state IN('partial','failed') THEN state ELSE 'succeeded' END,message=CASE WHEN $3 AND state IN('partial','failed') THEN message ELSE '' END,result_count=(SELECT count(*) FROM search_run_resources l JOIN resources r ON r.id=l.resource_id WHERE l.run_id=$1 AND r.account_id=$2) WHERE run_id=$1 AND account_id=$2`, searchRunID, accountID, record.Source == "both"); err != nil {
			return err
		}
	}
	return nil
}
