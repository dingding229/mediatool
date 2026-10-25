package httpapi

import (
	"crypto/sha256"
	"errors"
	"io"
	"media-tool/internal/modules/imageedit"
	"media-tool/internal/modules/library"
	"media-tool/internal/modules/playbackgrants"
	"media-tool/internal/modules/strm"
	"media-tool/internal/platform/auth"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func (a *API) grants() playbackgrants.Service {
	return playbackgrants.Service{DB: a.Deps.DB, Vault: a.Deps.Integrations.Repository.Vault, PublicURL: a.Config.PublicURL}
}
func (a *API) listAssets(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	p, err := pagination(r)
	if err != nil {
		fail(w, 400, "invalid_pagination", "分页参数无效")
		return
	}
	assetFilter := r.URL.Query().Get("assetId")
	if values := r.URL.Query()["assetId"]; len(values) > 1 || len(values) == 1 && (!library.ValidArchiveID(assetFilter)) {
		fail(w, 400, "asset_filter_invalid", "资产筛选ID无效或重复")
		return
	}
	limit := p.Limit
	if !p.Paged {
		limit = 500
	}
	rows, err := a.Deps.DB.Query(r.Context(), `SELECT a.id::text,a.revision,a.artifacts_state,m.title,m.media_type,f.full_path,f.state,coalesce(g.revoked_at IS NOT NULL,false),g.id IS NOT NULL,coalesce(m.metadata->>'posterPath','') FROM media_assets a JOIN cloud_files f ON f.id=a.cloud_file_id LEFT JOIN media_asset_bindings b ON b.asset_id=a.id LEFT JOIN media_items m ON m.id=b.media_id LEFT JOIN playback_grants g ON g.asset_id=a.id WHERE ($3='' OR a.id::text=$3) ORDER BY a.created_at DESC,a.id DESC LIMIT $1 OFFSET $2`, limit, p.Offset, strings.ToLower(assetFilter))
	if err != nil {
		fail(w, 503, "assets_unavailable", "媒体文件暂不可用")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var revision int64
		var artifactsState, poster string
		var id, cloudPath, state string
		var title, kind *string
		var revoked, scoped bool
		if err = rows.Scan(&id, &revision, &artifactsState, &title, &kind, &cloudPath, &state, &revoked, &scoped, &poster); err != nil {
			fail(w, 503, "assets_unavailable", "媒体文件读取失败")
			return
		}
		out = append(out, map[string]any{"id": id, "revision": revision, "artifactsState": artifactsState, "title": title, "mediaType": kind, "path": cloudPath, "state": state, "revoked": revoked, "fileScoped": scoped, "poster": func() string {
			if validTMDBImagePath(poster) {
				return "/api/v1/media/tmdb/image?path=" + url.QueryEscape(poster)
			}
			return ""
		}()})
	}
	if rows.Err() != nil {
		fail(w, 503, "assets_unavailable", "分页读取失败")
		return
	}
	rows.Close()
	ids := []string{}
	for _, item := range out {
		ids = append(ids, item["id"].(string))
	}
	posters, imageErr := imageedit.ManagedPrimary(r.Context(), a.Deps.DB, ids)
	if imageErr != nil {
		fail(w, 503, "assets_unavailable", "海报读取失败")
		return
	}
	for _, item := range out {
		if poster := posters[item["id"].(string)]; poster != "" {
			item["poster"] = poster
		}
	}
	if p.Paged {
		var total int
		if a.Deps.DB.QueryRow(r.Context(), `SELECT count(*) FROM media_assets a JOIN cloud_files f ON f.id=a.cloud_file_id WHERE ($1='' OR a.id::text=$1)`, strings.ToLower(assetFilter)).Scan(&total) != nil {
			fail(w, 503, "assets_unavailable", "读取总数失败")
			return
		}
		write(w, 200, map[string]any{"items": out, "total": total, "page": p.Page, "pageSize": p.Limit})
		return
	}
	write(w, 200, out)
}
func (a *API) revokeGrant(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	var body struct{}
	if !decode(w, r, &body) {
		return
	}
	if err := a.grants().Revoke(r.Context(), r.PathValue("id")); err != nil {
		fail(w, 400, "grant_revoke_failed", "文件播放凭据撤销失败")
		return
	}
	write(w, 200, map[string]any{"revoked": true, "note": "旧的path全局Key链接仍由全局兼容开关控制"})
}
func (a *API) rotateGrant(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	var body struct{}
	if !decode(w, r, &body) {
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")
	tx, err := a.Deps.DB.Begin(ctx)
	if err != nil {
		fail(w, 503, "grant_unavailable", "数据库不可用")
		return
	}
	defer deferutil.Rollback(ctx, tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,715))`, id); err != nil {
		fail(w, 503, "grant_unavailable", "数据库不可用")
		return
	}
	var grantID string
	var encrypted []byte
	var currentPath string
	err = tx.QueryRow(ctx, `SELECT g.id::text,g.token_ciphertext,f.full_path FROM media_assets a JOIN cloud_files f ON f.id=a.cloud_file_id LEFT JOIN playback_grants g ON g.asset_id=a.id WHERE a.id=$1 AND f.state='present' FOR UPDATE OF a`, id).Scan(&grantID, &encrypted, &currentPath)
	if err != nil {
		fail(w, 409, "grant_unavailable", "文件级凭据或媒体不可用，请先重新确认入库")
		return
	}
	values, err := a.Deps.Integrations.Repository.Vault.Open(grantID, encrypted)
	if err != nil {
		fail(w, 503, "grant_unavailable", "凭据解密失败")
		return
	}
	rows, err := tx.Query(ctx, `SELECT output_path FROM strm_entries WHERE asset_id=$1 ORDER BY output_path`, id)
	if err != nil {
		fail(w, 503, "grant_unavailable", "输出清单读取失败")
		return
	}
	outputs := []string{}
	for rows.Next() {
		var output string
		if err = rows.Scan(&output); err != nil {
			rows.Close()
			fail(w, 503, "grant_unavailable", "输出读取失败")
			return
		}
		outputs = append(outputs, output)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(outputs) == 0 {
		fail(w, 409, "grant_unavailable", "没有STRM输出")
		return
	}
	root, err := os.OpenRoot(a.Config.STRMOutputDir)
	if err != nil {
		fail(w, 409, "grant_artifact_failed", "输出目录不可用")
		return
	}
	defer deferutil.Close(root, "Read/descriptor teardown only; primary operations handle their own I/O result")
	// Safely adopt only older database-owned STRM whose token and asset match.
	for _, output := range outputs {
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,713))`, output); err != nil {
			fail(w, 503, "grant_unavailable", "输出锁不可用")
			return
		}
		var exists bool
		if checkedErr := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM managed_media_artifacts WHERE relative_path=$1)`, output).Scan(&exists); checkedErr != nil {
			fail(w, 503, "operation_failed", "读取或写入失败，请稍后重试")
			return
		}
		if exists {
			continue
		}
		f, openErr := root.Open(output)
		if errors.Is(openErr, os.ErrNotExist) {
			continue
		}
		if openErr != nil {
			fail(w, 409, "grant_artifact_failed", "STRM不可读")
			return
		}
		raw, readErr := io.ReadAll(io.LimitReader(f, 8193))
		deferutil.Close(f, "Read/descriptor teardown only; primary operations handle their own I/O result")
		u, parseErr := url.Parse(strings.TrimSpace(string(raw)))
		if readErr != nil || len(raw) > 8192 || parseErr != nil || u.User != nil || u.Fragment != "" || u.RawPath != "" || u.Path != strm.Endpoint || u.Scheme+"://"+u.Host != a.Config.PublicURL || u.Query().Get("assetId") != id || !auth.Match(values["token"], u.Query().Get("apiKey")) {
			fail(w, 409, "grant_artifact_failed", "旧STRM未通过归属验证，不覆盖手工内容")
			return
		}
		digest := sha256.Sum256(raw)
		if _, err = tx.Exec(ctx, `INSERT INTO managed_media_artifacts(relative_path,asset_id,artifact_kind,content_digest,owner_scope)VALUES($1,$2,'strm',$3,$4)`, output, id, digest[:], "asset:"+id); err != nil {
			fail(w, 503, "grant_unavailable", "归属清单保存失败")
			return
		}
	}
	service := a.grants()
	link, err := service.RotateTx(ctx, tx, id)
	if err != nil {
		fail(w, 409, "grant_rotate_failed", "轮换失败")
		return
	}
	journal := library.Journal{DB: a.Deps.DB, Vault: a.Deps.Integrations.Repository.Vault}
	intents := []string{}
	for _, output := range outputs {
		intent, err := journal.Stage(ctx, tx, a.Config.STRMOutputDir, output, id, "asset:"+id, "strm", []byte(link+"\n"))
		if err != nil {
			fail(w, 409, "grant_artifact_failed", "输出有未处理的恢复日志或归属冲突")
			return
		}
		intents = append(intents, intent)
	}
	if _, err = tx.Exec(ctx, `UPDATE strm_entries SET key_reference='grant:'||$2,updated_at=now()WHERE asset_id=$1`, id, grantID); err != nil {
		fail(w, 503, "grant_unavailable", "凭据引用更新失败")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 503, "grant_unavailable", "轮换未确认提交")
		return
	}
	for _, intent := range intents {
		if err = journal.Apply(ctx, intent); err != nil {
			fail(w, 409, "grant_artifact_failed", "凭据已轮换，STRM待恢复；修复权限或手工内容后使用恢复入口，不要反复轮换")
			return
		}
	}
	write(w, 200, map[string]bool{"rotated": true})
}
