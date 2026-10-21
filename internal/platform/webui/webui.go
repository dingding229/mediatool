package webui

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
)

//go:embed all:assets
var assets embed.FS

func Handler() http.Handler {
	files, _ := fs.Sub(assets, "assets")
	return handler(files)
}
func handler(files fs.FS) http.Handler {
	server := http.FileServer(http.FS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, segment := range strings.Split(r.URL.Path, "/") {
			if strings.HasPrefix(segment, ".") && segment != "" {
				http.NotFound(w, r)
				return
			}
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if name == "index.html" || path.Ext(name) == "" {
			data, err := fs.ReadFile(files, "index.html")
			if err != nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusServiceUnavailable)
				if _, writeErr := w.Write([]byte(`<html lang="zh-CN"><meta charset="utf-8"><title>Media Tool</title><h1>前端尚未构建</h1><p>请运行 make build-web，或启动 Vue 开发服务器。</p></html>`)); writeErr != nil {
					// The response has already started; there is no safe status to append.
				}
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			if _, err := w.Write(data); err != nil {
				// The response has already started; there is no safe status to append.
			}
			return
		}
		if _, err := fs.Stat(files, name); err != nil {
			http.NotFound(w, r)
			return
		}
		if name == "sw.js" || name == "manifest.webmanifest" || name == "offline.html" || strings.HasPrefix(name, "pwa/") {
			w.Header().Set("Cache-Control", "no-cache")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		if name == "sw.js" {
			w.Header().Set("Service-Worker-Allowed", "/")
		}
		if path.Ext(name) == ".js" || path.Ext(name) == ".css" {
			if _, err := fs.Stat(files, name+".gz"); err == nil {
				w.Header().Add("Vary", "Accept-Encoding")
				if r.Header.Get("Range") == "" && acceptsGzip(strings.Join(r.Header.Values("Accept-Encoding"), ",")) {
					w.Header().Set("Content-Encoding", "gzip")
					w.Header().Set("Content-Type", mime.TypeByExtension(path.Ext(name)))
					compressed := r.Clone(r.Context())
					compressed.URL.Path = "/" + name + ".gz"
					compressed.URL.RawPath = ""
					server.ServeHTTP(w, compressed)
					return
				}
			}
		}
		server.ServeHTTP(w, r)
	})
}

func acceptsGzip(value string) bool {
	wildcard := false
	for _, part := range strings.Split(value, ",") {
		fields := strings.Split(part, ";")
		name := strings.TrimSpace(fields[0])
		q := 1.0
		for _, p := range fields[1:] {
			k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
			if ok && strings.EqualFold(k, "q") {
				n, err := strconv.ParseFloat(v, 64)
				if err != nil || n < 0 || n > 1 {
					q = 0
				} else {
					q = n
				}
			}
		}
		if strings.EqualFold(name, "gzip") {
			return q > 0
		}
		if name == "*" {
			wildcard = q > 0
		}
	}
	return wildcard
}
