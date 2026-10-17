package embyapi

import (
	"context"
	"io"
	"log/slog"
	"media-tool/internal/modules/nativelibrary"
	"net/http/httptest"
	"testing"
)

type imageResolverFunc func(context.Context, ImageRequest) (ImageResult, error)

func (f imageResolverFunc) ResolveImage(ctx context.Context, r ImageRequest) (ImageResult, error) {
	return f(ctx, r)
}
func TestNativeImageEndpointETagHEADAnonymousAndDisabled(t *testing.T) {
	settings := settingsFixture()
	settings.Enabled = true
	var got ImageRequest
	server, err := NewServer(ServerOptions{Settings: nativeSettingsFunc(func(context.Context) (nativelibrary.ServerSettings, error) { return settings, nil }), Images: imageResolverFunc(func(_ context.Context, r ImageRequest) (ImageResult, error) {
		got = r
		return ImageResult{Body: []byte("image"), ContentType: "image/jpeg", Tag: nativelibrary.ImageTag("/poster.jpg")}, nil
	}), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/emby/Items/100001/Images/Primary?maxWidth=342&tag=old&unknown=x", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != 200 || w.Body.String() != "image" || got.RequestedWidth != 342 || got.Type != "Primary" {
		t.Fatal(w.Code, w.Body.String(), got)
	}
	tag := w.Header().Get("ETag")
	if tag == "" || w.Header().Get("Cache-Control") == "" {
		t.Fatal(w.Header())
	}
	req = httptest.NewRequest("GET", "/Items/100001/Images/Primary", nil)
	req.Header.Set("If-None-Match", tag)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != 304 || w.Body.Len() != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	req = httptest.NewRequest("HEAD", "/Items/100001/Images/Backdrop/2", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != 200 || w.Body.Len() != 0 || got.Index != 2 {
		t.Fatal(w.Code, w.Body.Len(), got)
	}
	settings.Enabled = false
	req = httptest.NewRequest("GET", "/Items/100001/Images/Primary", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatal("disabled image", w.Code)
	}
}
func TestNativeImageEndpointReturns404WithoutResolverOrUnexposed(t *testing.T) {
	settings := settingsFixture()
	server, err := NewServer(ServerOptions{Settings: nativeSettingsFunc(func(context.Context) (nativelibrary.ServerSettings, error) { return settings, nil }), Images: imageResolverFunc(func(context.Context, ImageRequest) (ImageResult, error) { return ImageResult{}, ErrImageNotFound })})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	server.ServeHTTP(w, httptest.NewRequest("GET", "/Items/999999/Images/Primary", nil))
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}
