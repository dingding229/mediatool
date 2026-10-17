package linksource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"media-tool/internal/adapters/open115"
	"media-tool/internal/modules/playbackgrants"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/requestbudget"
)

const asset = "12345678-1234-4234-8234-123456789abc"
const privateURL = "https://synthetic.cdn.example/file?SIGN=NEVER_LOG_LINK"
const probeUA = "SyntheticProbe/1.0"

type repositoryFunc func(context.Context, string) (Evidence, error)

func (f repositoryFunc) Load(ctx context.Context, id string) (Evidence, error) { return f(ctx, id) }

type readerFunc func(context.Context, string) ([]byte, error)

func (f readerFunc) Read(ctx context.Context, id string) ([]byte, error) { return f(ctx, id) }

type parserFunc func(context.Context, string) (storage.FileRef, error)

func (f parserFunc) Parse(ctx context.Context, s string) (storage.FileRef, error) { return f(ctx, s) }

type resolverFunc func(context.Context, storage.FileRef) (storage.DownloadLink, error)

func (f resolverFunc) Resolve(ctx context.Context, r storage.FileRef) (storage.DownloadLink, error) {
	return f(ctx, r)
}

type policyFunc func(context.Context, string) error

func (f policyFunc) Check(ctx context.Context, s string) error { return f(ctx, s) }

type fixture struct {
	s     *Source
	e     Evidence
	body  []byte
	log   bytes.Buffer
	calls int
	ref   storage.FileRef
}

func setup(t *testing.T) *fixture {
	t.Helper()
	admission.Lock()
	admission.active = 0
	admission.limit = 1
	admission.after = time.Time{}
	admission.Unlock()
	f := &fixture{}
	f.body = []byte("https://resolver.example/api/v1/cloudStorage/strm302?assetId=" + asset + "&path=%2F115%2Fsynthetic.mkv&apiKey=NEVER_LOG_GRANT\n")
	sum := sha256.Sum256(f.body)
	f.e = Evidence{AssetID: asset, Path: "/115/synthetic.mkv", PickCode: "syntheticpickcode", Output: "电影/Synthetic.strm", Size: 4096, Revision: 3, Digest: sum[:]}
	d := Dependencies{
		Repository: repositoryFunc(func(ctx context.Context, id string) (Evidence, error) {
			if id != asset {
				t.Error("wrong asset")
			}
			return f.e, nil
		}),
		Reader: readerFunc(func(context.Context, string) ([]byte, error) { return f.body, nil }),
		Parser: parserFunc(func(context.Context, string) (storage.FileRef, error) {
			return storage.FileRef{Path: f.e.Path, AssetID: asset}, nil
		}),
		Resolver: resolverFunc(func(ctx context.Context, ref storage.FileRef) (storage.DownloadLink, error) {
			f.calls++
			f.ref = ref
			if !trustedProbe(ctx) {
				t.Error("missing capability")
			}
			return storage.DownloadLink{URL: privateURL, UserAgent: ref.UserAgent}, nil
		}),
		Policy: policyFunc(func(context.Context, string) error { return nil }),
	}
	var err error
	f.s, err = New(d, Config{UserAgent: probeUA}, slog.New(slog.NewJSONHandler(&f.log, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func assertFailure(t *testing.T, err error, class Class, reason Reason) {
	t.Helper()
	var f *Failure
	if !errors.As(err, &f) || f.Class != class || f.Reason != reason {
		t.Fatalf("want %s/%s got %v", class, reason, err)
	}
	if class == Retryable && f.RetryAfter <= 0 {
		t.Fatal("retry delay missing")
	}
}
func noSecrets(t *testing.T, value string) {
	t.Helper()
	for _, s := range []string{privateURL, "NEVER_LOG_LINK", "NEVER_LOG_GRANT", "/115/", "syntheticpickcode", probeUA} {
		if strings.Contains(value, s) {
			t.Fatal("secret leaked")
		}
	}
}
func TestFetchUAAndTicketRedaction(t *testing.T) {
	f := setup(t)
	ticket, err := f.s.Fetch(context.Background(), asset)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []any{ticket, *ticket} {
		noSecrets(t, fmt.Sprintf("%v %+v %#v", v, v, v))
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != "{}" {
			t.Fatal("ticket JSON contract")
		}
	}
	link, ua, err := ticket.Claim()
	if err != nil || link != privateURL || ua != probeUA || f.ref.UserAgent != ua || f.ref.AssetID != asset || f.ref.ExpectedSize != 4096 || f.ref.PickCode != f.e.PickCode {
		t.Fatal("link/UA identity lost")
	}
	_, _, err = ticket.Claim()
	assertFailure(t, err, Permanent, Used)
	noSecrets(t, f.log.String())
	var empty Ticket
	_, _, err = empty.Claim()
	assertFailure(t, err, Permanent, Used)
	var absent *Ticket
	_, _, err = absent.Claim()
	assertFailure(t, err, Permanent, Used)
	expired := &Ticket{state: &ticketState{url: privateURL, ua: probeUA, expires: time.Now().Add(-time.Second)}}
	_, _, err = expired.Claim()
	assertFailure(t, err, Permanent, Used)
}
func TestFetchRejectionsAndNoUpstreamForInvalidEvidence(t *testing.T) {
	tests := []struct {
		name   string
		change func(*fixture)
		class  Class
		reason Reason
	}{
		{"not exposable", func(f *fixture) {
			f.s.deps.Repository = repositoryFunc(func(context.Context, string) (Evidence, error) { return Evidence{}, fail(Permanent, NotExposable, 0) })
		}, Permanent, NotExposable},
		{"file missing", func(f *fixture) {
			f.s.deps.Reader = readerFunc(func(context.Context, string) ([]byte, error) { return nil, fail(Permanent, FileMissing, 0) })
		}, Permanent, FileMissing},
		{"wrong asset", func(f *fixture) { f.e.AssetID = "00000000-0000-4000-8000-000000000000" }, Manual, Mismatch},
		{"missing digest", func(f *fixture) { f.e.Digest = nil }, Manual, Mismatch},
		{"modified STRM", func(f *fixture) { f.body = append(f.body, []byte("changed")...) }, Manual, Mismatch},
		{"db path mismatch", func(f *fixture) { f.e.Path = "/115/other.mkv" }, Manual, Mismatch},
		{"parser path mismatch", func(f *fixture) {
			f.s.deps.Parser = parserFunc(func(context.Context, string) (storage.FileRef, error) {
				return storage.FileRef{Path: "/115/other.mkv"}, nil
			})
		}, Manual, Mismatch},
		{"query asset mismatch", func(f *fixture) {
			f.body = []byte(strings.ReplaceAll(string(f.body), asset, "00000000-0000-4000-8000-000000000000"))
			sum := sha256.Sum256(f.body)
			f.e.Digest = sum[:]
		}, Manual, Mismatch},
		{"query pickcode mismatch", func(f *fixture) {
			f.body = []byte(strings.TrimSpace(string(f.body)) + "&pickCode=wrong")
			sum := sha256.Sum256(f.body)
			f.e.Digest = sum[:]
		}, Manual, Mismatch},
		{"revoked grant", func(f *fixture) {
			f.s.deps.Parser = parserFunc(func(context.Context, string) (storage.FileRef, error) {
				return storage.FileRef{}, playbackgrants.ErrDenied
			})
		}, Manual, Authorization},
		{"oversize", func(f *fixture) { f.body = bytes.Repeat([]byte("x"), 8193) }, Manual, Mismatch},
		{"invalid UTF8", func(f *fixture) { f.body = []byte{255} }, Manual, Mismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := setup(t)
			tt.change(f)
			_, err := f.s.Fetch(context.Background(), asset)
			assertFailure(t, err, tt.class, tt.reason)
			if f.calls != 0 {
				t.Fatal("invalid evidence reached upstream")
			}
			noSecrets(t, f.log.String()+err.Error())
		})
	}
}
func TestUpstreamFailuresAreClassifiedAndRedacted(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		class  Class
		reason Reason
	}{
		{"rate sentinel", open115.ErrRateLimited, Retryable, Limited},
		{"429", &open115.APIError{HTTPStatus: 429}, Retryable, Limited},
		{"auth", open115.ErrAuthorization, Manual, Authorization},
		{"invalid access", &open115.APIError{Code: 40140125}, Manual, Authorization},
		{"timeout", context.DeadlineExceeded, Retryable, Timeout},
		{"missing upstream", storage.ErrNotFound, Permanent, NotExposable},
		{"unsafe response", fmt.Errorf("SECRET %s", privateURL), Retryable, Unavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := setup(t)
			f.s.deps.Resolver = resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
				return storage.DownloadLink{}, tt.err
			})
			_, err := f.s.Fetch(context.Background(), asset)
			assertFailure(t, err, tt.class, tt.reason)
			noSecrets(t, f.log.String()+err.Error())
			if tt.reason == Limited {
				_, err = f.s.Fetch(context.Background(), asset)
				assertFailure(t, err, Retryable, Limited)
			}
		})
	}
}
func TestPostResolveSafetyAndRecheck(t *testing.T) {
	for _, name := range []string{"wrong UA", "extra header", "self callback", "unsafe target", "changed evidence", "changed body", "expired link"} {
		t.Run(name, func(t *testing.T) {
			f := setup(t)
			reason := Mismatch
			switch name {
			case "wrong UA":
				reason = UAMismatch
				f.s.deps.Resolver = resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
					return storage.DownloadLink{URL: privateURL, UserAgent: "WrongPlayer"}, nil
				})
			case "extra header":
				reason = UAMismatch
				f.s.deps.Resolver = resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
					return storage.DownloadLink{URL: privateURL, Headers: map[string]string{"Cookie": "SECRET"}}, nil
				})
			case "self callback":
				reason = UnsafeTarget
				f.s.deps.Resolver = resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
					return storage.DownloadLink{URL: "https://resolver.example/api/v1/cloudStorage/strm302?apiKey=NEVER_LOG_GRANT"}, nil
				})
			case "unsafe target":
				reason = UnsafeTarget
				f.s.deps.Policy = policyFunc(func(context.Context, string) error { return errors.New(privateURL) })
			case "changed evidence":
				count := 0
				f.s.deps.Repository = repositoryFunc(func(context.Context, string) (Evidence, error) {
					count++
					e := f.e
					if count > 1 {
						e.Revision++
					}
					return e, nil
				})
			case "changed body":
				count := 0
				f.s.deps.Reader = readerFunc(func(context.Context, string) ([]byte, error) {
					count++
					if count > 1 {
						return []byte("changed"), nil
					}
					return f.body, nil
				})
			case "expired link":
				reason = Unavailable
				f.s.deps.Resolver = resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
					return storage.DownloadLink{URL: privateURL, ExpiresAt: time.Now().Add(-time.Second)}, nil
				})
			}
			_, err := f.s.Fetch(context.Background(), asset)
			class := Manual
			if name == "expired link" {
				class = Retryable
			}
			assertFailure(t, err, class, reason)
			noSecrets(t, f.log.String()+err.Error())
		})
	}
}
func TestProbeBudgetAndGlobalAdmission(t *testing.T) {
	f := setup(t)
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	f.s.deps.Resolver = resolverFunc(func(ctx context.Context, ref storage.FileRef) (storage.DownloadLink, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > requestbudget.DefaultResolve {
			t.Error("budget missing")
		}
		close(started)
		<-release
		return storage.DownloadLink{URL: privateURL}, nil
	})
	go func() { _, err := f.s.Fetch(context.Background(), asset); done <- err }()
	<-started
	_, err := f.s.Fetch(context.Background(), asset)
	assertFailure(t, err, Retryable, Limited)
	if ConfigureConcurrency(2) == nil {
		t.Error("changed live semaphore")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := ConfigureConcurrency(0); err == nil {
		t.Fatal("invalid concurrency")
	}
	if err := ConfigureConcurrency(2); err != nil {
		t.Fatal(err)
	}
	f = setup(t)
	f.s.config.ResolveTimeout = 10 * time.Millisecond
	f.s.deps.Resolver = resolverFunc(func(ctx context.Context, _ storage.FileRef) (storage.DownloadLink, error) {
		<-ctx.Done()
		return storage.DownloadLink{}, ctx.Err()
	})
	_, err = f.s.Fetch(context.Background(), asset)
	assertFailure(t, err, Retryable, Timeout)
}
func TestTrustedCapabilityCannotBeSuppliedByCaller(t *testing.T) {
	calls := 0
	r := trustedResolver{resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		calls++
		return storage.DownloadLink{}, nil
	})}
	_, err := r.Resolve(context.Background(), storage.FileRef{UserAgent: DefaultUserAgent})
	assertFailure(t, err, Manual, Authorization)
	if calls != 0 {
		t.Fatal("untrusted call reached storage")
	}
}
func TestHTTPTraceOnlyLogsSafeFieldsAndOverridesInheritedObserver(t *testing.T) {
	f := setup(t)
	inherited := false
	f.s.deps.Resolver = resolverFunc(func(ctx context.Context, ref storage.FileRef) (storage.DownloadLink, error) {
		storage.ObserveResolveHTTP(ctx, storage.StageOfficialHTTPDownload, time.Now(), &url.Error{Op: "GET", URL: privateURL, Err: errors.New("failed")}, &storage.HTTPTraceTimings{Attempt: 1, DNSMillis: 1, TCPMillis: 2, TLSMillis: 3, FirstByteMillis: 4, TotalMillis: 5})
		return storage.DownloadLink{URL: privateURL}, nil
	})
	ctx := storage.WithResolveObserver(context.Background(), func(storage.ResolveObservation) { inherited = true })
	if _, err := f.s.Fetch(ctx, asset); err != nil {
		t.Fatal(err)
	}
	if inherited {
		t.Fatal("inherited unsafe observer invoked")
	}
	noSecrets(t, f.log.String())
	for _, key := range []string{"dnsMs", "tcpMs", "tlsMs", "firstByteMs", "httpTotalMs", "official_http_download"} {
		if !strings.Contains(f.log.String(), key) {
			t.Fatal("missing timing", key)
		}
	}
}
func TestTicketConcurrentClaimOnce(t *testing.T) {
	f := setup(t)
	ticket, err := f.s.Fetch(context.Background(), asset)
	if err != nil {
		t.Fatal(err)
	}
	var claims atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			link, ua, err := ticket.Claim()
			if err == nil {
				if link != privateURL || ua != probeUA {
					t.Error("claim mismatch")
				}
				claims.Add(1)
			}
		})
	}
	wg.Wait()
	if claims.Load() != 1 {
		t.Fatal("ticket reused")
	}
}
func TestConfigAndInvalidIDs(t *testing.T) {
	f := setup(t)
	for _, c := range []Config{{UserAgent: " padded "}, {UserAgent: "x\r\nX:"}, {UserAgent: strings.Repeat("x", 513)}, {ResolveTimeout: -1}, {ResolveTimeout: time.Hour}} {
		if _, err := New(f.s.deps, c, nil); err == nil {
			t.Fatal("invalid config accepted")
		}
	}
	if _, err := New(Dependencies{}, Config{}, nil); err == nil {
		t.Fatal("missing dependencies")
	}
	s, err := New(f.s.deps, Config{}, nil)
	if err != nil || s.config.UserAgent != DefaultUserAgent {
		t.Fatal("defaults")
	}
	for _, id := range []string{"", "bad-id", strings.Repeat("x", 36)} {
		_, err := s.Fetch(context.Background(), id)
		assertFailure(t, err, Permanent, NotExposable)
	}
	if _, err := NewForManager(nil, Config{}, nil); err == nil {
		t.Fatal("nil manager")
	}
}

func TestFailureConstantsAreStableAndUnique(t *testing.T) {
	classes := []Class{Permanent, Retryable, Manual}
	if fmt.Sprint(classes) != "[permanent retryable manual]" {
		t.Fatal("class contract changed")
	}
	reasons := []Reason{NotExposable, FileMissing, Mismatch, Authorization, Limited, Timeout, Unavailable, UnsafeTarget, UAMismatch, Used}
	if fmt.Sprint(reasons) != "[not_exposable file_missing strm_mismatch authorization rate_limited timeout unavailable unsafe_target ua_mismatch ticket_unavailable]" {
		t.Fatal("reason contract changed")
	}
	seen := map[Reason]bool{}
	for _, reason := range reasons {
		if seen[reason] {
			t.Fatal("duplicate reason")
		}
		seen[reason] = true
	}
}
func TestGlobalConcurrencyConfiguredAcrossSources(t *testing.T) {
	f := setup(t)
	if err := ConfigureConcurrency(2); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan error, 2)
	defer func() {
		if err := ConfigureConcurrency(1); err != nil {
			t.Error(err)
		}
	}()
	f.s.deps.Resolver = resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		started <- struct{}{}
		<-release
		return storage.DownloadLink{URL: privateURL}, nil
	})
	other, err := New(f.s.deps, f.s.config, f.s.logger)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []*Source{f.s, other} {
		go func() { _, err := source.Fetch(context.Background(), asset); done <- err }()
	}
	<-started
	<-started
	_, err = other.Fetch(context.Background(), asset)
	assertFailure(t, err, Retryable, Limited)
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}
func TestCancelledBudgetAndTicketTTL(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := f.s.Fetch(ctx, asset)
	assertFailure(t, err, Retryable, Timeout)
	expiry := time.Now().Add(5 * time.Second)
	f.s.deps.Resolver = resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		return storage.DownloadLink{URL: privateURL, CacheUntil: expiry, ExpiresAt: expiry.Add(5 * time.Second)}, nil
	})
	ticket, err := f.s.Fetch(context.Background(), asset)
	if err != nil {
		t.Fatal(err)
	}
	if !ticket.state.expires.Equal(expiry) {
		t.Fatal("upstream short expiry ignored")
	}
}
