package embyapi

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func mustWrite(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := io.WriteString(w, body); err != nil {
		t.Error(err)
	}
}
func testRouter(t *testing.T, log io.Writer) *Router {
	t.Helper()
	if log == nil {
		log = io.Discard
	}
	router, err := NewRouter([]Route{
		{"GET", "/Users/{userId}/Items/{itemId}", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" && r.Method != "HEAD" {
				t.Error("HEAD did not select the GET route")
			}
			if info, ok := RequestInfo(r.Context()); !ok || info.User != nil {
				t.Error("request context missing or already authenticated")
			}
			w.Header().Set("Content-Type", "application/json")
			mustWrite(t, w, r.PathValue("userId")+":"+r.PathValue("itemId"))
		}},
		{"GET", "/Users/{id}", func(w http.ResponseWriter, r *http.Request) { mustWrite(t, w, "user:"+r.PathValue("id")) }},
		{"POST", "/Items/{itemId}", func(w http.ResponseWriter, r *http.Request) { mustWrite(t, w, "item:"+r.PathValue("itemId")) }},
		{"POST", "/Users/Public", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(202) }},
	}, Options{Logger: slog.New(slog.NewTextHandler(log, nil))})
	if err != nil {
		t.Fatal(err)
	}
	return router
}
func TestRouterPrefixesCaseAndRepeatedSlashes(t *testing.T) {
	for _, prefix := range []string{"", "/emby", "/mediabrowser", "/EMBY", "/MediaBrowser"} {
		for _, path := range []string{"/Users/AbC", "//uSeRs///AbC/"} {
			t.Run(prefix+path, func(t *testing.T) {
				router := testRouter(t, nil)
				request := httptest.NewRequest("GET", prefix+path, nil)
				before := request.URL.Path
				w := httptest.NewRecorder()
				router.ServeHTTP(w, request)
				if w.Code != 200 || w.Body.String() != "user:AbC" || request.URL.Path != before {
					t.Fatalf("status=%d body=%q original=%s", w.Code, w.Body.String(), request.URL.Path)
				}
			})
		}
	}
	w := httptest.NewRecorder()
	testRouter(t, nil).ServeHTTP(w, httptest.NewRequest("POST", "//mediabrowser//Items/item-7", nil))
	if w.Code != 200 || w.Body.String() != "item:item-7" {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestRouterHeadUsesGetWithoutAnyResponseBody(t *testing.T) {
	for _, path := range []string{"/emby/Users/U/Items/I", "/Users/U", "/Items/42", "/missing"} {
		w := httptest.NewRecorder()
		testRouter(t, nil).ServeHTTP(w, httptest.NewRequest("HEAD", path, nil))
		if w.Body.Len() != 0 {
			t.Fatalf("%s returned body %q", path, w.Body.String())
		}
		if path == "/emby/Users/U/Items/I" && (w.Code != 200 || w.Header().Get("Content-Type") != "application/json") {
			t.Fatalf("GET headers lost: %d %v", w.Code, w.Header())
		}
	}
}
func TestRouterNotFoundMethodOptionsAndPrivateLogs(t *testing.T) {
	var log bytes.Buffer
	router := testRouter(t, &log)
	w := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/unknown?api_key=PRIVATE-TOKEN&path=PRIVATE-PATH", nil)
	request.Header.Set("User-Agent", "Owned/1")
	router.ServeHTTP(w, request)
	if w.Code != 404 || !strings.Contains(log.String(), "/unknown") || !strings.Contains(log.String(), "Owned/1") || strings.Contains(log.String(), "PRIVATE-") {
		t.Fatalf("404=%d log=%q", w.Code, log.String())
	}
	for _, method := range []string{"GET", "DELETE", "TRACE"} {
		w = httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, "/Items/42", nil))
		if w.Code != 405 || w.Header().Get("Allow") != "OPTIONS, POST" {
			t.Fatalf("405: %d %v", w.Code, w.Header())
		}
	}
	w = httptest.NewRecorder()
	request = httptest.NewRequest("OPTIONS", "/emby/Users/42", nil)
	request.Header.Set("Origin", "https://untrusted.invalid")
	request.Header.Set("Access-Control-Request-Headers", "X-Evil")
	router.ServeHTTP(w, request)
	if w.Code != 204 || w.Body.Len() != 0 || w.Header().Get("Allow") != "GET, HEAD, OPTIONS" || w.Header().Get("Access-Control-Allow-Origin") != "" || w.Header().Get("Access-Control-Allow-Credentials") != "" || strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Evil") {
		t.Fatal(w.Code, w.Header(), w.Body.String())
	}
	// Static paths win over a dynamic route, even if only the latter has GET.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/Users/Public", nil))
	if w.Code != 405 || w.Header().Get("Allow") != "OPTIONS, POST" {
		t.Fatalf("dynamic method bypass: %d %v", w.Code, w.Header())
	}
}
func TestRouterRejectsMalformedPathsAndAuthorizationBeforeCallingHandler(t *testing.T) {
	for _, path := range []string{"/Users/../secret", "/Users/%2e%2e/secret", "/Users/A%2FB", "/Users/A%5cB", "/Users/" + strings.Repeat("a", 4097)} {
		w := httptest.NewRecorder()
		testRouter(t, nil).ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 400 {
			t.Fatalf("invalid path: status=%d", w.Code)
		}
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/Users/1?ApiKey=first", nil)
	r.Header.Set("X-Emby-Token", "second")
	testRouter(t, nil).ServeHTTP(w, r)
	if w.Code != 400 || strings.Contains(w.Body.String(), "first") || strings.Contains(w.Body.String(), "second") {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestRouterValidatesRegistrationAndHandlesConcurrency(t *testing.T) {
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	for _, routes := range [][]Route{
		{{"GET", "/Users/{id}", handler}, {"GET", "/users/{userId}", handler}},
		{{"TRACE", "/Users", handler}}, {{"GET", "/Users", nil}},
		{{"GET", "/Users/{id}/Items/{id}", handler}}, {{"GET", "/Users/{", handler}},
		{{"GET", "/Users/../Items", handler}}, {{"GET", "/Users/{bad-name}", handler}},
	} {
		if _, err := NewRouter(routes, Options{}); err == nil {
			t.Fatalf("accepted invalid route %v", routes)
		}
	}
	if _, err := NewRouter(nil, Options{TrustedProxies: []string{"0.0.0.0/0"}}); err == nil {
		t.Fatal("accepted unrestricted proxy")
	}
	router := testRouter(t, nil)
	var wait sync.WaitGroup
	for i := 0; i < 32; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for j := 0; j < 10; j++ {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest("GET", "/mediabrowser/Users/Test", nil))
				if w.Code != 200 {
					t.Error(w.Code)
				}
			}
		}()
	}
	wait.Wait()
}

func TestRouterHeadPreservesObservedMethodAndExplicitHeadRoute(t *testing.T) {
	seen := ""
	handler := func(w http.ResponseWriter, r *http.Request) { seen = r.Method; mustWrite(t, w, "omitted") }
	router, err := NewRouter([]Route{{"GET", "/fallback", handler}, {"GET", "/explicit", handler}, {"HEAD", "/explicit", func(w http.ResponseWriter, r *http.Request) {
		seen = r.Method
		w.WriteHeader(202)
		mustWrite(t, w, "omitted")
	}}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, wantMethod string
		status           int
	}{{"/fallback", "GET", 200}, {"/explicit", "HEAD", 202}} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("HEAD", tc.path, nil))
		if seen != tc.wantMethod || w.Code != tc.status || w.Body.Len() != 0 {
			t.Fatalf("HEAD semantics: method=%s status=%d body=%q", seen, w.Code, w.Body.String())
		}
	}
}

func TestRouterSpecificityKeepsStaticLatestResumeCountsAndSimilarRoutes(t *testing.T) {
	routes := []Route{
		{"GET", "/Users/{userId}/Items/{itemId}", func(w http.ResponseWriter, r *http.Request) { mustWrite(t, w, "item:"+r.PathValue("itemId")) }},
		{"GET", "/Users/{userId}/Items/Resume", func(w http.ResponseWriter, r *http.Request) { mustWrite(t, w, "resume") }},
		{"GET", "/Users/{userId}/Items/Latest", func(w http.ResponseWriter, r *http.Request) { mustWrite(t, w, "user-latest") }},
		{"GET", "/Items/{id}", func(w http.ResponseWriter, r *http.Request) { mustWrite(t, w, "item:"+r.PathValue("id")) }},
		{"GET", "/Items/Latest", func(w http.ResponseWriter, r *http.Request) { mustWrite(t, w, "latest") }},
		{"GET", "/Items/Counts", func(w http.ResponseWriter, r *http.Request) { mustWrite(t, w, "counts") }},
		{"GET", "/Items/{id}/Similar", func(w http.ResponseWriter, r *http.Request) { mustWrite(t, w, "similar") }},
	}
	router, err := NewRouter(routes, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, want string }{{"/Users/u/Items/Resume", "resume"}, {"/Users/u/Items/Latest", "user-latest"}, {"/Items/Latest", "latest"}, {"/Items/Counts", "counts"}, {"/Items/77/Similar", "similar"}} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != 200 || w.Body.String() != tc.want {
			t.Fatalf("%s: %d %q", tc.path, w.Code, w.Body.String())
		}
	}
}
