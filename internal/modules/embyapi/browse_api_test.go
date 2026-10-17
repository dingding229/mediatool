package embyapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
)

var updateBrowseGolden = flag.Bool("update", false, "update synthetic browse HTTP goldens")

type browseStub struct {
	items  []dto.BaseItemDto
	err    error
	seen   nativelibrary.ItemQuery
	before func(context.Context, contracts.User, contracts.PermissionChecker) error
}

func (b *browseStub) run(ctx context.Context, u contracts.User, p contracts.PermissionChecker) error {
	if b.before != nil {
		return b.before(ctx, u, p)
	}
	return b.err
}
func (b *browseStub) Views(ctx context.Context, u contracts.User, p contracts.PermissionChecker) ([]dto.BaseItemDto, error) {
	return b.items, b.run(ctx, u, p)
}
func (b *browseStub) Query(ctx context.Context, u contracts.User, p contracts.PermissionChecker, q nativelibrary.ItemQuery) ([]dto.BaseItemDto, int, error) {
	b.seen = q
	return b.items, len(b.items), b.run(ctx, u, p)
}
func (b *browseStub) Counts(ctx context.Context, u contracts.User, p contracts.PermissionChecker, q ...nativelibrary.ItemQuery) (nativelibrary.ItemCounts, error) {
	return nativelibrary.ItemCounts{MovieCount: len(b.items)}, b.run(ctx, u, p)
}
func browseUnitServer(t *testing.T, stub *browseStub) *Server {
	t.Helper()
	s := authTestServer(t, &fakeAccountService{user: contracts.User{ID: strings.Repeat("a", 32), Enabled: true}}, nil, nil)
	s.options.Browse = stub
	return s
}
func browseUnitCall(s *Server, method, path string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.Header.Set("X-Emby-Authorization", `MediaBrowser Client="Infuse", DeviceId="synthetic-device", Token="valid-token"`)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func TestBrowseHTTPGoldens(t *testing.T) {
	library, err := nativelibrary.LibraryDTO(nativelibrary.Library{ID: "g:synthetic", ItemID: "100000", Name: "Synthetic Library", ServerID: strings.Repeat("a", 32), CollectionType: nativelibrary.CollectionMovies})
	if err != nil {
		t.Fatal(err)
	}
	movie, err := nativelibrary.MovieDTO(nativelibrary.Movie{ID: "100001", ServerID: strings.Repeat("a", 32), ParentID: "100000", Name: "Synthetic Movie", Year: "2025", TMDBID: 900001, CreatedAt: time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC), Metadata: nativelibrary.StoredMovieMetadata{Overview: "Synthetic overview", Genres: []string{"Drama"}}, Sources: []nativelibrary.MovieSource{{ID: "synthetic-source", Container: "mkv"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path string
		item       dto.BaseItemDto
	}{{"views", "/Users/" + strings.Repeat("a", 32) + "/Views", library}, {"items", "/Items?UserId=" + strings.Repeat("a", 32) + "&Fields=Overview,People", movie}, {"detail", "/Items/100001", movie}, {"counts", "/Items/Counts", movie}} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &browseStub{items: []dto.BaseItemDto{tc.item}}
			s := browseUnitServer(t, stub)
			w := browseUnitCall(s, "GET", tc.path)
			if w.Code != 200 || !json.Valid(w.Body.Bytes()) {
				t.Fatal(w.Code, w.Body.String())
			}
			var doc any
			decoder := json.NewDecoder(bytes.NewReader(w.Body.Bytes()))
			decoder.UseNumber()
			if err := decoder.Decode(&doc); err != nil {
				t.Fatal(err)
			}
			checkBrowseHTTPShape(t, doc, "")
			normalized, err := json.MarshalIndent(doc, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			normalized = append(normalized, '\n')
			path := filepath.Join("testdata", "browse_http", tc.name+".json")
			if *updateBrowseGolden {
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, normalized, 0644); err != nil {
					t.Fatal(err)
				}
			}
			expected, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(expected, normalized) {
				t.Fatalf("HTTP golden mismatch: %s", path)
			}
			head := browseUnitCall(s, "HEAD", tc.path)
			if head.Code != 200 || head.Body.Len() != 0 {
				t.Fatal("HEAD body/status")
			}
		})
	}
}
func TestBrowseHTTPAliasesErrorsAndParameters(t *testing.T) {
	stub := &browseStub{}
	s := browseUnitServer(t, stub)
	for _, path := range []string{"/emby//uSeRs/" + strings.Repeat("a", 32) + "/iTeMs?lImIt=2&gEnReS=Drama&yEaRs=2025&SeArChTeRm=Synthetic", "/mediabrowser/Items?LiMiT=2&Genres=Drama&Years=2025&SearchTerm=Synthetic"} {
		w := browseUnitCall(s, "GET", path)
		if w.Code != 200 || stub.seen.Limit != 2 || stub.seen.SearchTerm != "Synthetic" || len(stub.seen.Genres) != 1 || len(stub.seen.Years) != 1 {
			t.Fatal(w.Code, stub.seen)
		}
	}
	for _, path := range []string{"/Items?Limit=201", "/Items?Years=bad", "/Items?EnableImages=bad"} {
		if w := browseUnitCall(s, "GET", path); w.Code != 400 {
			t.Fatal(path, w.Code)
		}
	}
	for _, tc := range []struct {
		err    error
		status int
	}{{nativelibrary.ErrNotFound, 404}, {nativelibrary.ErrInvalidQuery, 400}, {contracts.ErrUnavailable, 503}, {context.DeadlineExceeded, 503}, {errors.New("SQL secret /115/private"), 503}} {
		stub.err = tc.err
		w := browseUnitCall(s, "GET", "/Items?Recursive=true")
		if w.Code != tc.status || strings.Contains(w.Body.String(), "secret") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	stub.err = nil
	w := browseUnitCall(s, "GET", "/Items/100001")
	if w.Code != 404 {
		t.Fatal("empty detail not 404", w.Code)
	}
	for _, path := range []string{"/Library/MediaFolders", "/Users/" + strings.Repeat("a", 32) + "/Views"} {
		if w := browseUnitCall(s, "GET", path); w.Code != 200 {
			t.Fatal(path, w.Code)
		}
	}
}

type slowBrowseAuth struct{ *fakeAccountService }

func (a slowBrowseAuth) Authenticate(ctx context.Context, _ string, _ contracts.DeviceInfo) (contracts.User, contracts.Device, contracts.Token, error) {
	<-ctx.Done()
	return contracts.User{}, contracts.Device{}, contracts.Token{}, contracts.ErrUnavailable
}

type slowBrowsePermissions struct{ contracts.PermissionChecker }

func (p slowBrowsePermissions) VisibleLibraries(ctx context.Context, _ contracts.User) ([]string, error) {
	<-ctx.Done()
	return nil, contracts.ErrUnavailable
}
func TestBrowseBudgetStartsBeforeSettingsAndAuthentication(t *testing.T) {
	for _, stage := range []string{"settings", "authentication", "permissions"} {
		t.Run(stage, func(t *testing.T) {
			stub := &browseStub{}
			s := browseUnitServer(t, stub)
			original := s.options.Settings
			s.options.Settings = nativeSettingsFunc(func(ctx context.Context) (nativelibrary.ServerSettings, error) {
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > BrowseRequestBudget {
					t.Error("missing shared deadline")
				}
				if stage == "settings" {
					<-ctx.Done()
					return nativelibrary.ServerSettings{}, nativelibrary.ErrSettingsUnavailable
				}
				return original.Get(ctx)
			})
			if stage == "authentication" {
				s.options.Auth = slowBrowseAuth{&fakeAccountService{}}
			}
			if stage == "permissions" {
				s.options.Permissions = func() contracts.PermissionChecker { return slowBrowsePermissions{contracts.NewPermissionChecker()} }
				stub.before = func(ctx context.Context, u contracts.User, p contracts.PermissionChecker) error {
					_, err := p.VisibleLibraries(ctx, u)
					return err
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancel()
			r := httptest.NewRequest("GET", "/Items", nil).WithContext(ctx)
			r.Header.Set("X-Emby-Token", "valid-token")
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != 503 || !json.Valid(w.Body.Bytes()) {
				t.Fatal(stage, w.Code, w.Body.String())
			}
		})
	}
	s := browseUnitServer(t, &browseStub{})
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest("GET", "/Items", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	s.browseBudget(w, r, func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte(`{"private":"partial-success"}`)); err != nil {
			t.Fatal(err)
		}
		cancel()
	})
	if w.Code != 503 || strings.Contains(w.Body.String(), "partial-success") {
		t.Fatal("partial success escaped deadline", w.Code, w.Body.String())
	}
	buffer := &browseBuffer{header: make(http.Header)}
	if _, err := buffer.Write(make([]byte, maxBrowseResponse+1)); err == nil || !buffer.overflow {
		t.Fatal("unbounded response buffer")
	}
}

type budgetAuth struct {
	*fakeAccountService
	check func(context.Context)
}

func (a budgetAuth) Authenticate(ctx context.Context, token string, d contracts.DeviceInfo) (contracts.User, contracts.Device, contracts.Token, error) {
	a.check(ctx)
	return a.fakeAccountService.Authenticate(ctx, token, d)
}

type budgetPermissions struct {
	contracts.PermissionChecker
	check func(context.Context)
}

func (p budgetPermissions) VisibleLibraries(ctx context.Context, _ contracts.User) ([]string, error) {
	p.check(ctx)
	return []string{}, nil
}
func TestBrowseDefaultSixSecondDeadlineIsSharedAcrossStages(t *testing.T) {
	stub := &browseStub{}
	s := browseUnitServer(t, stub)
	var deadline time.Time
	stages := 0
	check := func(ctx context.Context) {
		t.Helper()
		d, ok := ctx.Deadline()
		if !ok || !d.Equal(deadline) {
			t.Fatal("stage reset/lost overall deadline")
		}
		stages++
	}
	s.options.Settings = nativeSettingsFunc(func(ctx context.Context) (nativelibrary.ServerSettings, error) {
		d, ok := ctx.Deadline()
		if !ok || time.Until(d) <= 5*time.Second || time.Until(d) > BrowseRequestBudget {
			t.Fatal("missing default six-second budget")
		}
		deadline = d
		stages++
		return settingsFixture(), nil
	})
	s.options.Auth = budgetAuth{&fakeAccountService{user: contracts.User{ID: strings.Repeat("a", 32), Enabled: true}}, check}
	s.options.Permissions = func() contracts.PermissionChecker { return budgetPermissions{contracts.NewPermissionChecker(), check} }
	stub.before = func(ctx context.Context, u contracts.User, p contracts.PermissionChecker) error {
		_, err := p.VisibleLibraries(ctx, u)
		return err
	}
	w := browseUnitCall(s, "GET", "/Items")
	if w.Code != 200 || stages != 3 {
		t.Fatal("budget stages", w.Code, stages)
	}
}

func checkBrowseHTTPShape(t *testing.T, value any, key string) {
	t.Helper()
	switch key {
	case "Id", "ParentId", "SeriesId", "SeasonId", "ServerId":
		if _, ok := value.(string); !ok {
			t.Fatal("HTTP ID is not string", key)
		}
	case "SearchHints", "Items", "Genres", "People", "MediaSources", "MediaStreams", "BackdropImageTags":
		if _, ok := value.([]any); !ok {
			t.Fatal("null/non-array HTTP field", key)
		}
	}
	if strings.HasSuffix(key, "Ticks") {
		n, ok := value.(json.Number)
		if !ok {
			t.Fatal("HTTP ticks not number", key)
		}
		if _, err := strconv.ParseInt(string(n), 10, 64); err != nil {
			t.Fatal("HTTP ticks not int64", err)
		}
	}
	switch v := value.(type) {
	case map[string]any:
		for k, x := range v {
			checkBrowseHTTPShape(t, x, k)
		}
	case []any:
		for _, x := range v {
			checkBrowseHTTPShape(t, x, "")
		}
	}
}
