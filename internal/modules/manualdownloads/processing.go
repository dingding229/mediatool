package manualdownloads

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"strconv"
	"strings"
	"time"

	"media-tool/internal/modules/acquisition"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"media-tool/internal/modules/releasename"
	"media-tool/internal/modules/settings"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/mediaext"
	"media-tool/internal/platform/textkey"
)

type importSource struct {
	id, integration, hash, title, endpoint, save, category, state string
	encrypted                                                     []byte
	options                                                       map[string]string
}

func (e Engine) importSource(ctx context.Context, id string) (s importSource, err error) {
	s.id = id
	err = e.DB.QueryRow(ctx, `SELECT integration_id::text,info_hash,title,endpoint,save_path,category,state,payload_ciphertext FROM manual_download_tasks WHERE id=$1`, id).Scan(&s.integration, &s.hash, &s.title, &s.endpoint, &s.save, &s.category, &s.state, &s.encrypted)
	if err == nil {
		s.options, err = e.Connections.Repository.Vault.Open("manual-download:"+id, s.encrypted)
	}
	return
}
func importKey(id string) string { return "download.manual.import:" + id }
func (e Engine) queueImport(ctx context.Context, id string) error {
	raw, err := json.Marshal(map[string]string{"id": id})
	if err != nil {
		return err
	}
	tag, err := e.DB.Exec(ctx, `INSERT INTO jobs(id,kind,payload,idempotency_key)VALUES($1,'download.manual.import',$2,$3) ON CONFLICT(idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING`, jobs.UUID(), raw, importKey(id))
	if err == nil && tag.RowsAffected() == 1 {
		_, err = e.DB.Exec(ctx, `UPDATE manual_download_tasks SET message='下载完成，已排队识别；上传校验及入库前不删除原文件',updated_at=now() WHERE id=$1`, id)
	}
	return err
}

// Existing ciphertext lacks autoImport. No timestamp heuristic, history backfill,
// implicit re-add or enabling a new setting can opt an old task in.
func (e Engine) trackImport(ctx context.Context, id string, complete bool) error {
	source, err := e.importSource(ctx, id)
	if err != nil {
		return err
	}
	if source.options["autoImport"] != "true" {
		_, err = e.DB.Exec(ctx, `UPDATE manual_download_tasks SET message='历史任务仅下载，不自动回溯；可明确选择身份后入库',updated_at=now() WHERE id=$1`, id)
		return err
	}
	if complete {
		return e.queueImport(ctx, id)
	}
	return nil
}

// ConfirmImport is an explicit operator opt-in/identity correction, not an
// observational inspect action. It also provides a safe path for old tasks.
func (e Engine) ConfirmImport(ctx context.Context, id, kind string, tmdbID int, confirm bool) error {
	if !confirm || tmdbID < 1 || kind != "movie" && kind != "series" {
		return errors.New("请明确确认媒体身份与原任务后处理")
	}
	source, err := e.importSource(ctx, id)
	if err != nil {
		return err
	}
	if source.state != "completed" {
		return errors.New("下载未完成，不能入库")
	}
	c, err := e.Connections.Repository.Get(ctx, source.integration)
	if err != nil {
		return err
	}
	if !c.Record.Enabled || c.Record.Kind != "qbittorrent" || c.String("endpoint") != source.endpoint || c.String("downloadPath") != source.save {
		return errors.New("原下载器映射已变化，请先人工核对")
	}
	if source.options["autoImport"] != "true" {
		prefs, err := (settings.Repository{DB: e.DB}).Get(ctx)
		if err != nil {
			return err
		}
		source.options["autoImport"] = "true"
		source.options["cleanupAfterImport"] = strconv.FormatBool(prefs.Value.CleanupAfterImport)
		source.options["localPath"] = c.String("localPath")
	}
	source.options["mediaType"], source.options["tmdbId"] = kind, strconv.Itoa(tmdbID)
	encrypted, err := e.Connections.Repository.Vault.Seal("manual-download:"+id, source.options)
	if err != nil {
		return err
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var current, managed, running bool
	if err = tx.QueryRow(ctx, `SELECT state='completed' AND payload_ciphertext=$2,EXISTS(SELECT 1 FROM download_tasks WHERE id=$1) FROM manual_download_tasks WHERE id=$1 FOR UPDATE`, id, source.encrypted).Scan(&current, &managed); err != nil {
		return err
	}
	if !current || managed {
		return errors.New("原任务已变化或已进入托管流水线，请在下载进度中核对，不重复入库")
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE idempotency_key=$1 AND state='running')`, importKey(id)).Scan(&running); err != nil {
		return err
	}
	if running {
		return errors.New("识别仍在执行，请稍后核对")
	}
	if _, err = tx.Exec(ctx, `UPDATE manual_download_tasks SET payload_ciphertext=$2,message='已明确选择身份，等待原任务后处理',updated_at=now() WHERE id=$1`, id, encrypted); err != nil {
		return err
	}
	raw, err := json.Marshal(map[string]string{"id": id})
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,payload,idempotency_key)VALUES($1,'download.manual.import',$2,$3) ON CONFLICT(idempotency_key) WHERE idempotency_key IS NOT NULL DO UPDATE SET state='pending',attempts=0,available_at=now(),last_error=NULL WHERE jobs.state<>'running'`, jobs.UUID(), raw, importKey(id)); err != nil {
		return err
	}
	return dbutil.Commit(ctx, tx)
}
func (e Engine) ProcessImport(ctx context.Context, job jobs.Job) (result error) {
	var in struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(job.Payload, &in) != nil || in.ID == "" {
		return jobs.ErrOperatorRequired
	}
	defer func() {
		if result != nil {
			bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			if _, err := e.DB.Exec(bounded, `UPDATE manual_download_tasks SET message=$2,updated_at=now() WHERE id=$1`, in.ID, "待人工处理，原文件保留："+result.Error()); err != nil {
				result = errors.Join(result, err)
			}
		}
	}()
	source, err := e.importSource(ctx, in.ID)
	if err != nil {
		return err
	}
	if source.state != "completed" || source.options["autoImport"] != "true" {
		return jobs.ErrOperatorRequired
	}
	var managed bool
	if err = e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM download_tasks WHERE id=$1 AND details->>'manualTaskId'=($1::uuid)::text)`, in.ID).Scan(&managed); err != nil {
		return err
	}
	if managed {
		return nil
	}
	client, err := e.client(ctx, source.integration, source.endpoint)
	if err != nil {
		return err
	}
	task, err := client.Inspect(ctx, source.hash)
	if err != nil {
		return err
	}
	if !task.Complete || path.Clean(task.SavePath) != path.Clean(source.save) || task.Category != source.category {
		return errors.Join(jobs.ErrOperatorRequired, errors.New("下载完成或原路径/分类证明不一致"))
	}
	files, err := client.Files(ctx, source.hash)
	if err != nil {
		return err
	}
	hints := []releasename.Hint{}
	store := pluginv2.Store{DB: e.DB, Vault: e.Connections.Repository.Vault}
	parent := store.RecognizeName(ctx, source.title)
	if parent.Title == "" || parent.Year == "" || parent.MediaType == "unknown" {
		observed := store.RecognizeName(ctx, task.Name)
		if observed.Title != "" && observed.Year != "" {
			parent = observed
		}
	}
	for _, file := range files {
		if file.Priority == 0 || !mediaext.IsVideo(file.Name) || isSample(file.Name) {
			continue
		}
		if file.Progress < 1 || file.Size <= 0 {
			return errors.Join(jobs.ErrOperatorRequired, errors.New("媒体文件尚未完整下载"))
		}
		h := store.RecognizeName(ctx, file.Name)
		hints = append(hints, h)
		if h.MediaType == "series" {
			parent.MediaType = "series"
		}
	}
	if len(hints) == 1 && (parent.Title == "" || parent.Year == "" || parent.MediaType == "unknown") && hints[0].Title != "" && hints[0].Year != "" {
		parent = hints[0]
	}
	if len(hints) == 0 {
		return errors.Join(jobs.ErrOperatorRequired, errors.New("没有可识别的完整媒体文件"))
	}
	provider, err := e.Connections.TMDB(ctx)
	if err != nil {
		return err
	}
	var media metadata.Item
	explicit := source.options["tmdbId"] != ""
	if explicit {
		id, parseErr := strconv.Atoi(source.options["tmdbId"])
		if parseErr != nil {
			return parseErr
		}
		media, err = provider.Details(ctx, source.options["mediaType"], id)
		if err == nil && (media.ID != id || media.MediaType != source.options["mediaType"]) {
			err = errors.New("所选媒体身份已变化")
		}
	} else {
		if parent.Title == "" || parent.Year == "" || parent.MediaType != "movie" && parent.MediaType != "series" {
			return errors.Join(jobs.ErrOperatorRequired, errors.New("标题、年份或媒体类型不明确，请选择身份后入库"))
		}
		matches, searchErr := provider.Search(ctx, parent.MediaType, parent.Title)
		if searchErr != nil {
			return searchErr
		}
		selected := metadata.UniqueExact(matches, parent.Title, parent.Year, parent.MediaType)
		if selected == nil {
			return errors.Join(jobs.ErrOperatorRequired, errors.New("标题、年份与媒体类型没有唯一精确匹配，请选择身份后入库"))
		}
		media, err = provider.Details(ctx, selected.MediaType, selected.ID)
		if err == nil && (media.ID != selected.ID || metadata.UniqueExact([]metadata.Item{media}, parent.Title, parent.Year, parent.MediaType) == nil) {
			err = errors.New("媒体详情与识别证据不一致")
		}
	}
	if err != nil {
		return err
	}
	for _, hint := range hints {
		if media.MediaType == "series" && (hint.MediaType != "series" || hint.Episode < 1) {
			return errors.Join(jobs.ErrOperatorRequired, errors.New("季集不明，请核对文件名后再入库"))
		}
		if (!explicit || len(hints) > 1) && hint.Title != "" && textkey.Title(hint.Title) != textkey.Title(parent.Title) && textkey.Title(hint.Title) != textkey.Title(media.Title) && textkey.Title(hint.Title) != textkey.Title(media.OriginalTitle) {
			return errors.Join(jobs.ErrOperatorRequired, errors.New("多个文件的媒体身份不一致，禁止自动归并"))
		}
		if !explicit && hint.Year != "" && hint.Year != media.Year {
			return errors.Join(jobs.ErrOperatorRequired, errors.New("文件年份不一致，需人工核对"))
		}
	}
	return (acquisition.Engine{DB: e.DB, Connections: e.Connections}).AdoptManual(ctx, in.ID, media)
}
func isSample(name string) bool {
	return strings.EqualFold(strings.TrimSuffix(path.Base(name), path.Ext(name)), "sample") || strings.Contains(strings.ToLower(name), "/sample/")
}
