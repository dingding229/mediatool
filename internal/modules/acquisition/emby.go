package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"media-tool/internal/modules/jobs"
)

func (e Engine) RefreshEmby(ctx context.Context, job jobs.Job) error {
	var payload struct {
		TaskID string `json:"taskId"`
	}
	if json.Unmarshal(job.Payload, &payload) != nil {
		return errors.New("invalid Emby refresh job")
	}
	c, err := e.Connections.Repository.Default(ctx, "emby")
	if err != nil {
		return errors.New("configure and enable Emby before completing library import")
	}
	if c.Values["apiKey"] == "" {
		return errors.New("Emby refresh requires its dedicated library API Key")
	}
	request, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(c.String("endpoint"), "/")+"/Library/Refresh", nil)
	if err != nil {
		return errors.New("invalid Emby endpoint")
	}
	request.Header.Set("X-Emby-Token", c.Values["apiKey"])
	response, err := (&http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(request)
	if err != nil {
		return errors.New("Emby refresh request failed")
	}
	defer response.Body.Close()
	io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != 200 && response.StatusCode != 204 {
		return errors.New("Emby refused media library refresh")
	}
	_, _, _, _, details, err := e.load(ctx, payload.TaskID)
	if err != nil {
		return err
	}
	if err = e.transition(ctx, payload.TaskID, "refresh_requested", "library.refresh_requested", details); err != nil {
		return err
	}
	raw, _ := json.Marshal(map[string]string{"taskId": payload.TaskID})
	_, err = e.DB.Exec(ctx, `INSERT INTO jobs(id,kind,payload,idempotency_key,max_attempts)VALUES($1,'emby.confirm',$2,$3,20)ON CONFLICT(idempotency_key)WHERE idempotency_key IS NOT NULL DO NOTHING`, jobs.UUID(), raw, "emby.confirm:"+payload.TaskID)
	return err
}
func (e Engine) ConfirmEmby(ctx context.Context, job jobs.Job) error {
	var payload struct {
		TaskID string `json:"taskId"`
	}
	if json.Unmarshal(job.Payload, &payload) != nil {
		return errors.New("invalid Emby confirmation job")
	}
	_, _, _, state, details, err := e.load(ctx, payload.TaskID)
	if err != nil {
		return err
	}
	if state == "imported" {
		return nil
	}
	c, err := e.Connections.Repository.Default(ctx, "emby")
	if err != nil {
		return err
	}
	prefix := c.String("strmPathPrefix")
	if prefix == "" {
		return errors.New("configure the STRM directory as seen by Emby")
	}
	rows, err := e.DB.Query(ctx, `SELECT s.asset_id::text,s.output_path FROM strm_entries s JOIN media_asset_bindings b ON b.asset_id=s.asset_id JOIN download_files f ON f.id=b.download_file_id WHERE f.task_id=$1`, payload.TaskID)
	if err != nil {
		return err
	}
	expected := map[string]string{}
	for rows.Next() {
		var asset, output string
		if err = rows.Scan(&asset, &output); err != nil {
			rows.Close()
			return err
		}
		expected[path.Join(prefix, output)] = asset
	}
	rows.Close()
	if len(expected) == 0 {
		return errors.New("no managed STRM entry to confirm")
	}
	query := url.Values{"Recursive": {"true"}, "Fields": {"Path,MediaSources"}, "Limit": {"200"}, "IncludeItemTypes": {"Movie,Episode"}}
	// Episode titles need not contain the series title; exact owned output paths
	// are the acceptance identity, not a fuzzy SearchTerm.
	// Current confirmed per-file identities may differ from the original task;
	// exact output paths, not stale original titles, remain the acceptance key.
	type match struct{ asset, item, source string }
	matches := []match{}
	matched := map[string]bool{}
	for page := 0; page < 100 && len(matched) < len(expected); page++ {
		query.Set("StartIndex", strconv.Itoa(page*200))
		request, _ := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(c.String("endpoint"), "/")+"/Items?"+query.Encode(), nil)
		request.Header.Set("X-Emby-Token", c.Values["apiKey"])
		response, err := (&http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(request)
		if err != nil {
			return errors.New("Emby item confirmation unavailable")
		}
		if response.StatusCode != 200 {
			response.Body.Close()
			return errors.New("Emby item confirmation refused")
		}
		var data struct {
			Items []struct {
				ID      string `json:"Id"`
				Path    string `json:"Path"`
				Sources []struct {
					ID   string `json:"Id"`
					Path string `json:"Path"`
				} `json:"MediaSources"`
			}
			Total int `json:"TotalRecordCount"`
		}
		decodeErr := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&data)
		response.Body.Close()
		if decodeErr != nil {
			return errors.New("Emby item response invalid")
		}
		for _, item := range data.Items {
			asset := expected[item.Path]
			if asset != "" {
				source := ""
				if len(item.Sources) == 1 {
					source = item.Sources[0].ID
				}
				matches = append(matches, match{asset, item.ID, source})
				matched[item.Path] = true
			}
			for _, source := range item.Sources {
				if asset := expected[source.Path]; asset != "" {
					matches = append(matches, match{asset, item.ID, source.ID})
					matched[source.Path] = true
				}
			}
		}
		if len(data.Items) == 0 || len(data.Items) < 200 || (data.Total > 0 && (page+1)*200 >= data.Total) {
			break
		}
	}
	if len(matched) != len(expected) {
		return errors.New("Emby scan has not exposed all managed STRM files yet")
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, m := range matches {
		if m.item == "" {
			return errors.New("Emby item has no ID")
		}
		if _, err = tx.Exec(ctx, `INSERT INTO emby_item_mappings(emby_instance,item_id,media_source_id,asset_id)VALUES($1,$2,$3,$4)ON CONFLICT(emby_instance,item_id,media_source_id)DO UPDATE SET asset_id=excluded.asset_id`, c.Record.ID, m.item, m.source, m.asset); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return e.transition(ctx, payload.TaskID, "imported", "library.visible", details)
}
