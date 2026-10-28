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
	target  *nativeplayback.RedirectTarget
}

func (p *playbackStub) Info(_ context.Context, a nativeplayback.Actor, q nativeplayback.Request) (dto.PlaybackInfoResponse, error) {
	p.seen, p.request = a, q
	p.calls++
	return dto.PlaybackInfoResponse{}, p.err
}
func (p *playbackStub) Stream(_ context.Context, a nativeplayback.Actor, q nativeplayback.Request) (nativeplayback.RedirectTarget, error) {
	p.seen, p.request = a, q
	p.calls++
	if p.target != nil {
		return *p.target, p.err
	}
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
	for _, denied := range []error{nativeplayback.ErrForbidden, nativeplayback.ErrUADenied, nativeplayback.ErrStreamLimit} {
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

func TestUploadedSubtitleHTTPIsOnlySmallAuthenticatedText(t *testing.T) {
	text, e := nativeplayback.NewTextSubtitle("srt", []byte("1\n00:00:01,000 --> 00:00:02,000\ntext\n"))
	if e != nil {
		t.Fatal(e)
	}
	play := &playbackStub{target: &nativeplayback.RedirectTarget{Subtitle: text}}
	settings := settingsFixture()
	settings.Enabled = true
	s, e := NewServer(ServerOptions{Playback: play, Auth: &fakeAccountService{user: contracts.User{ID: strings.Repeat("a", 32), Enabled: true}}, Settings: nativeSettingsFunc(func(context.Context) (nativelibrary.ServerSettings, error) { return settings, nil }), PublicURL: "https://native.synthetic.invalid"})
	if e != nil {
		t.Fatal(e)
	}
	call := func(method, path, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		if token != "" {
			r.Header.Set("X-Emby-Token", token)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	path := "/Videos/100031/00000000-0000-4000-8000-000000000002/Subtitles/65536/Stream.srt"
	for _, method := range []string{"GET", "HEAD"} {
		w := call(method, path, "valid-token")
		if w.Code != 200 || w.Header().Get("Content-Type") != text.ContentType() || w.Header().Get("Content-Length") == "" || method == "GET" && w.Body.String() != string(text.Bytes()) || method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal(method, w.Code, w.Body.String())
		}
	}
	if w := call("GET", path, ""); w.Code != 401 {
		t.Fatal("anonymous subtitle", w.Code)
	}
	if w := call("GET", "/Videos/100031/stream.mkv", "valid-token"); w.Code != 503 {
		t.Fatal("text escaped onto media route", w.Code)
	}
	play.target.URL = "https://cdn.synthetic.invalid/video"
	if w := call("GET", path, "valid-token"); w.Code != 503 {
		t.Fatal("ambiguous redirect and body", w.Code)
	}
	for _, raw := range []string{"65536", "2147483647"} {
		if _, _, e := playbackQuery(values(t, "SubtitleStreamIndex="+raw)); e != nil {
			t.Fatal(raw, e)
		}
	}
	for _, raw := range []string{"2147483648", "-2"} {
		if _, _, e := playbackQuery(values(t, "SubtitleStreamIndex="+raw)); e == nil {
			t.Fatal(raw)
		}
	}
}
