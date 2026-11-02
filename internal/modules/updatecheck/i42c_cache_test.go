package updatecheck

import (
	"encoding/json"
	"media-tool/internal/platform/buildinfo"
	"testing"
	"time"
)

func TestI42cCachedReleaseComparedToExecutable(t *testing.T) {
	for _, tc := range []struct {
		version                  string
		hasUpdate, current, keep bool
	}{{"0.49.0", false, true, false}, {"0.51.0", false, true, true}, {"0.52.0", true, false, true}} {
		t.Run(tc.version, func(t *testing.T) {
			at := time.Now().UTC()
			release := &Release{Metadata: Metadata{Version: tc.version}}
			in := State{Enabled: true, IntervalHours: 12, Current: buildinfo.Info{Version: "0.51.0"}, Result: Result{Status: "verified", Latest: release, CheckedAt: at}}
			out := projectCachedRelease(in)
			if out.HasUpdate != tc.hasUpdate || out.CurrentAtLeastLatest != tc.current || (out.Result.Latest != nil) != tc.keep {
				t.Fatalf("%+v", out)
			}
			if out.Result.CheckedAt != at || out.IntervalHours != 12 || in.Result.Latest != release {
				t.Fatal("view mutated cache/interval/provenance")
			}
		})
	}
}
func TestI42cGetIgnoresOlderCacheWithoutWriting(t *testing.T) {
	s := isolatedService(t)
	s.Current = func() buildinfo.Info { return buildinfo.Info{Version: "0.51.0"} }
	at := time.Now().UTC()
	if e := s.store(t.Context(), Result{Status: "verified", Latest: &Release{Metadata: Metadata{Version: "0.49.0"}}, CheckedAt: at}); e != nil {
		t.Fatal(e)
	}
	out, e := s.Get(t.Context())
	if e != nil || !out.CurrentAtLeastLatest || out.HasUpdate || out.Result.Latest != nil {
		t.Fatal(out, e)
	}
	var raw []byte
	if e = s.DB.QueryRow(t.Context(), `SELECT value FROM app_settings WHERE key=$1`, settingsKey).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var stored State
	if e = json.Unmarshal(raw, &stored); e != nil || stored.Result.Latest.Version != "0.49.0" {
		t.Fatal(string(raw), e)
	}
	if shouldCheck(out, at.Add(time.Hour)) {
		t.Fatal("read projection changed checker cadence")
	}
}
