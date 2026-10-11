package library

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"media-tool/internal/modules/jobs"
	"net/http"
	"strings"
	"time"
)

func (e Engine) QueueRefresh(ctx context.Context, assetID string) error {
	payload, _ := json.Marshal(map[string]string{"assetId": assetID})
	_, err := e.DB.Exec(ctx, `INSERT INTO jobs(id,kind,payload,idempotency_key)VALUES($1,'library.refresh',$2,$3)ON CONFLICT(idempotency_key)WHERE idempotency_key IS NOT NULL DO NOTHING`, jobs.UUID(), payload, "library.refresh:"+assetID+":"+jobs.UUID())
	return err
}
func (e Engine) Refresh(ctx context.Context, job jobs.Job) error {
	var body struct {
		AssetID string `json:"assetId"`
	}
	if json.Unmarshal(job.Payload, &body) != nil {
		return errors.New("invalid library refresh job")
	}
	connection, err := e.Connections.Repository.Default(ctx, "emby")
	if err != nil {
		return errors.New("Emby connection required for library refresh")
	}
	if connection.Values["apiKey"] == "" {
		return errors.New("dedicated Emby library key required")
	}
	request, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(connection.String("endpoint"), "/")+"/Library/Refresh", nil)
	if err != nil {
		return errors.New("invalid Emby address")
	}
	request.Header.Set("X-Emby-Token", connection.Values["apiKey"])
	response, err := (&http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(request)
	if err != nil {
		return errors.New("Emby library refresh unavailable")
	}
	defer response.Body.Close()
	io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
	if response.StatusCode != 200 && response.StatusCode != 204 {
		return errors.New("Emby refused library refresh")
	}
	payload, _ := json.Marshal(map[string]string{"assetId": body.AssetID})
	_, err = e.DB.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)VALUES($1,'library.refresh_requested',$2,$3)`, jobs.UUID(), body.AssetID, payload)
	return err
}
