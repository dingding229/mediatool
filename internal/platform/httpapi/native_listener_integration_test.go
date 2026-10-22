package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"log/slog"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/settings"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestNativeListenerDatabaseSmoke(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated media_tool_test required")
	}
	parsed, err := pgxpool.ParseConfig(dsn)
	if err != nil || parsed.ConnConfig.Database != "media_tool_test" {
		t.Fatal("only media_tool_test is allowed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db, err := pgxpool.NewWithConfig(ctx, parsed)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name, serverID, serverName, version string
	var enabled, publicUsers bool
	var revision int64
	var updated time.Time
	var application []byte
	if err := db.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("isolated DB guard", err)
	}
	// DML suites can reset these rows without resetting the schema version.
	for _, table := range []string{"native_server_settings", "application_settings"} {
		query := "INSERT INTO " + table + " DEFAULT VALUES ON CONFLICT DO NOTHING"
		if table == "native_server_settings" {
			query = "INSERT INTO native_server_settings(server_id) VALUES(repeat('e',32)) ON CONFLICT DO NOTHING"
		}
		tag, err := db.Exec(ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		if tag.RowsAffected() == 1 {
			t.Cleanup(func() {
				cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
				defer done()
				pool, err := pgxpool.NewWithConfig(cleanup, parsed)
				if err != nil {
					t.Error(err)
					return
				}
				defer pool.Close()
				if _, err := pool.Exec(cleanup, "DELETE FROM "+table+" WHERE singleton"); err != nil {
					t.Error(err)
				}
			})
		}
	}
	if err := db.QueryRow(ctx, `SELECT server_id,server_name,version,enabled,public_user_list,revision,updated_at FROM native_server_settings WHERE singleton`).Scan(&serverID, &serverName, &version, &enabled, &publicUsers, &revision, &updated); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT value FROM application_settings WHERE singleton`).Scan(&application); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		tag, err := db.Exec(cleanup, `UPDATE native_server_settings SET server_name=$1,version=$2,enabled=$3,public_user_list=$4,revision=$5,updated_at=$6 WHERE singleton`, serverName, version, enabled, publicUsers, revision, updated)
		if err != nil || tag.RowsAffected() != 1 {
			t.Error("restore native test settings", err)
		}
		tag, err = db.Exec(cleanup, `UPDATE application_settings SET value=$1 WHERE singleton`, application)
		if err != nil || tag.RowsAffected() != 1 {
			t.Error("restore application test settings", err)
		}
	}()
	if tag, err := db.Exec(ctx, `UPDATE application_settings SET value=jsonb_set(value,'{access}',coalesce(value->'access','{}'::jsonb)||'{"memberUrl":"https://database-native.fixture.invalid"}'::jsonb,true) WHERE singleton`); err != nil || tag.RowsAffected() != 1 {
		t.Fatal("native origin fixture", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	configuration := cfg()
	configuration.MemberPublicURL = "https://environment-native.fixture.invalid"
	handler, err := NativeMediaHandler(configuration, Dependencies{DB: db, Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	instance := httptest.NewServer(handler)
	defer instance.Close()
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	type endpoint struct{ method, path string }
	var endpoints []endpoint
	raw, err := os.ReadFile("../../../docs/NATIVE_EMBY_SERVER.md")
	if err != nil {
		t.Fatal(err)
	}
	sections := strings.Split(string(raw), "### 媒体端口端点")
	if len(sections) != 2 {
		t.Fatal("missing media endpoint table")
	}
	table := strings.Split(sections[1], "### 管理端口端点")[0]
	for _, row := range regexp.MustCompile("(?m)^\\| (GET|POST|PUT|DELETE) \\| `([^`]+)` \\|").FindAllStringSubmatch(table, -1) {
		path := strings.NewReplacer("{ext}", "mkv", "{format}", "vtt", "{type}", "Primary", "{index}", "0", "*", "Countries").Replace(row[2])
		path = regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(path, "100001")
		endpoints = append(endpoints, endpoint{row[1], path})
	}
	if len(endpoints) == 0 {
		t.Fatal("no endpoints discovered")
	}
	requests := 0
	call := func(method, path string) (int, []byte) {
		t.Helper()
		request, err := http.NewRequestWithContext(ctx, method, instance.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("User-Agent", "SyntheticSmokePlayer/1")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 65537))
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil || len(body) > 65536 {
			t.Fatal("bounded response", readErr, closeErr)
		}
		requests++
		if method == "HEAD" {
			if len(body) != 0 {
				t.Fatal("HEAD wrote body", path)
			}
		} else if response.StatusCode == 204 {
			if len(body) != 0 {
				t.Fatal("204 body", path)
			}
		} else if !strings.HasPrefix(response.Header.Get("Content-Type"), "text/css") && !json.Valid(body) {
			t.Fatal("non-JSON response", path, string(body))
		}
		return response.StatusCode, body
	}
	for _, on := range []bool{true, false} {
		if tag, err := db.Exec(ctx, `UPDATE native_server_settings SET server_name='Synthetic Smoke Server',version='4.8.12.1',enabled=$1,revision=revision+1 WHERE singleton`, on); err != nil || tag.RowsAffected() != 1 {
			t.Fatal(err)
		}
		want := 401
		if !on {
			want = 503
		}
		deadline := time.Now().Add(time.Second)
		for {
			code, _ := call("GET", "/System/Info")
			if code == want {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("settings change failed to converge within 1s")
			}
			time.Sleep(10 * time.Millisecond)
		}
		for _, e := range endpoints {
			for _, prefix := range []string{"", "/emby", "/mediabrowser"} {
				methods := []string{e.method}
				if e.method == "GET" {
					methods = append(methods, "HEAD")
				}
				for _, method := range methods {
					path := prefix + e.path
					status, body := call(method, path)
					expected := 401
					if !on {
						expected = 503
					}
					if e.path == "/System/Info/Public" || e.path == "/System/Ping" {
						expected = 200
					} else if on {
						switch e.path {
						case "/Users/AuthenticateByName":
							expected = 400
						case "/Users/Public", "/System/Endpoint", "/Branding/Configuration", "/Branding/Css", "/Branding/Css.css":
							expected = 200
						case "/Sessions/Capabilities", "/Sessions/Capabilities/Full":
							expected = 401
						}
					}
					// Native artwork is authenticated; anonymous probes remain 401 even for a missing image.
					if status != expected {
						t.Fatalf("enabled=%v %s %s: got %d want %d", on, method, path, status, expected)
					}
					if method == "GET" && e.path == "/System/Info/Public" {
						var info dto.PublicSystemInfo
						if err := json.Unmarshal(body, &info); err != nil {
							t.Fatal(err)
						}
						if info.Id != serverID || info.ServerName != "Synthetic Smoke Server" || info.Version != "4.8.12.1" || info.LocalAddress != "https://database-native.fixture.invalid" {
							t.Fatal("database identity/origin not used")
						}
					}
				}
			}
		}
		for _, path := range []string{"/api/v1/media-server/users", "/assets/app.js", "/dashboard", "/manifest.webmanifest", "/sw.js"} {
			for _, method := range []string{"GET", "HEAD", "POST"} {
				code, _ := call(method, path)
				if code != 404 {
					t.Fatal("management leaked", method, path, code)
				}
			}
		}
	}
	management := httptest.NewServer(New(configuration, Dependencies{Logger: logger}))
	defer management.Close()
	response, err := client.Get(management.URL + "/System/Info/Public")
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 65536))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil || strings.Contains(string(body), `"StartupWizardCompleted"`) {
		t.Fatal("management answered native route", readErr, closeErr)
	}
	t.Log(fmt.Sprintf("native listener smoke: %d unique method/path endpoints, %d local HTTP checks; settings cache TTL %s", len(endpoints), requests, settings.NativeServerCacheTTL))
}
