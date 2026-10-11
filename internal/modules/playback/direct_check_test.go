package playback

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"media-tool/internal/modules/storage"
)

func successfulSample(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "video/x-matroska")
	w.Header().Set("Content-Range", "bytes 0-15/16")
	w.WriteHeader(http.StatusPartialContent)
	io.WriteString(w, "0123456789abcdef")
}

func TestDirectCheckPositiveCacheNeverSkipsResolverOrPolicy(t *testing.T) {
	var probes, resolves, lookups atomic.Int32
	client, policy := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		probes.Add(1)
		if r.Header.Get("Range") != "bytes=0-1023" || r.UserAgent() != "player" || r.Header.Get("Authorization") != "" {
			t.Error("unsafe probe")
		}
		successfulSample(w)
	})
	policy.Lookup = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		lookups.Add(1)
		return publicLookup(ctx, host)
	}
	link := storage.DownloadLink{URL: "https://cdn.example.com/video?secret=private", DirectOnly: true, CacheUntil: time.Now().Add(time.Minute)}
	e := New(resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		resolves.Add(1)
		return link, nil
	}), Options{Policy: policy, Client: client})
	for i := 0; i < 6; i++ {
		req := httptest.NewRequest("GET", "http://gateway/fixed", nil)
		req.Header.Set("User-Agent", "player")
		req.Header.Set("Authorization", "private-account-token")
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", i*10000))
		res := httptest.NewRecorder()
		e.Serve(res, req, storage.FileRef{})
		if res.Code != 302 || res.Header().Get("X-Media-Direct-Verified") != "true" {
			t.Fatal("redirect failed", res.Code)
		}
	}
	if probes.Load() != 1 || resolves.Load() != 6 || lookups.Load() != 6 {
		t.Fatal("cache bypassed per-request checks", probes.Load(), resolves.Load(), lookups.Load())
	}
}

func TestDirectCheckPartitionsURLAgentAndIPAndDoesNotExtendDeadline(t *testing.T) {
	var calls atomic.Int32
	client, p := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); successfulSample(w) })
	e := New(nil, Options{Policy: p, Client: client})
	link := storage.DownloadLink{URL: "https://cdn.example.com/a?signature=1", CacheUntil: time.Now().Add(time.Minute)}
	ctx := context.Background()
	if !e.checkDirect(ctx, link, "player", "203.0.113.1") {
		t.Fatal("initial probe")
	}
	key := directVerificationKey(link, "player", "203.0.113.1")
	e.directChecks.mu.Lock()
	original := e.directChecks.entries[key]
	e.directChecks.mu.Unlock()
	for i := 0; i < 3; i++ {
		if !e.checkDirect(ctx, link, "player", "203.0.113.1") {
			t.Fatal("cached probe")
		}
	}
	e.directChecks.mu.Lock()
	after := e.directChecks.entries[key]
	e.directChecks.mu.Unlock()
	if !after.Equal(original) || calls.Load() != 1 {
		t.Fatal("sliding cache or duplicate probe")
	}
	if !e.checkDirect(ctx, link, "other-player", "203.0.113.1") || !e.checkDirect(ctx, link, "player", "203.0.113.2") {
		t.Fatal("partition probe")
	}
	changed := link
	changed.URL = "https://cdn.example.com/a?signature=2"
	if !e.checkDirect(ctx, changed, "player", "203.0.113.1") || calls.Load() != 4 {
		t.Fatal("URL/UA/IP partition missing")
	}
	e.directChecks.mu.Lock()
	e.directChecks.entries[key] = time.Now().Add(-time.Second)
	e.directChecks.mu.Unlock()
	if !e.checkDirect(ctx, link, "player", "203.0.113.1") || calls.Load() != 5 {
		t.Fatal("expired cache reused")
	}
}

func TestDirectVerificationDeadlinesAndUnknownLifetime(t *testing.T) {
	now := time.Unix(1000, 0)
	for _, tc := range []struct {
		name string
		link storage.DownloadLink
		want time.Duration
	}{
		{"unknown", storage.DownloadLink{}, 0},
		{"cache", storage.DownloadLink{CacheUntil: now.Add(time.Minute)}, 20 * time.Second},
		{"expiry", storage.DownloadLink{ExpiresAt: now.Add(12 * time.Second)}, 7 * time.Second},
		{"both", storage.DownloadLink{CacheUntil: now.Add(9 * time.Second), ExpiresAt: now.Add(time.Minute)}, 4 * time.Second},
		{"expired", storage.DownloadLink{CacheUntil: now.Add(2 * time.Second)}, -3 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := directVerificationDeadline(tc.link, now).Sub(now); got != tc.want {
				t.Fatal(got, tc.want)
			}
		})
	}
	var calls atomic.Int32
	client, p := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); successfulSample(w) })
	e := New(nil, Options{Policy: p, Client: client})
	link := storage.DownloadLink{URL: "https://cdn.example.com/a"}
	for i := 0; i < 2; i++ {
		if !e.checkDirect(context.Background(), link, "player", "") {
			t.Fatal("uncached check")
		}
	}
	if calls.Load() != 2 || len(e.directChecks.entries) != 0 {
		t.Fatal("unknown link lifetime cached")
	}
}

func TestDirectCheckFailuresAreNotCached(t *testing.T) {
	var calls atomic.Int32
	client, p := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(403)
			return
		}
		successfulSample(w)
	})
	e := New(nil, Options{Policy: p, Client: client})
	link := storage.DownloadLink{URL: "https://cdn.example.com/a", CacheUntil: time.Now().Add(time.Minute)}
	if e.checkDirect(context.Background(), link, "player", "") {
		t.Fatal("403 accepted")
	}
	if len(e.directChecks.entries) != 0 {
		t.Fatal("failure cached")
	}
	if !e.checkDirect(context.Background(), link, "player", "") || calls.Load() != 2 {
		t.Fatal("failure blocked fresh recheck")
	}
}

func TestDirectCheckSharedProbeSurvivesCallerCancellation(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	client, p := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		successfulSample(w)
	})
	e := New(nil, Options{Policy: p, Client: client})
	link := storage.DownloadLink{URL: "https://cdn.example.com/a", CacheUntil: time.Now().Add(time.Minute)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	leader := make(chan bool, 1)
	go func() { leader <- e.checkDirect(ctx, link, "player", "") }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("probe not started")
	}
	var wg sync.WaitGroup
	results := make(chan bool, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- e.checkDirect(context.Background(), link, "player", "") }()
	}
	cancel()
	select {
	case ok := <-leader:
		if ok {
			close(release)
			t.Fatal("cancelled caller succeeded")
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("cancelled caller blocked")
	}
	close(release)
	wg.Wait()
	close(results)
	for ok := range results {
		if !ok {
			t.Fatal("other waiter cancelled")
		}
	}
	if calls.Load() != 1 {
		t.Fatal("duplicate concurrent probes", calls.Load())
	}
	if e.checkDirect(ctx, link, "player", "") {
		t.Fatal("cancelled caller used cache")
	}
}

func TestDirectVerificationMemoryBoundAndExpiry(t *testing.T) {
	c := directCheckCache{}
	now := time.Now()
	for i := 0; i < 1000; i++ {
		key := directVerificationKey(storage.DownloadLink{URL: fmt.Sprint(i)}, "", "")
		c.remember(key, now.Add(time.Duration(i+1)*time.Millisecond), now)
	}
	if len(c.entries) != maxDirectVerifications {
		t.Fatal("unbounded cache", len(c.entries))
	}
	first := directVerificationKey(storage.DownloadLink{URL: "0"}, "", "")
	if c.contains(first, now) {
		t.Fatal("oldest entry retained")
	}
	last := directVerificationKey(storage.DownloadLink{URL: "999"}, "", "")
	if !c.contains(last, now) || c.contains(last, now.Add(time.Second)) {
		t.Fatal("expiry not enforced")
	}
	c.remember(first, now.Add(2*time.Second), now.Add(time.Second))
	if len(c.entries) != 1 {
		t.Fatal("expired entries not pruned")
	}
}

func TestDirectCheckOutstandingCDNRequestsBounded(t *testing.T) {
	var active, maximum atomic.Int32
	ready, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	client, p := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := maximum.Load(); n > old; old = maximum.Load() {
			if maximum.CompareAndSwap(old, n) {
				break
			}
		}
		if n == 4 {
			once.Do(func() { close(ready) })
		}
		<-release
		successfulSample(w)
	})
	e := New(nil, Options{Policy: p, Client: client})
	var wg sync.WaitGroup
	results := make(chan bool, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results <- e.checkDirect(context.Background(), storage.DownloadLink{URL: fmt.Sprintf("https://cdn.example.com/%d", i), CacheUntil: time.Now().Add(time.Minute)}, "player", "")
		}(i)
	}
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		close(release)
		wg.Wait()
		t.Fatal("slots not used")
	}
	if maximum.Load() > 4 {
		close(release)
		wg.Wait()
		t.Fatal("unbounded network probes")
	}
	close(release)
	wg.Wait()
	close(results)
	for ok := range results {
		if !ok {
			t.Fatal("probe failed")
		}
	}
	if maximum.Load() != 4 {
		t.Fatal("concurrency cap", maximum.Load())
	}
}

func TestDirectCacheCannotBypassChangedPolicyOrNearExpiry(t *testing.T) {
	var probes atomic.Int32
	var private atomic.Bool
	client, p := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) { probes.Add(1); successfulSample(w) })
	p.Lookup = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if private.Load() {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		}
		return publicLookup(ctx, host)
	}
	var expiring atomic.Bool
	e := New(resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		link := storage.DownloadLink{URL: "https://cdn.example.com/a", CacheUntil: time.Now().Add(time.Minute), DirectOnly: true}
		if expiring.Load() {
			link.CacheUntil = time.Now().Add(2 * time.Second)
		}
		return link, nil
	}), Options{Policy: p, Client: client})
	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "http://gateway/fixed", nil)
		res := httptest.NewRecorder()
		e.Serve(res, req, storage.FileRef{})
		return res
	}
	if request().Code != 302 {
		t.Fatal("initial redirect failed")
	}
	private.Store(true)
	blocked := request()
	if blocked.Code == 302 || blocked.Header().Get("Location") != "" {
		t.Fatal("cached sample bypassed private DNS")
	}
	private.Store(false)
	expiring.Store(true)
	expired := request()
	if expired.Code == 302 || expired.Header().Get("Location") != "" || probes.Load() != 1 {
		t.Fatal("cached sample bypassed lifetime check")
	}
}

func TestInvalidSamplesCannotEnterPositiveCache(t *testing.T) {
	for _, tc := range []struct{ name, contentType, contentRange, body string }{
		{"html", "text/html", "bytes 0-15/16", "0123456789abcdef"},
		{"bad_mime", "video/mp4;broken", "bytes 0-15/16", "0123456789abcdef"},
		{"bad_range", "video/mp4", "bytes 5-20/50", "0123456789abcdef"},
		{"overflow", "video/mp4", "bytes 0-15/999999999999999999999999999999999", "0123456789abcdef"},
		{"truncated", "video/mp4", "bytes 0-15/16", "short"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			client, p := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", tc.contentType)
				w.Header().Set("Content-Range", tc.contentRange)
				w.WriteHeader(206)
				io.WriteString(w, tc.body)
			})
			e := New(nil, Options{Policy: p, Client: client})
			link := storage.DownloadLink{URL: "https://cdn.example.com/a", CacheUntil: time.Now().Add(time.Minute)}
			for i := 0; i < 2; i++ {
				if e.checkDirect(context.Background(), link, "player", "") {
					t.Fatal("invalid sample accepted")
				}
			}
			if calls.Load() != 2 || len(e.directChecks.entries) != 0 {
				t.Fatal("invalid sample cached")
			}
		})
	}
}

func TestServeRedirectCannotSilentlyRelayNonOfficialSources(t *testing.T) {
	for _, force := range []bool{false, true} {
		var calls atomic.Int32
		client, p := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); successfulSample(w) })
		e := New(resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
			return storage.DownloadLink{URL: "https://cdn.example.com/a", UserAgent: "required-other-agent", Headers: map[string]string{"Cookie": "SECRET"}}, nil
		}), Options{Policy: p, Client: client, ProxyFallback: true, ForceProxy: force})
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "http://gateway/fixed", nil)
		r.Header.Set("User-Agent", "player")
		e.ServeRedirect(w, r, storage.FileRef{})
		if w.Code != 409 || calls.Load() != 0 || w.Header().Get("Location") != "" || w.Header().Get("X-Media-Delivery") == "proxy" {
			t.Fatal("strict delivery relayed media")
		}
	}
}
