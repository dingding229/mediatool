package httpapi

import (
	"media-tool/internal/platform/scheduling"
	"net/http"
	"time"
)

func (a *API) previewCron(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Expression string `json:"expression"`
		Purpose    string `json:"purpose"`
	}
	if !decodeLimit(w, r, &in, 2048) {
		return
	}
	out, err := scheduling.Preview(in.Expression, in.Purpose, time.Now())
	if err != nil {
		fail(w, 400, "invalid_cron", err.Error())
		return
	}
	write(w, 200, out)
}
