package embyapi

import (
	"bytes"
	"context"
	"encoding/json"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativeplayback"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type reportStub struct {
	playbackStub
	report nativeplayback.Report
}

func (p *reportStub) Report(_ context.Context, a nativeplayback.Actor, q nativeplayback.Report) error {
	p.report = q
	p.seen = a
	p.calls++
	return p.err
}
func TestPlaybackReportHTTPContractsAndValidation(t *testing.T) {
	raw, e := os.ReadFile("testdata/contracts/playback-reports.json")
	if e != nil {
		t.Fatal(e)
	}
	var fixtures []struct {
		Method, Path string
		Body         *struct {
			ItemID   string `json:"ItemId,omitempty"`
			Source   string `json:"MediaSourceId,omitempty"`
			Session  string `json:"PlaySessionId,omitempty"`
			Position *int64 `json:"PositionTicks,omitempty"`
			Paused   *bool  `json:"IsPaused,omitempty"`
			Event    string `json:"EventName,omitempty"`
		}
		Status int
		Kind   nativeplayback.ReportKind
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if e = dec.Decode(&fixtures); e != nil {
		t.Fatal(e)
	}
	value := settingsFixture()
	play := &reportStub{}
	s, e := NewServer(ServerOptions{Playback: play, Auth: &fakeAccountService{user: contracts.User{ID: strings.Repeat("a", 32), Enabled: true}}, Settings: nativeSettingsFunc(func(context.Context) (nativelibrary.ServerSettings, error) { return value, nil })})
	if e != nil {
		t.Fatal(e)
	}
	call := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("X-Emby-Token", "valid-token")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	for _, f := range fixtures {
		body := ""
		if f.Body != nil {
			b, e := json.Marshal(f.Body)
			if e != nil {
				t.Fatal(e)
			}
			body = string(b)
		}
		w := call(f.Method, f.Path, body)
		if w.Code != f.Status || w.Body.Len() != 0 || play.report.Kind != f.Kind {
			t.Fatal(f.Path, w.Code, w.Body.String(), play.report)
		}
	}
	for _, f := range []struct {
		path, body string
		status     int
	}{
		{"/Sessions/Playing", `{"PositionTicks":-1}`, 400}, {"/Sessions/Playing", `{"PositionTicks":9223372036854775808}`, 400},
		{"/Sessions/Playing", `{"IsPaused":[]}`, 400}, {"/Sessions/Playing", `{"ItemId":"1","itemID":"2"}`, 400},
		{"/Sessions/Playing?PositionTicks=1", `{"PositionTicks":2}`, 400}, {"/Sessions/Playing", `{"IsPaused":true,"EventName":"Unpause"}`, 400},
		{"/Sessions/Playing", `{"UserId":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`, 403},
		{"/Users/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/PlayingItems/1", `{}`, 403},
		{"/Sessions/Playing", `{"DeviceProfile":{"huge":"` + strings.Repeat("x", 256<<10) + `"}}`, 400},
	} {
		before := play.calls
		w := call("POST", f.path, f.body)
		if w.Code != f.status || before != play.calls {
			t.Fatal(f.path, w.Code, w.Body.String())
		}
	}
	value.Enabled = false
	if w := call("POST", "/Sessions/Playing", `{}`); w.Code != 503 {
		t.Fatal(w.Code)
	}
}
