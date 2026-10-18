package business

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"io"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/sitemetrics"
	"media-tool/internal/modules/sites"
	"media-tool/internal/modules/subscriptions"
	dbutil "media-tool/internal/platform/database"
	"net/http"
	"strings"
	"testing"
	"time"
)

type acquisitionTransport func(*http.Request) (*http.Response, error)

func (f acquisitionTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAcquisitionFactsCommitRollbackRedactionAndOccurrences(t *testing.T) {
	h, ctx := hostFixture(t)
	h.Connections.OutboundHTTP = &http.Client{Transport: acquisitionTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":1,"title":"NEVER_EVENT_SECRET","release_date":"2026-01-01"}`))}, nil
	})}
	repo := h.Connections.Repository
	in := integrations.Input{Kind: "ptsite", Name: "NEVER_EVENT_SECRET", Enabled: true, Settings: map[string]any{"endpoint": "https://owned.invalid", "definitionId": "fixture_f4a"}, Secrets: map[string]string{"cookie": "NEVER_EVENT_SECRET", "rssUrl": "https://owned.invalid/rss?passkey=NEVER_EVENT_SECRET"}}
	account, err := repo.Save(ctx, "", in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Save(ctx, "", integrations.Input{Kind: "tmdb", Name: "fixture", Settings: map[string]any{}, Enabled: true, Secrets: map[string]string{"token": "NEVER_EVENT_SECRET"}}); err != nil {
		t.Fatal(err)
	}
	engine := subscriptions.Engine{DB: h.DB, Connections: h.Connections}
	subIn := subscriptions.Input{MediaType: "movie", TMDBID: 1, Enabled: true, Source: "search", Rules: subscriptions.Rules{AccountIDs: []string{account.ID}, Keyword: "NEVER_EVENT_SECRET"}}
	var sub subscriptions.Record
	metrics := sitemetrics.Engine{DB: h.DB, Connections: h.Connections}
	started := time.Now().UTC()
	count := func(c context.Context, kind string) int {
		t.Helper()
		var n int
		if e := h.DB.QueryRow(c, `SELECT count(*) FROM outbox_events WHERE event_type=$1`, kind).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	cases := []struct {
		kind  string
		apply func(context.Context) error
	}{
		{"site.account.updated", func(c context.Context) error {
			in.Revision = account.Revision
			next, e := repo.Save(c, account.ID, in)
			if e == nil {
				account = next
			}
			return e
		}},
		{"site.metrics.refreshed", func(c context.Context) error {
			return metrics.Save(c, account.ID, account.Revision, "", started, sites.Metrics{Name: "NEVER_EVENT_SECRET", Notes: []string{"cookie=NEVER_EVENT_SECRET"}}, nil)
		}},
		{"subscription.created", func(c context.Context) error {
			next, e := engine.Save(c, "", subIn)
			if e == nil {
				sub = next
			}
			return e
		}},
		{"subscription.updated", func(c context.Context) error {
			subIn.Revision = sub.Revision
			next, e := engine.Save(c, sub.ID, subIn)
			if e == nil {
				sub = next
			}
			return e
		}},
		{"subscription.deleted", func(c context.Context) error { return engine.Delete(c, sub.ID) }},
		{"site.account.deleted", func(c context.Context) error { return repo.Delete(c, account.ID) }},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			before := count(ctx, tc.kind)
			sentinel := errors.New("rollback after staged fact")
			called := false
			guarded := dbutil.WithCommitGuard(ctx, func(c context.Context, tx pgx.Tx) error {
				called = true
				var staged int
				if e := tx.QueryRow(c, `SELECT count(*) FROM outbox_events WHERE event_type=$1`, tc.kind).Scan(&staged); e != nil {
					return e
				}
				if staged != before+1 || count(ctx, tc.kind) != before {
					t.Error("fact visible before commit", staged, before)
				}
				return sentinel
			})
			if e := tc.apply(guarded); e == nil || !called {
				t.Fatal("rollback not reached", e, called)
			}
			if got := count(ctx, tc.kind); got != before {
				t.Fatal("rollback published", got, before)
			}
			if e := tc.apply(ctx); e != nil {
				t.Fatal(e)
			}
			if got := count(ctx, tc.kind); got != before+1 {
				t.Fatal("successful fact missing", got, before)
			}
			var raw []byte
			if e := h.DB.QueryRow(ctx, `SELECT payload FROM outbox_events WHERE event_type=$1 ORDER BY created_at DESC LIMIT 1`, tc.kind).Scan(&raw); e != nil {
				t.Fatal(e)
			}
			if strings.Contains(string(raw), "NEVER_EVENT_SECRET") || strings.Contains(string(raw), "https://") {
				t.Fatal("secret leaked", tc.kind)
			}
			var e error
			switch tc.kind {
			case "site.account.updated", "site.account.deleted":
				var v contracts.SiteAccountChanged
				e = contracts.Decode(raw, &v)
			case "site.metrics.refreshed":
				var v contracts.SiteMetricsRefreshed
				e = contracts.Decode(raw, &v)
			default:
				var v contracts.SubscriptionChanged
				e = contracts.Decode(raw, &v)
			}
			if e != nil {
				t.Fatal("typed payload", e)
			}
			if tc.kind == "site.metrics.refreshed" {
				if e = tc.apply(ctx); e != nil {
					t.Fatal(e)
				}
				if e = metrics.Save(ctx, account.ID, account.Revision, "", started.Add(time.Second), sites.Metrics{}, errors.New("unknown private source")); e != nil {
					t.Fatal(e)
				}
				if got := count(ctx, tc.kind); got != before+1 {
					t.Fatal("replayed/failed collection emitted success", got)
				}
			}
		})
	}
	// An unrelated integration must never masquerade as a site account event.
	var raw json.RawMessage
	if e := h.DB.QueryRow(ctx, `SELECT jsonb_agg(payload) FROM outbox_events WHERE event_type LIKE 'site.account.%'`).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(raw), "NEVER_EVENT_SECRET") {
		t.Fatal("account event leak")
	}
}
