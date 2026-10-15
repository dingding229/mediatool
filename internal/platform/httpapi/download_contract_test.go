package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadYearConfirmationJSONContract(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/v1/downloads", strings.NewReader(`{"confirmYearMismatch":true,"correctMediaIdentity":false,"searchRunId":"run","resourceId":"resource","mediaType":"series","tmdbId":10}`))
	w := httptest.NewRecorder()
	var in startDownloadInput
	if !decode(w, r, &in) || !in.ConfirmYearMismatch || in.CorrectMediaIdentity || in.SearchRunID != "run" || in.ResourceID != "resource" || in.MediaType != "series" || in.TMDBID != 10 {
		t.Fatal("request fields lost", in, w.Body.String())
	}
}
func TestManualDownloadRetryJSONContract(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/v1/downloads/manual/id/retry", strings.NewReader(`{"confirm":true,"acceptDuplicateRisk":true}`))
	w := httptest.NewRecorder()
	var in manualDownloadRetryInput
	if !decode(w, r, &in) || !in.Confirm || !in.AcceptDuplicateRisk {
		t.Fatal("request consent fields lost", in, w.Body.String())
	}
}
func TestManualDownloadActionsRegisteredWithoutRoutingConflict(t *testing.T) {
	h, _ := testHandler(nil)
	for _, action := range []string{"retry", "inspect"} {
		w := invoke(h, "POST", "/api/v1/downloads/manual/00000000-0000-4000-8000-000000000001/"+action, `{}`, "")
		if w.Code != 401 {
			t.Fatal("manual actions must require administrator login", action, w.Code)
		}
	}
}
