package migrations_test

import (
	"crypto/sha256"
	"encoding/json"
	"media-tool/internal/modules/embyapi/dto"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Real HTTP auth + SQL grants + original STRM parser + fake official provider.
// No redirects are followed, and every UA is observed at the provider boundary.
func TestF2ClientPathPlaybackAuthenticationSequence(t *testing.T) {
	f := newPlaybackFixture(t)
	hashes := map[string][32]byte{}
	err := filepath.WalkDir(f.root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() && strings.HasSuffix(p, ".strm") {
			b, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			hashes[p] = sha256.Sum256(b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"SenPlayer/6.2.1", "Infuse/8", "Generic-Emby-Client/1.0"} {
		t.Run(agent, func(t *testing.T) {
			var detail dto.BaseItemDto
			w := f.call("GET", "/Users/"+strings.ReplaceAll(f.user.ID, "-", "")+"/Items/"+f.item, f.token, agent)
			if e := json.Unmarshal(w.Body.Bytes(), &detail); e != nil || w.Code != 200 || len(detail.MediaSources) != 2 {
				t.Fatal(w.Code, e, w.Body.String())
			}
			var info dto.PlaybackInfoResponse
			w = f.call("GET", "/Items/"+f.item+"/PlaybackInfo", f.token, agent)
			if e := json.Unmarshal(w.Body.Bytes(), &info); e != nil || w.Code != 200 || len(info.MediaSources) != 2 {
				t.Fatal(w.Code, e, w.Body.String())
			}
			for _, sources := range [][]dto.MediaSourceInfo{detail.MediaSources, info.MediaSources} {
				m := sources[0]
				u, e := url.Parse(m.Path)
				if e != nil || u.Host != "native.synthetic.invalid" || len(u.Query()) != 2 || m.Name != detail.Name || m.SupportsTranscoding() {
					t.Fatal("Path/source contract", m, e)
				}
				for _, credential := range []string{"api_key", "Token", "cookie", "apiKey", "115"} {
					if strings.Contains(m.Path, credential) {
						t.Fatal("credential in Path", m.Path)
					}
				}
				if w := f.call("GET", u.RequestURI(), "", agent); w.Code != 401 {
					t.Fatal("anonymous Path", w.Code)
				}
				for _, auth := range []string{"X-Emby-Token", "X-Emby-Authorization", "api_key", "Authorization"} {
					r := httptest.NewRequest("GET", u.RequestURI(), nil)
					r.Header.Set("User-Agent", agent)
					switch auth {
					case "api_key":
						q := r.URL.Query()
						q.Set("api_key", f.token)
						r.URL.RawQuery = q.Encode()
					case "X-Emby-Token":
						r.Header.Set(auth, f.token)
					default:
						r.Header.Set(auth, `MediaBrowser Token="`+f.token+`"`)
					}
					response := httptest.NewRecorder()
					f.handler.ServeHTTP(response, r)
					if response.Code != 302 || response.Body.Len() != 0 || !strings.HasPrefix(response.Header().Get("Location"), "https://cdn.synthetic.invalid/") {
						t.Fatal(auth, response.Code, response.Body.String())
					}
				}
			}
			if _, ok := f.agents.Load(agent); !ok {
				t.Fatal("player UA not forwarded")
			}
		})
	}
	pathURL := "/Videos/" + f.item + "/stream.mkv?Static=true&MediaSourceId=" + f.a.asset
	f.exec(t, `UPDATE media_user_library_grants SET can_play=false WHERE user_id=$1 AND library_id='g:test-a'`, f.user.ID)
	if w := f.call("GET", pathURL, f.token, "SenPlayer/6.2.1"); w.Code != 403 {
		t.Fatal("browse-only Path allowed", w.Code)
	}
	f.exec(t, `UPDATE native_libraries SET hidden=true WHERE id='g:test-a'`)
	if w := f.call("GET", "/Items/"+f.item, f.token, "SenPlayer/6.2.1"); w.Code != 404 {
		t.Fatal("hidden detail leaked", w.Code)
	}
	for p, h := range hashes {
		b, e := os.ReadFile(p)
		if e != nil || sha256.Sum256(b) != h {
			t.Fatal("STRM mutated", p, e)
		}
	}
	if len(hashes) == 0 {
		t.Fatal("no original STRM evidence")
	}
}
