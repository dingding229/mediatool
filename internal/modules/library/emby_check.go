package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"io"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"media-tool/internal/modules/settings"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// A successful refresh response is only a request receipt, never visibility.
func (e Engine) Refresh(ctx context.Context, job jobs.Job) error {
	var body struct {
		AssetID string `json:"assetId"`
	}
	if json.Unmarshal(job.Payload, &body) != nil || body.AssetID == "" {
		return errors.New("invalid library refresh job")
	}
	c, err := e.Connections.Repository.Default(ctx, "emby")
	if err != nil {
		return err
	}
	prefix := c.String("strmPathPrefix")
	if prefix == "" || c.Values["apiKey"] == "" {
		return errors.New("Emby STRM path mapping and dedicated API key required")
	}
	rows, err := e.DB.Query(ctx, `SELECT output_path FROM strm_entries WHERE asset_id=$1 ORDER BY output_path`, body.AssetID)
	if err != nil {
		return err
	}
	updates := []map[string]string{}
	for rows.Next() {
		var output string
		if rows.Scan(&output) != nil {
			rows.Close()
			return errors.New("output evidence unreadable")
		}
		updates = append(updates, map[string]string{"Path": path.Join(prefix, output), "UpdateType": "Created"})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(updates) == 0 {
		return errors.New("no managed output to refresh")
	}
	receipt, moduleErr := (pluginv2.Store{DB: e.DB, Vault: e.Connections.Repository.Vault}).ExternalEffect(ctx, "mediaserver.refresh", contracts.EffectRequest{RequestID: job.ID, ObjectID: body.AssetID, Kind: "asset"})
	if moduleErr != nil || receipt.State == "unknown" {
		return jobs.ErrOperatorRequired
	}
	if receipt.State == "not_started" {
		if err = (pluginv2.Store{DB: e.DB, Vault: e.Connections.Repository.Vault}).BuiltinEffect(ctx, "mediaserver.refresh", contracts.EffectRequest{RequestID: job.ID, ObjectID: body.AssetID, Kind: "asset"}, func(cctx context.Context) error { return requestEmbyUpdates(cctx, c, updates) }); err != nil {
			return jobs.ErrOperatorRequired
		}
	}

	payload, _ := json.Marshal(map[string]string{"assetId": body.AssetID})
	_, err = e.DB.Exec(ctx, `INSERT INTO jobs(id,kind,payload,idempotency_key,max_attempts)VALUES($1,'library.emby.confirm',$2,$3,20) ON CONFLICT(idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING`, jobs.UUID(), payload, "library.emby.confirm:"+job.ID)
	return err
}
func embyHTTP() *http.Client {
	return &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

type EmbyPath struct {
	Path     string `json:"path"`
	ItemID   string `json:"itemId,omitempty"`
	SourceID string `json:"sourceId,omitempty"`
	Visible  bool   `json:"visible"`
	Reason   string `json:"reason,omitempty"`
}

func (e Engine) CheckEmby(ctx context.Context, job jobs.Job) (result error) {
	var body struct {
		AssetID string `json:"assetId"`
	}
	if json.Unmarshal(job.Payload, &body) != nil || body.AssetID == "" {
		return errors.New("invalid library visibility job")
	}
	c, err := e.Connections.Repository.Default(ctx, "emby")
	if err != nil {
		return err
	}
	prefix := c.String("strmPathPrefix")
	if prefix == "" || c.Values["apiKey"] == "" {
		return errors.New("configure Emby STRM path and API key")
	}
	var revision int64
	var tmdb int
	var mediaType string
	if err = e.DB.QueryRow(ctx, `SELECT a.revision,m.tmdb_id,m.media_type FROM media_assets a JOIN media_asset_bindings b ON b.asset_id=a.id JOIN media_items m ON m.id=b.media_id WHERE a.id=$1 AND a.artifacts_state='ready'`, body.AssetID).Scan(&revision, &tmdb, &mediaType); err != nil {
		return errors.New("asset outputs not ready")
	}
	rows, err := e.DB.Query(ctx, `SELECT output_path FROM strm_entries WHERE asset_id=$1 ORDER BY output_path`, body.AssetID)
	if err != nil {
		return err
	}
	expected := []EmbyPath{}
	indices := map[string]int{}
	for rows.Next() {
		var output string
		if rows.Scan(&output) != nil {
			rows.Close()
			return errors.New("output evidence unreadable")
		}
		p := path.Join(prefix, output)
		indices[p] = len(expected)
		expected = append(expected, EmbyPath{Path: p, Reason: "尚未可见"})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(expected) == 0 {
		return errors.New("no STRM output")
	}
	evidence, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	snapshot := fmt.Sprintf("%x", sha256.Sum256(evidence))
	query := url.Values{"Recursive": {"true"}, "Fields": {"Path,MediaSources,ProviderIds"}, "Limit": {"200"}, "IncludeItemTypes": {"Movie,Episode"}}
	if parent := c.String("libraryId"); parent != "" {
		query.Set("ParentId", parent)
	}
	endpoint := strings.TrimRight(c.String("endpoint"), "/") + "/Items"
	prefs, err := (settings.Repository{DB: e.DB}).Get(ctx)
	if err != nil {
		return err
	}
	defer func() {
		state, message := "visible", "全部托管路径可见；播放器解码兼容性需单独验收"
		if result != nil {
			state = "pending"
			message = result.Error()
		}
		raw, _ := json.Marshal(expected)
		bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_, err := e.DB.Exec(bounded, `INSERT INTO library_emby_checks(asset_id,integration_id,asset_revision,state,message,paths,integration_revision,user_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8)ON CONFLICT(asset_id)DO UPDATE SET integration_id=excluded.integration_id,asset_revision=excluded.asset_revision,state=excluded.state,message=excluded.message,paths=excluded.paths,integration_revision=excluded.integration_revision,user_id=excluded.user_id,checked_at=now() WHERE library_emby_checks.asset_revision<=excluded.asset_revision`, body.AssetID, c.Record.ID, revision, state, message, raw, c.Record.Revision, prefs.Value.EmbyUserID)
		if err != nil && result == nil {
			result = err
		}
	}()
	if user := prefs.Value.EmbyUserID; user != "" {
		endpoint = strings.TrimRight(c.String("endpoint"), "/") + "/Users/" + url.PathEscape(user) + "/Items"
	}
	for page := 0; page < 100; page++ {
		query.Set("StartIndex", strconv.Itoa(page*200))
		request, err := http.NewRequestWithContext(ctx, "GET", endpoint+"?"+query.Encode(), nil)
		if err != nil {
			return err
		}
		request.Header.Set("X-Emby-Token", c.Values["apiKey"])
		response, err := embyHTTP().Do(request)
		if err != nil {
			return err
		}
		if response.StatusCode != 200 {
			deferutil.Close(response.Body, "Read/descriptor teardown only; primary operations handle their own I/O result")
			return errors.New("Emby visibility request refused")
		}
		var data struct {
			Items []struct {
				ID          string            `json:"Id"`
				Path        string            `json:"Path"`
				ProviderIDs map[string]string `json:"ProviderIds"`
				Sources     []struct {
					ID   string `json:"Id"`
					Path string `json:"Path"`
				} `json:"MediaSources"`
			} `json:"Items"`
			Total int `json:"TotalRecordCount"`
		}
		err = json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&data)
		deferutil.Close(response.Body, "Read/descriptor teardown only; primary operations handle their own I/O result")
		if err != nil {
			return errors.New("invalid Emby item evidence")
		}
		for _, item := range data.Items {
			if item.ID == "" {
				continue
			}
			apply := func(p, source string) error {
				index, ok := indices[p]
				if !ok {
					return nil
				}
				entry := &expected[index]
				if entry.Visible && entry.ItemID != item.ID {
					return errors.New("duplicate Emby items share an owned path")
				}
				if mediaType == "movie" && item.ProviderIDs["Tmdb"] != "" && item.ProviderIDs["Tmdb"] != strconv.Itoa(tmdb) {
					return errors.New("Emby TMDB identity differs from confirmed media")
				}
				entry.Visible = true
				entry.ItemID = item.ID
				entry.SourceID = source
				entry.Reason = "路径匹配"
				return nil
			}
			if err = apply(item.Path, ""); err != nil {
				return err
			}
			for _, source := range item.Sources {
				if err = apply(source.Path, source.ID); err != nil {
					return err
				}
			}
		}
		if len(data.Items) < 200 || data.Total > 0 && (page+1)*200 >= data.Total {
			break
		}
		if page == 99 {
			return errors.New("Emby listing exceeds bounded verification scope")
		}
	}
	for _, entry := range expected {
		if !entry.Visible {
			return errors.New("Emby has not exposed all managed paths for this scope/user")
		}
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	currentPrefs, err := (settings.Repository{DB: e.DB}).Get(ctx)
	if err != nil {
		return err
	}
	if currentPrefs.Value.EmbyUserID != prefs.Value.EmbyUserID {
		return errors.New("Emby user scope changed during verification")
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,1315))`, body.AssetID); err != nil {
		return err
	}
	var connectionRevision int64
	var enabled bool
	if err = tx.QueryRow(ctx, `SELECT revision,enabled FROM integrations WHERE id=$1 FOR SHARE`, c.Record.ID).Scan(&connectionRevision, &enabled); err != nil {
		return err
	}
	if !enabled || connectionRevision != c.Record.Revision {
		return errors.New("Emby scope changed during visibility verification")
	}
	var current int64
	if err = tx.QueryRow(ctx, `SELECT revision FROM media_assets WHERE id=$1 FOR SHARE`, body.AssetID).Scan(&current); err != nil {
		return err
	}
	if current != revision {
		return errors.New("asset changed during Emby verification")
	}
	currentPaths := []EmbyPath{}
	rows, err = tx.Query(ctx, `SELECT output_path FROM strm_entries WHERE asset_id=$1 ORDER BY output_path`, body.AssetID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var output string
		if rows.Scan(&output) != nil {
			rows.Close()
			return errors.New("output evidence unreadable")
		}
		currentPaths = append(currentPaths, EmbyPath{Path: path.Join(prefix, output), Reason: "尚未可见"})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(currentPaths)
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != snapshot {
		return errors.New("output layout changed during Emby verification")
	}
	for _, entry := range expected {
		var conflict bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM emby_item_mappings WHERE emby_instance=$1 AND item_id=$2 AND media_source_id=$3 AND asset_id<>$4)`, c.Record.ID, entry.ItemID, entry.SourceID, body.AssetID).Scan(&conflict); err != nil {
			return err
		}
		if conflict {
			return errors.New("Emby item already belongs to another file grant")
		}
		if _, err = tx.Exec(ctx, `INSERT INTO emby_item_mappings(emby_instance,item_id,media_source_id,asset_id)VALUES($1,$2,$3,$4)ON CONFLICT DO NOTHING`, c.Record.ID, entry.ItemID, entry.SourceID, body.AssetID); err != nil {
			return err
		}
	}
	event, _ := json.Marshal(map[string]any{"assetId": body.AssetID, "revision": revision, "title": "托管输出逐路径可见"})
	if _, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)SELECT $1,'library.emby.visible',$2,$3 WHERE NOT EXISTS(SELECT 1 FROM outbox_events WHERE aggregate_id=$2 AND event_type='library.emby.visible' AND payload->>'revision'=$4)`, jobs.UUID(), body.AssetID, event, strconv.FormatInt(revision, 10)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RequestTargetedRefresh is shared by cloud confirmations and download import.
// It sends path notifications only, never a global /Library/Refresh fallback.
func RequestTargetedRefresh(ctx context.Context, c integrations.Credentials, outputs []string) error {
	if c.String("strmPathPrefix") == "" || len(outputs) == 0 || c.Values["apiKey"] == "" {
		return errors.New("owned output paths and Emby mapping required")
	}
	updates := []map[string]string{}
	for _, output := range outputs {
		if strings.HasPrefix(output, "/") || path.Clean(output) != output || strings.HasPrefix(output, "../") {
			return errors.New("unsafe owned output path")
		}
		updates = append(updates, map[string]string{"Path": path.Join(c.String("strmPathPrefix"), output), "UpdateType": "Created"})
	}
	return requestEmbyUpdates(ctx, c, updates)
}
func requestEmbyUpdates(ctx context.Context, c integrations.Credentials, updates []map[string]string) error {
	raw, _ := json.Marshal(map[string]any{"Updates": updates})
	request, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(c.String("endpoint"), "/")+"/Library/Media/Updated", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	request.Header.Set("X-Emby-Token", c.Values["apiKey"])
	request.Header.Set("Content-Type", "application/json")
	response, err := embyHTTP().Do(request)
	if err != nil {
		return err
	}
	defer deferutil.Close(response.Body, "Read/descriptor teardown only; primary operations handle their own I/O result")
	if _, checkedErr := io.Copy(io.Discard, io.LimitReader(response.Body, 4096)); checkedErr != nil {
		return checkedErr
	}
	if response.StatusCode != 200 && response.StatusCode != 204 {
		return fmt.Errorf("Emby targeted refresh refused (%d)", response.StatusCode)
	}
	return nil
}
