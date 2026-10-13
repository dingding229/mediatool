package qbittorrent

import (
	"context"
	"fmt"
	"media-tool/internal/modules/downloads"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAuthenticationAddInspectFilesAndVersionedPause(t *testing.T) {
	var logins, adds, stops atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			r.ParseForm()
			if r.Form.Get("username") != "user" || r.Form.Get("password") != "secret" {
				w.WriteHeader(403)
				return
			}
			logins.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "sid-token", Path: "/"})
			fmt.Fprint(w, "Ok.")
			return
		}
		cookie, err := r.Cookie("SID")
		if err != nil || cookie.Value != "sid-token" {
			w.WriteHeader(403)
			return
		}
		switch r.URL.Path {
		case "/api/v2/app/version":
			fmt.Fprint(w, "v5.0.4")
		case "/api/v2/torrents/add":
			r.ParseMultipartForm(1 << 20)
			if r.FormValue("savepath") != "/downloads" {
				t.Error("save path lost")
			}
			adds.Add(1)
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/info":
			fmt.Fprintf(w, `[{"hash":%q,"name":"movie","state":"stalledUP","progress":1,"save_path":"/downloads","content_path":"/downloads/movie.mkv","size":100,"amount_left":0,"completion_on":1}]`, r.URL.Query().Get("hashes"))
		case "/api/v2/torrents/files":
			fmt.Fprint(w, `[{"index":0,"name":"movie.mkv","size":100,"progress":1,"priority":1}]`)
		case "/api/v2/torrents/stop":
			stops.Add(1)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	a := &Adapter{Endpoint: server.URL, Username: "user", Password: "secret"}
	v, err := a.Probe(context.Background())
	if err != nil || v != "v5.0.4" {
		t.Fatal(err)
	}
	torrent := []byte("d4:infod6:lengthi100e4:name9:movie.mkv12:piece lengthi16384e6:pieces20:" + strings.Repeat("a", 20) + "ee")
	id, err := a.Add(context.Background(), downloads.Request{Torrent: torrent, SavePath: "/downloads", Category: "media-tool"})
	if err != nil || len(id) != 40 {
		t.Fatal(err, id)
	}
	task, err := a.Inspect(context.Background(), id)
	if err != nil || !task.Complete || !task.Seeding {
		t.Fatal(err)
	}
	files, err := a.Files(context.Background(), id)
	if err != nil || len(files) != 1 {
		t.Fatal(err)
	}
	if err = a.SetPaused(context.Background(), id, true); err != nil {
		t.Fatal(err)
	}
	if logins.Load() != 1 || adds.Load() != 1 || stops.Load() != 1 {
		t.Fatal("session or endpoint behavior")
	}
}

func TestInspectRejectsWrongOrDuplicateReturnedHash(t *testing.T) {
	for _, body := range []string{`[{"hash":"other"}]`, `[{"hash":"wanted"},{"hash":"wanted"}]`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		adapter := &Adapter{Endpoint: server.URL}
		if _, err := adapter.Inspect(context.Background(), "wanted"); err == nil {
			t.Fatal("unproven qB hash response accepted")
		}
		server.Close()
	}
}

func TestModernUnauthorizedAndNoContentAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") == "" || !strings.HasSuffix(r.Header.Get("Referer"), "/") {
			t.Error("CSRF request headers missing")
		}
		if r.URL.Path == "/api/v2/auth/login" {
			http.SetCookie(w, &http.Cookie{Name: "QBT_SID_8080", Value: "owned-session", Path: "/"})
			w.WriteHeader(204)
			return
		}
		if _, err := r.Cookie("QBT_SID_8080"); err != nil {
			w.WriteHeader(401)
			return
		}
		fmt.Fprint(w, "v5.2.4")
	}))
	defer server.Close()
	adapter := Adapter{Endpoint: server.URL, Username: "admin", Password: "owned-password"}
	v, err := adapter.Probe(context.Background())
	if err != nil || v != "v5.2.4" {
		t.Fatal(v, err)
	}
}
func TestModernTorrentAddResponseMustConfirmOwnedHash(t *testing.T) {
	torrent := []byte("d4:infod6:lengthi100e4:name9:movie.mkv12:piece lengthi16384e6:pieces20:" + strings.Repeat("a", 20) + "ee")
	meta, _ := downloads.ParseTorrent(torrent)
	for _, valid := range []bool{true, false} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hash := meta.InfoHash
			if !valid {
				hash = strings.Repeat("f", 40)
			}
			fmt.Fprintf(w, `{"success_count":1,"failure_count":0,"added_torrent_ids":[%q]}`, hash)
		}))
		a := Adapter{Endpoint: server.URL}
		id, err := a.Add(context.Background(), downloads.Request{Torrent: torrent, SavePath: "/downloads"})
		if valid && (err != nil || id != meta.InfoHash) {
			t.Fatal(id, err)
		}
		if !valid && err == nil {
			t.Fatal("unconfirmed hash accepted")
		}
		server.Close()
	}
}
