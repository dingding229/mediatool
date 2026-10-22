package embyapi

import (
	"context"
	"io"
	"log/slog"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
	"net/http/httptest"
	"strings"
	"testing"
)

type imageResolverFunc func(context.Context, ImageRequest) (ImageResult, error)

func (f imageResolverFunc) ResolveImage(ctx context.Context, r ImageRequest) (ImageResult, error) {
	return f(ctx, r)
}
func TestNativeImageEndpointETagHEADAuthenticatedAndDisabled(t *testing.T) {
	settings := settingsFixture()
	settings.Enabled = true
	var got ImageRequest
	server, err := NewServer(ServerOptions{Auth: &fakeAccountService{user: contracts.User{ID: strings.Repeat("a", 32), Enabled: true}}, Settings: nativeSettingsFunc(func(context.Context) (nativelibrary.ServerSettings, error) { return settings, nil }), Images: imageResolverFunc(func(_ context.Context, r ImageRequest) (ImageResult, error) {
		got = r
		return ImageResult{Body: []byte("image"), ContentType: "image/jpeg", Tag: nativelibrary.ImageTag("/poster.jpg")}, nil
	}), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/emby/Items/100001/Images/Primary?maxWidth=342&tag=old&unknown=x", nil)
	w := httptest.NewRecorder()
	req.Header.Set("X-Emby-Token", "valid-token")
	server.ServeHTTP(w, req)
	if w.Code != 200 || w.Body.String() != "image" || got.RequestedWidth != 342 || got.Type != "Primary" || got.Index != 0 || got.User == nil || got.Permissions == nil {
		t.Fatal(w.Code, w.Body.String(), got)
	}
	tag := w.Header().Get("ETag")
	if tag == "" || w.Header().Get("Cache-Control") == "" {
		t.Fatal(w.Header())
	}
	req = httptest.NewRequest("GET", "/Items/100001/Images/Primary", nil)
	req.Header.Set("If-None-Match", tag)
	w = httptest.NewRecorder()
	req.Header.Set("X-Emby-Token", "valid-token")
	server.ServeHTTP(w, req)
	if w.Code != 304 || w.Body.Len() != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	req = httptest.NewRequest("HEAD", "/Items/100001/Images/Backdrop/2", nil)
	w = httptest.NewRecorder()
	req.Header.Set("X-Emby-Token", "valid-token")
	server.ServeHTTP(w, req)
	if w.Code != 200 || w.Body.Len() != 0 || got.Index != 2 {
		t.Fatal(w.Code, w.Body.Len(), got)
	}
	settings.Enabled = false
	req = httptest.NewRequest("GET", "/Items/100001/Images/Primary", nil)
	w = httptest.NewRecorder()
	req.Header.Set("X-Emby-Token", "valid-token")
	server.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatal("disabled image", w.Code)
	}
}
func TestNativeImageEndpointReturns404WithoutResolverOrUnexposed(t *testing.T) {
	settings := settingsFixture()
	server, err := NewServer(ServerOptions{Auth: &fakeAccountService{user: contracts.User{ID: strings.Repeat("a", 32), Enabled: true}}, Settings: nativeSettingsFunc(func(context.Context) (nativelibrary.ServerSettings, error) { return settings, nil }), Images: imageResolverFunc(func(context.Context, ImageRequest) (ImageResult, error) { return ImageResult{}, ErrImageNotFound })})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/Items/999999/Images/Primary", nil)
	req.Header.Set("X-Emby-Token", "valid-token")
	server.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}

func TestF4ImageAuthenticationTransportsAndClientSequence(t *testing.T) {
	user := contracts.User{ID: strings.Repeat("a", 32), Enabled: true}
	calls := 0
	server := authTestServer(t, &fakeAccountService{user: user}, imageResolverFunc(func(_ context.Context, r ImageRequest) (ImageResult, error) {
		calls++
		if r.User == nil || r.Permissions == nil || r.Index != 0 {
			t.Fatal("image auth context/default index", r)
		}
		return ImageResult{Body: []byte("image"), ContentType: "image/jpeg", Tag: "synthetic-tag"}, nil
	}), nil)
	for _, ua := range []string{"SenPlayer/6.2.1", "Infuse/8.3.6", "GenericEmbyClient/1.0"} {
		for _, transport := range []string{"X-Emby-Token", "X-Emby-Authorization", "api_key", "Authorization"} {
			for _, method := range []string{"GET", "HEAD"} {
				path := "/Items/100000/Images/Logo?tag=synthetic-tag&quality=90&maxWidth=320"
				if transport == "api_key" {
					path += "&api_key=valid-token"
				}
				r := httptest.NewRequest(method, path, nil)
				r.Header.Set("User-Agent", ua)
				switch transport {
				case "X-Emby-Token":
					r.Header.Set(transport, "valid-token")
				case "X-Emby-Authorization":
					r.Header.Set(transport, `Emby Client="SenPlayer", Token="valid-token"`)
				case "Authorization":
					r.Header.Set(transport, `MediaBrowser Client="Generic", Token="valid-token"`)
				}
				w := httptest.NewRecorder()
				server.ServeHTTP(w, r)
				if w.Code != 200 {
					t.Fatal(ua, transport, method, w.Code, w.Body.String())
				}
				if method == "HEAD" && w.Body.Len() != 0 {
					t.Fatal("HEAD body")
				}
			}
		}
	}
	before := calls
	r := httptest.NewRequest("GET", "/Items/100000/Images/Logo", nil)
	r.Header.Set("If-None-Match", `"synthetic-tag"`)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, r)
	if w.Code != 401 || calls != before {
		t.Fatal("anonymous conditional response bypassed authentication", w.Code, calls, before)
	}
}
