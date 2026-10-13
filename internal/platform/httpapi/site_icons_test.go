package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestSiteIdentityIconIsPublicStaticICOWithValidator(t *testing.T) {
	a := API{}
	for _, id := range []string{"hdsky", "ourbits", "ptchdbits", "ssd", "ttg", "ubits"} {
		r := httptest.NewRequest("GET", "/icon", nil)
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		a.ptTemplateIcon(w, r)
		b := w.Body.Bytes()
		if w.Code != 200 || len(b) < 22 || string(b[:4]) != "\x00\x00\x01\x00" || w.Header().Get("Content-Type") != "image/x-icon" {
			t.Fatal(id, w.Code)
		}
		r.Header.Set("If-None-Match", w.Header().Get("ETag"))
		w = httptest.NewRecorder()
		a.ptTemplateIcon(w, r)
		if w.Code != 304 || w.Body.Len() != 0 {
			t.Fatal("cache validator")
		}
	}
	r := httptest.NewRequest("GET", "/icon", nil)
	r.SetPathValue("id", "../../private")
	w := httptest.NewRecorder()
	a.ptTemplateIcon(w, r)
	if w.Code != 404 {
		t.Fatal("unknown icon")
	}
}
