package embyapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativeplayback"
	"net/http/httptest"
	"strings"
	"testing"
)

type playbackStub struct {
	seen    nativeplayback.Actor
	request nativeplayback.Request
	calls   int
	err     error
}

func (p *playbackStub) Info(_ context.Context, a nativeplayback.Actor, q nativeplayback.Request) (dto.PlaybackInfoResponse, error) {
	p.seen, p.request = a, q
	p.calls++
	return dto.PlaybackInfoResponse{}, p.err
}
func (p *playbackStub) Stream(_ context.Context, a nativeplayback.Actor, q nativeplayback.Request) (nativeplayback.RedirectTarget, error) {
	p.seen, p.request = a, q
	p.calls++
	return nativeplayback.RedirectTarget{URL: "https://cdn.synthetic.invalid/video?signature=VISIBLE"}, p.err
}
func TestNativePlaybackParametersCaseVariantsEmpty302AndDenials(t *testing.T) {
	user := contracts.User{ID: strings.Repeat("a", 32), Enabled: true, MaxStreams: 2}
	auth := &fakeAccountService{user: user}
	play := &playbackStub{}
	var logs bytes.Buffer
	settings := settingsFixture()
	settings.Enabled = true
	settings.PublicURL = ""
	s, err := NewServer(ServerOptions{Logger: slog.New(slog.NewJSONHandler(&logs, nil)), Playback: play, Auth: auth, Settings: nativeSettingsFunc(func(context.Context) (nativelibrary.ServerSettings, error) { return settings, nil }), PublicURL: "https://native.synthetic.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("X-Emby-Token", "valid-token")
		r.Header.Set("User-Agent", "Infuse/8")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	for _, method := range []string{"GET", "POST", "HEAD"} {
		body := ""
		if method == "POST" {
			body = `{"deviceprofile":{"CodecProfiles":[]},"USERID":"` + user.ID + `","starttimeticks":0,"MaxStreamingBitrate":100000,"AudioStreamIndex":0,"SubtitleStreamIndex":-1,"EnableDirectPlay":false,"EnableDirectStream":true,"EnableTranscoding":true}`
		}
		w := call(method, "/EmBy/iTeMs/100031/pLaYbAcKiNfO", body)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := call("POST", "/Items/100031/PlaybackInfo", `{"DeviceProfile":null,"UserId":null,"MediaSourceId":null,"AudioStreamIndex":null,"SubtitleStreamIndex":null,"MaxStreamingBitrate":null,"EnableTranscoding":null}`); w.Code != 200 {
		t.Fatal("nullable client fields rejected", w.Code, w.Body.String())
	}
	if play.seen.Token != "valid-token" || play.seen.Origin != "https://native.synthetic.invalid" {
		t.Fatal("actor binding", play.seen.Origin, play.seen.Token)
	}
	for _, path := range []string{"/Videos/100031/stream.mkv", "/emby/videos/100031/STREAM", "/Videos/100031/original.mp4", "/Items/100031/Download"} {
		for _, method := range []string{"GET", "HEAD"} {
			w := call(method, path, "")
			if play.seen.Method != method {
				t.Fatal("original request method lost", play.seen.Method, method)
			}
			if w.Code != 302 || w.Body.Len() != 0 || w.Header().Get("Content-Length") != "0" || !strings.Contains(w.Header().Get("Location"), "VISIBLE") {
				t.Fatal(method, path, w.Code, w.Body.String())
			}
		}
	}
	if !strings.Contains(logs.String(), "VISIBLE") || strings.Contains(logs.String(), "valid-token") {
		t.Fatal("raw token in redirect log")
	}
	for _, body := range []string{`{"StartTimeTicks":9223372036854775808}`, `{"StartTimeTicks":-1}`, `{"AudioStreamIndex":65536}`, `{"EnableTranscoding":"maybe"}`, `{"DeviceProfile":"url"}`, `{"UserId":"a","USERID":"b"}`} {
		if w := call("POST", "/Items/100031/PlaybackInfo", body); w.Code != 400 {
			t.Fatal(body, w.Code)
		}
	}
	if w := call("POST", "/Items/100031/PlaybackInfo", `{"UserId":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`); w.Code != 403 {
		t.Fatal("body cross user", w.Code)
	}
	for _, path := range []string{"/Videos/100031/stream.m3u8", "/Videos/100031/stream?Static=false", "/Videos/100031/stream?VideoCodec=h264"} {
		if w := call("GET", path, ""); w.Code != 400 {
			t.Fatal("transcoding allowed", path, w.Code)
		}
	}
	for _, denied := range []error{nativeplayback.ErrForbidden, nativeplayback.ErrUADenied, nativeplayback.ErrStreamLimit, nativeplayback.ErrParentalDenied} {
		play.err = denied
		w := call("GET", "/Videos/100031/stream", "")
		var out struct{ ErrorCode, Message string }
		if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != 403 || out.ErrorCode != "NotAllowed" || out.Message == "" {
			t.Fatal(w.Code, w.Body.String(), err)
		}
	}
	play.err = &nativeplayback.RetryError{}
	if w := call("GET", "/Videos/100031/stream", ""); w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal(w.Code)
	}
	play.err = errors.New("SECRET https://private.invalid/")
	if w := call("GET", "/Videos/100031/stream", ""); w.Code != 503 || strings.Contains(w.Body.String(), "SECRET") {
		t.Fatal(w.Code, w.Body.String())
	}
}
