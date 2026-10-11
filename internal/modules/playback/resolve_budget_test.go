package playback

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/storage"
)

func TestResolveBudgetSurvivesOldTenSecondCeilingWithoutMediaRelay(t *testing.T) {
	if testing.Short() {
		t.Skip("real clock regression exceeds former 10s ceiling")
	}
	source := resolverFunc(func(ctx context.Context, ref storage.FileRef) (storage.DownloadLink, error) {
		if remaining := time.Until(mustDeadline(t, ctx)); remaining < 28*time.Second {
			t.Error("default aggregate resolution budget still too short", remaining)
		}
		select {
		case <-time.After(10200 * time.Millisecond):
			return storage.DownloadLink{URL: "https://cdn.example.com/owned", CacheUntil: time.Now().Add(time.Minute)}, nil
		case <-ctx.Done():
			return storage.DownloadLink{}, ctx.Err()
		}
	})
	e := New(source, Options{Policy: Policy{Lookup: publicLookup}})
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "http://gateway/fixed", nil)
	e.ServeRedirect(w, r, storage.FileRef{Path: "/owned"})
	if w.Code != 302 || w.Header().Get("Location") != "https://cdn.example.com/owned" || w.Header().Get("X-Media-Delivery") != "redirect" {
		t.Fatal("cold resolution clipped at old deadline", w.Code)
	}
}
func mustDeadline(t *testing.T, ctx context.Context) time.Time {
	t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("missing bound")
	}
	return deadline
}
func TestResolutionTimeoutReturns504AndNeverFallbackOrSignedURL(t *testing.T) {
	var stages []storage.ResolveObservation
	source := resolverFunc(func(ctx context.Context, _ storage.FileRef) (storage.DownloadLink, error) {
		<-ctx.Done()
		return storage.DownloadLink{}, ctx.Err()
	})
	e := New(source, Options{ResolveTimeout: 25 * time.Millisecond, ProxyFallback: true, ForceProxy: true})
	r := httptest.NewRequest("GET", "http://gateway/fixed?apiKey=PRIVATE-GRANT", nil)
	r = r.WithContext(storage.WithResolveObserver(r.Context(), func(e storage.ResolveObservation) { stages = append(stages, e) }))
	w := httptest.NewRecorder()
	e.ServeRedirect(w, r, storage.FileRef{Path: "/PRIVATE-PATH"})
	if w.Code != http.StatusGatewayTimeout || !strings.Contains(w.Body.String(), "cloud_resolution_timeout") || strings.Contains(w.Body.String(), "PRIVATE") || w.Header().Get("Location") != "" || w.Header().Get("X-Media-Delivery") == "proxy" {
		t.Fatal("unsafe timeout delivery", w.Code)
	}
	if len(stages) != 1 || stages[0].Stage != storage.StageResolve || stages[0].Outcome != "timeout" {
		t.Fatal("missing timeout stage")
	}
}
func TestPlayerCancellationAbortsInsteadOfWritingMisleadingError(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	stop()
	e := New(resolverFunc(func(ctx context.Context, _ storage.FileRef) (storage.DownloadLink, error) {
		return storage.DownloadLink{}, ctx.Err()
	}), Options{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "http://gateway/fixed", nil).WithContext(ctx)
	defer func() {
		if caught := recover(); !errors.Is(asError(caught), http.ErrAbortHandler) || w.Body.Len() != 0 {
			t.Fatal("cancelled player received error payload", caught)
		}
	}()
	e.ServeRedirect(w, r, storage.FileRef{Path: "/owned"})
	t.Fatal("cancelled playback did not abort")
}
func asError(v any) error { err, _ := v.(error); return err }

func TestTargetLookupIsBoundedAndPreservesCancellationOrTimeout(t *testing.T) {
	source := resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		return storage.DownloadLink{URL: "https://cdn.example.com/owned"}, nil
	})
	e := New(source, Options{Policy: Policy{Lookup: func(ctx context.Context, _ string) ([]net.IPAddr, error) { <-ctx.Done(); return nil, ctx.Err() }}})
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer stop()
	if _, err := e.Inspect(ctx, storage.FileRef{Path: "/owned"}, "player"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("DNS budget misreported as unsafe target", err)
	}
}
