package httpapi

import (
	"context"
	"media-tool/internal/modules/settings"
	"media-tool/internal/platform/database"
	"net/http/httptest"
	"os"
	"testing"
)

func TestSavedOriginsDoNotTrustForwardedHost(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PG")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal(name)
	}
	repo := settings.Repository{DB: db}
	before, err := repo.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { current, _ := repo.Get(ctx); repo.Save(ctx, before.Value, current.Revision) }()
	value := before.Value
	value.Access = settings.Access{ManagementURL: "https://admin.fixture.invalid", PlaybackURL: "https://play.fixture.invalid", MemberURL: "https://member.fixture.invalid"}
	if _, err = repo.Save(ctx, value, before.Revision); err != nil {
		t.Fatal(err)
	}
	a := API{Config: cfg(), Deps: Dependencies{DB: db}}
	a.Auth.Origin = a.Config.PublicURL
	for _, test := range []struct {
		path, origin string
		allowed      bool
	}{{"/api/v1/auth/login", "https://admin.fixture.invalid", true}, {"/api/v1/auth/login", "https://member.fixture.invalid", false}, {"/api/v1/auth/login", "https://evil.fixture.invalid", false}} {
		r := httptest.NewRequest("POST", test.path, nil)
		r.Header.Set("X-Requested-With", "media-tool")
		r.Header.Set("Origin", test.origin)
		r.Header.Set("X-Forwarded-Host", "evil.fixture.invalid")
		r.Host = "evil.fixture.invalid"
		if a.sameOrigin(r) != test.allowed {
			t.Fatal(test)
		}
	}
	if a.builder(ctx).BaseURL != "https://play.fixture.invalid" {
		t.Fatal("new STRM domain not used")
	}
}
