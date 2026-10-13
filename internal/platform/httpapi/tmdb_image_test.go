package httpapi

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"media-tool/internal/modules/integrations"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTMDBImageFixedOriginSignatureAndNoCredentialForwarding(t *testing.T) {
	var pixel bytes.Buffer
	png.Encode(&pixel, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	calls := 0
	manager := &integrations.Manager{OutboundHTTP: &http.Client{Transport: mediaCacheTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Scheme != "https" || r.URL.Host != "image.tmdb.org" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("User-Agent") != "" {
			t.Error("credentials/URL forwarded")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(pixel.Bytes()))}, nil
	})}}
	a := API{Deps: Dependencies{Integrations: manager}}
	query := func(size, file string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/image", nil)
		r.Header.Set("Authorization", "must-not-forward")
		r.Header.Set("Cookie", "private")
		r.Header.Set("User-Agent", "private-agent")
		r.SetPathValue("size", size)
		r.SetPathValue("file", file)
		w := httptest.NewRecorder()
		a.tmdbImage(w, r)
		return w
	}
	w := query("w500", "owned.png")
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, test := range [][2]string{{"original", "owned.png"}, {"w500", "../private.png"}, {"w500", "https://bad.png"}, {"w500", "owned.svg"}} {
		if query(test[0], test[1]).Code != 404 {
			t.Fatal(test)
		}
	}
	if calls != 1 {
		t.Fatal("invalid paths contacted provider")
	}
	manager.OutboundHTTP = &http.Client{Transport: mediaCacheTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewBufferString("<html>challenge</html>"))}, nil
	})}
	if query("w500", "owned.png").Code != 502 {
		t.Fatal("HTML returned as image")
	}
}
