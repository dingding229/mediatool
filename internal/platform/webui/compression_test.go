package webui

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestPrecompressedStaticNegotiationAndCachePolicy(t *testing.T) {
	body := []byte(strings.Repeat("console.log('static fixture');", 100))
	var compressed bytes.Buffer
	g := gzip.NewWriter(&compressed)
	g.Write(body)
	g.Close()
	h := handler(fstest.MapFS{"index.html": {Data: []byte("static shell")}, "assets/app-hash.js": {Data: body}, "assets/app-hash.js.gz": {Data: compressed.Bytes()}, "sw.js": {Data: []byte("sw")}})
	for _, tc := range []struct {
		encoding, method, rangeHeader string
		gzip                          bool
	}{{"gzip", "GET", "", true}, {"br, gzip;q=0.5", "HEAD", "", true}, {"gzip;q=0, *;q=1", "GET", "", false}, {"gzip;q=invalid", "GET", "", false}, {"gzip", "GET", "bytes=0-3", false}, {"", "GET", "", false}} {
		r := httptest.NewRequest(tc.method, "/assets/app-hash.js", nil)
		r.Header.Set("Accept-Encoding", tc.encoding)
		r.Header.Set("Range", tc.rangeHeader)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if (w.Header().Get("Content-Encoding") == "gzip") != tc.gzip || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") || !strings.Contains(w.Header().Get("Vary"), "Accept-Encoding") {
			t.Fatal(tc, w.Header())
		}
		if tc.method == "HEAD" {
			if w.Body.Len() != 0 {
				t.Fatal("HEAD had body")
			}
			continue
		}
		if tc.gzip {
			reader, err := gzip.NewReader(w.Body)
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(reader)
			reader.Close()
			if err != nil || !bytes.Equal(got, body) {
				t.Fatal("compressed representation changed")
			}
		} else if tc.rangeHeader != "" {
			if w.Code != 206 || w.Body.String() != string(body[:4]) {
				t.Fatal("range changed", w.Code)
			}
		} else if !bytes.Equal(w.Body.Bytes(), body) {
			t.Fatal("identity changed")
		}
	}
	for _, path := range []string{"/dashboard", "/index.html", "/sw.js"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Header().Get("Cache-Control") != "no-cache" {
			t.Fatal(path, w.Header())
		}
	}
}
