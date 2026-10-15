package webui

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:assets
var assets embed.FS

type realmKey struct{}

func MemberRequest(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), realmKey{}, true))
}
func IsMember(r *http.Request) bool { v, _ := r.Context().Value(realmKey{}).(bool); return v }
func Handler() http.Handler {
	files, _ := fs.Sub(assets, "assets")
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
				w.Write([]byte(`<html lang="zh-CN"><meta charset="utf-8"><title>Media Tool</title><h1>前端尚未构建</h1><p>请运行 make build-web，或启动 Vue 开发服务器。</p></html>`))
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			if IsMember(r) {
				data = []byte(strings.Replace(string(data), "<head>", `<head><meta name="media-tool-realm" content="member">`, 1))
			}
			w.Write(data)
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
		if name == "manifest.webmanifest" && IsMember(r) {
			data, err := fs.ReadFile(files, name)
			var manifest map[string]any
			if err != nil || json.Unmarshal(data, &manifest) != nil {
				http.Error(w, "manifest unavailable", http.StatusServiceUnavailable)
				return
			}
			manifest["start_url"] = "/"
			manifest["name"] = "Media Tool 媒体库"
			manifest["description"] = "独立媒体用户的授权目录"
			w.Header().Set("Content-Type", "application/manifest+json")
			json.NewEncoder(w).Encode(manifest)
			return
		}
		server.ServeHTTP(w, r)
	})
}
