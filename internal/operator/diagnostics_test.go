package operator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"media-tool/internal/modules/playback"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(n int, h http.Header, b string) *http.Response {
	return &http.Response{StatusCode: n, Header: h, Body: io.NopCloser(strings.NewReader(b))}
}
func TestStrict302NeverAcceptsRelayAndNeverLeaksCredentialsToCDN(t *testing.T) {
	for _, status := range []int{200, 206, 302} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			cdnCalls := 0
			d := diagnostic{Agent: "actual-player-UA", HTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("Cookie") != "PRIVATE-ADMIN" {
					t.Fatal("missing owned identity")
				}
				return response(status, http.Header{"Location": []string{"https://cdn.fixture.invalid/media?signature=PRIVATE-CDN"}, "X-Media-Gateway": []string{"emby-static"}}, ""), nil
			}), CheckRedirect: noRedirect}, CDN: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				cdnCalls++
				if r.Header.Get("Cookie") != "" || r.Header.Get("X-Emby-Token") != "" || r.Header.Get("Authorization") != "" || r.UserAgent() != "actual-player-UA" || r.Header.Get("Range") != "bytes=0-1023" {
					t.Fatal("CDN header leak", r.Header)
				}
				return response(206, http.Header{"Content-Type": []string{"video/mp4"}}, "sample"), nil
			})}}
			ok := d.redirect(context.Background(), "http://localhost/stream", http.Header{"Cookie": []string{"PRIVATE-ADMIN"}, "X-Emby-Token": []string{"PRIVATE-USER"}}, true)
			if ok != (status == 302) || cdnCalls != map[bool]int{true: 1, false: 0}[status == 302] {
				t.Fatal("relay accepted", ok, cdnCalls)
			}
		})
	}
}
func TestCDNPolicyRejectsPrivateMixedDNSAndHTTP(t *testing.T) {
	for _, target := range []string{"http://public.fixture/media", "https://127.0.0.1/media", "https://mixed.fixture/media", "https://user:secret@public.fixture/media"} {
		called := false
		policy := playback.Policy{Lookup: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("127.0.0.1")}}, nil
		}}
		tr := checkedTransport{base: roundTrip(func(*http.Request) (*http.Response, error) { called = true; return response(206, nil, "sample"), nil }), policy: policy}
		r, _ := http.NewRequest("GET", target, nil)
		if _, e := tr.RoundTrip(r); e == nil || called {
			t.Fatal("unsafe CDN reached", target)
		}
	}
}
func TestOwnedOriginsRefuseCredentialExfiltration(t *testing.T) {
	for _, s := range []string{"http://example.com", "https://user:pass@example.com", "https://example.com/path", "https://example.com?q=secret", "https://example.com#x"} {
		if _, e := origin(s, true); e == nil {
			t.Fatal(s)
		}
	}
	if _, e := origin("http://localhost:8080", true); e != nil {
		t.Fatal(e)
	}
}
func TestMissingProfilesReportNoPlaybackAndNoSecrets(t *testing.T) {
	r := t.TempDir()
	if e := playbackCheck(r, nil); !errors.Is(e, ErrNeedsConfiguration) {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(r, "data/diagnostics/playback-check.json"))
	if e != nil {
		t.Fatal(e)
	}
	var v map[string]any
	json.Unmarshal(b, &v)
	if v["status"] != "needs_configuration" || v["serverChainVerified"] != false || v["realPlayerVerified"] != false || strings.Contains(string(b), "PRIVATE-") {
		t.Fatal(string(b))
	}
	st, _ := os.Stat(filepath.Join(r, "data/diagnostics/playback-check.json"))
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
}
func TestReadinessLoadIsBoundedAndNotMediaClaim(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			t.Fatal(r.URL.Path)
		}
		io.WriteString(w, `{"data":{"status":"ready"}}`)
	}))
	defer server.Close()
	r := t.TempDir()
	if e := loadCheck(r, []string{"--base-url", server.URL, "--requests", "7", "--concurrency", "2"}); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(r, "data/diagnostics/readiness-load.json"))
	if !strings.Contains(string(b), "no media/CDN/client throughput claim") {
		t.Fatal(string(b))
	}
	for _, a := range [][]string{{"--requests", "10001"}, {"--concurrency", "65"}, {"--requests", "0"}} {
		if e := loadCheck(r, a); e == nil {
			t.Fatal("unbounded", a)
		}
	}
}
func TestCloneVerificationOnlyDeletesItsFreshOwnedDatabase(t *testing.T) {
	for _, restoreFails := range []bool{false, true} {
		t.Run(fmt.Sprint(restoreFails), func(t *testing.T) {
			r := t.TempDir()
			os.WriteFile(filepath.Join(r, "docker-compose.yml"), []byte("owned fixture"), 0600)
			archive := filepath.Join(r, "owned.dump")
			os.WriteFile(archive, []byte("synthetic fixture"), 0600)
			bin := filepath.Join(r, "bin")
			os.Mkdir(bin, 0700)
			record := filepath.Join(r, "commands")
			app := container{}
			app.Config.Env = []string{"DATABASE_URL=postgres://fixture@postgres/media_tool"}
			app.NetworkSettings.Networks = map[string]struct {
				Aliases   []string
				IPAddress string
			}{"owned": {}}
			pg := container{}
			pg.Name = "/owned-postgres"
			pg.NetworkSettings.Networks = map[string]struct {
				Aliases   []string
				IPAddress string
			}{"owned": {Aliases: []string{"postgres"}}}
			appJSON := string(jsonBytes([]container{app}))
			pgJSON := string(jsonBytes([]container{pg}))
			fail := "0"
			if restoreFails {
				fail = "1"
			}
			script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + shellQuote(record) + "\ncase \"$*\" in\n*'config --services'*) printf 'app\\npostgres\\nredis\\n';;\n*'ps -q app'*) echo fixture-app;;\n*'ps -q postgres'*) echo fixture-postgres;;\n'inspect fixture-app') printf '%s' " + shellQuote(appJSON) + ";;\n'inspect fixture-postgres') printf '%s' " + shellQuote(pgJSON) + ";;\n*'pg_restore --exit-on-error'*) cat >/dev/null;exit " + fail + ";;\n*'backup verify -database'*) printf '%s' '{\"initialTables\":{}}';;\nesac\n"
			os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0700)
			t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
			e := backupVerify(r, archive)
			if (e != nil) != restoreFails {
				t.Fatal(e)
			}
			b, _ := os.ReadFile(record)
			created := regexp.MustCompile(`createdb[^\n]+(media_tool_restore_[a-f0-9]{12})`).FindStringSubmatch(string(b))
			dropped := regexp.MustCompile(`dropdb[^\n]+(media_tool_restore_[a-f0-9]{12})`).FindStringSubmatch(string(b))
			if len(created) != 2 || len(dropped) != 2 || created[1] != dropped[1] {
				t.Fatal("unscoped cleanup", string(b))
			}
			if _, e = os.Stat(archive); e != nil {
				t.Fatal("backup deleted")
			}
		})
	}
}

func TestInteractivePasswordPreservesSpacesAndUnicode(t *testing.T) {
	for _, s := range []string{"with spaces", " 空格密碼 ", "single"} {
		got, e := readPassword(strings.NewReader(s + "\r\n"))
		if e != nil || got != s {
			t.Fatal("password changed", e)
		}
	}
}
func TestSchemaLabelMustRemainDecimal(t *testing.T) {
	for _, v := range []string{"+65", "-65", "65 ", "0x41"} {
		i := fixtureImage()
		i.Config.Labels["org.opencontainers.image.schema"] = v
		if _, e := CheckImage("dingding229/mediatool:sha-"+strings.Repeat("a", 40), i); e == nil {
			t.Fatal(v)
		}
	}
}
