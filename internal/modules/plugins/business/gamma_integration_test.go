package business

import (
	"encoding/json"
	"errors"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"io"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/sites"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestWishlistHostProjectionAndEnsurePermanentIdentityDedup(t *testing.T) {
	h, ctx := mediaFixture(t)
	h.Connections.OutboundHTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"id":1,"title":"Synthetic","release_date":"2026-01-01"}`
		if strings.Contains(r.URL.Path, "/list/") {
			body = `{"items":[{"id":1,"media_type":"movie","title":"Synthetic","release_date":"2026-01-01","poster_path":"SENTINEL_URL","token":"SENTINEL_SECRET"}],"item_count":1}`
		}
		if r.Header.Get("Authorization") != "Bearer SENTINEL_SECRET" {
			t.Error("host token missing")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Set-Cookie": {"SENTINEL_COOKIE"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	if _, e := h.Connections.Repository.Save(ctx, "", integrations.Input{Kind: "tmdb", Name: "synthetic", Enabled: true, Secrets: map[string]string{"token": "SENTINEL_SECRET"}}); e != nil {
		t.Fatal(e)
	}
	page := requireMedia[b.WishlistPage](t, h, "metadata.wishlist", b.WishlistRequest{Source: "tmdb-list", ID: "42", Page: 1})
	raw, _ := json.Marshal(page)
	if page.State != "ok" || len(page.Items) != 1 || strings.Contains(string(raw), "SENTINEL") || strings.Contains(string(raw), "http") {
		t.Fatal(page, string(raw))
	}
	if e := (sites.Repository{DB: h.DB}).LoadGoTemplates(ctx, []string{"hdsky"}); e != nil {
		t.Fatal(e)
	}
	account, e := h.Connections.Repository.Save(ctx, "", integrations.Input{Kind: "ptsite", Name: "synthetic", Enabled: true, Settings: map[string]any{"definitionId": "hdsky", "endpoint": "https://synthetic.invalid", "userId": "1"}, Secrets: map[string]string{"cookie": "SYNTHETIC_COOKIE"}})
	if e != nil {
		t.Fatal(e)
	}
	in := b.SubscriptionInput{MediaType: "movie", TMDBID: 1, Source: "search", Enabled: false, Rules: b.Rules{AccountIDs: []string{account.ID}}}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := mediaCall(t, h, "subscriptions.ensure", in)
			if e == nil && v.(b.Receipt).State != "ensured" {
				e = errors.New("bad ensure state")
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var count int
	if e = h.DB.QueryRow(ctx, `SELECT count(*) FROM subscriptions`).Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
	// Business ownership is independent of the expiring Host receipt. A ready
	// imported asset prevents a new subscription even under a different operation.
	if _, e = h.DB.Exec(ctx, `DELETE FROM subscriptions`); e != nil {
		t.Fatal(e)
	}
	media, asset, file, store := jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO storage_connections(id,provider,name)VALUES($1,'clouddrive2','synthetic')`, []any{store}},
		{`INSERT INTO cloud_files(id,storage_id,full_path,size_bytes,state)VALUES($1,$2,'/synthetic/file.mkv',4,'present')`, []any{file, store}},
		{`INSERT INTO media_items(id,tmdb_id,media_type,title,metadata)VALUES($1,1,'movie','Synthetic','{}')`, []any{media}},
		{`INSERT INTO media_assets(id,cloud_file_id,artifacts_state)VALUES($1,$2,'ready')`, []any{asset, file}},
		{`INSERT INTO media_asset_bindings(asset_id,media_id)VALUES($1,$2)`, []any{asset, media}},
	} {
		if _, e = h.DB.Exec(ctx, q.sql, q.args...); e != nil {
			t.Fatal(e)
		}
	}
	receipt := requireMedia[b.Receipt](t, h, "subscriptions.ensure", in)
	if receipt.State != "owned" {
		t.Fatal(receipt)
	}
	h.Connections.OutboundHTTP = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":999,"title":"Wrong identity"}`))}, nil
	})}
	in.TMDBID = 2
	if _, e = mediaCall(t, h, "subscriptions.ensure", in); e == nil {
		t.Fatal("provider identity substitution accepted")
	}

}
