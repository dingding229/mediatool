package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	v2 "media-tool/internal/modules/plugins/v2"
	"media-tool/internal/platform/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type rejectedPluginStore struct{ err error }

func (s rejectedPluginStore) List(context.Context, int, int) ([]v2.Instance, error) { return nil, nil }
func (s rejectedPluginStore) Apply(context.Context, string, v2.Request) (v2.Instance, error) {
	return v2.Instance{}, s.err
}
func TestPluginOperationFailureLogsOnlySafeCodeAndRequestID(t *testing.T) {
	for _, tc := range []struct {
		err    error
		code   string
		status int
	}{{v2.ErrConflict, "plugin_conflict", 409}, {v2.ErrInvalid, "invalid_plugin_request", 400}, {errors.New("private-error-sentinel"), "plugins_unavailable", 503}} {
		t.Run(tc.code, func(t *testing.T) {
			var logs bytes.Buffer
			c := cfg()
			a := &API{Config: c, Auth: auth.Auth{Key: c.SessionKey}, Deps: Dependencies{PluginInstances: rejectedPluginStore{tc.err}, AuthSessions: testSessionReader{}, Logger: slog.New(slog.NewJSONHandler(&logs, nil))}}
			token, e := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
			if e != nil {
				t.Fatal(e)
			}
			r := httptest.NewRequest("POST", "/api/v1/plugins/operations", strings.NewReader(`{"kind":"install","operationKey":"private-key-sentinel","config":{"secret":"private-config-sentinel"}}`))
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
			w := httptest.NewRecorder()
			w.Header().Set("X-Request-ID", "abcdef0123456789abcdef01")
			a.pluginV2Operation(w, r)
			if w.Code != tc.status {
				t.Fatal(w.Code, w.Body.String())
			}
			var record map[string]any
			if e = json.Unmarshal(logs.Bytes(), &record); e != nil {
				t.Fatal(e)
			}
			if record["resultCode"] != tc.code || record["requestId"] != "abcdef0123456789abcdef01" || record["module"] != "plugins" {
				t.Fatal(record)
			}
			for k := range record {
				switch k {
				case "time", "level", "msg", "module", "resultCode", "requestId":
				default:
					t.Fatalf("unexpected log attribute %s", k)
				}
			}
			for _, sentinel := range []string{"private-error-sentinel", "private-config-sentinel", "private-key-sentinel"} {
				if strings.Contains(logs.String(), sentinel) {
					t.Fatal("private content escaped")
				}
			}
		})
	}
}
