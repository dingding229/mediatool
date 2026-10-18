package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"media-tool/internal/modules/jobs"
	dbutil "media-tool/internal/platform/database"
	"testing"
	"time"
)

func discoveryFixture(t *testing.T) (Store, Lease) {
	s, l := activeFixture(t)
	m := contracts.Snapshot{Version: 2}
	for _, id := range []string{"media.recognize.convert", "subscribe.episodes.refresh", "subscribe.completion.check", "discover.source", "recommend.source"} {
		m.Descriptors = append(m.Descriptors, contracts.Descriptor{Kind: "chain", ID: id, Handler: contracts.HandlerID("chain", id), RequiredScopes: []string{"events.chain." + id}})
	}
	if e := s.Publish(context.Background(), l, m); e != nil {
		t.Fatal(e)
	}
	if e := s.Renew(context.Background(), l, true); e != nil {
		t.Fatal(e)
	}
	return s, l
}
func TestDiscoveryPoliciesNarrowTimeoutInvalidAndCommitFence(t *testing.T) {
	s, l := discoveryFixture(t)
	ctx := context.Background()
	a := contracts.MediaIdentity{TMDBID: 1, MediaType: "movie"}
	b := contracts.MediaIdentity{TMDBID: 1, MediaType: "series"}
	q := contracts.MediaConversion{Candidates: []contracts.MediaIdentity{a, b}, Selected: &a}
	for _, tc := range []struct {
		raw  string
		want contracts.MediaIdentity
	}{{`{"selected":{"tmdbId":1,"mediaType":"series"}}`, b}, {`{"selected":{"tmdbId":999,"mediaType":"movie"}}`, a}, {`{"selected":{"tmdbId":1,"mediaType":"tv"}}`, a}, {`{"selected":{"tmdbId":1,"mediaType":"movie"},"url":"https://private"}`, a}} {
		done := policyReply(s, json.RawMessage(tc.raw), false)
		prepared, got, e := s.ConvertMedia(ctx, q)
		if e != nil || got == nil || *got != tc.want {
			t.Fatal(got, e)
		}
		if e = <-done; e != nil {
			t.Fatal(e)
		}
		if e = dbutil.CheckPrepared(prepared, s.DB); e != nil {
			t.Fatal(e)
		}
	}
	before := time.Now()
	_, got, e := s.ConvertMedia(ctx, q)
	if e != nil || got == nil || *got != a || time.Since(before) > time.Second {
		t.Fatal("timeout fallback", got, e)
	}
	expectation := contracts.EpisodeExpectation{SubscriptionID: jobs.UUID(), Revision: 1, TMDBID: 1, Season: 1, Expected: []int{1, 2}}
	for _, tc := range []struct {
		raw  string
		want int
	}{{`{"expected":[1]}`, 2}, {`{"expected":[1,2,3]}`, 3}, {`{"expected":[1,2],"owned":[1,2]}`, 2}} {
		done := policyReply(s, json.RawMessage(tc.raw), false)
		_, got, e := s.SupplementEpisodes(ctx, expectation)
		if e != nil || len(got) != tc.want {
			t.Fatal(got, e)
		}
		if e = <-done; e != nil {
			t.Fatal(e)
		}
	}
	done := policyReply(s, json.RawMessage(`{"allow":true}`), false)
	_, allow, e := s.CheckCoverage(ctx, contracts.CompletionCheck{EpisodeExpectation: expectation, Owned: []int{1}, Complete: false})
	if e != nil || allow {
		t.Fatal("fabricated owned", allow, e)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	done = policyReply(s, json.RawMessage(`{"selected":{"tmdbId":1,"mediaType":"series"}}`), false)
	prepared, got, e := s.ConvertMedia(ctx, q)
	if e != nil || got == nil || *got != b {
		t.Fatal(got, e)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope='events.chain.media.recognize.convert'`, l.ID); e != nil {
		t.Fatal(e)
	}
	if dbutil.CheckPrepared(prepared, s.DB) == nil {
		t.Fatal("revoked prepared decision accepted")
	}
	_, got, e = s.ConvertMedia(ctx, q)
	if e != nil || got == nil || *got != a {
		t.Fatal("revoked hook", got, e)
	}
	for _, typ := range []contracts.ChainEventType{contracts.ChainDiscoverSource, contracts.ChainRecommendSource} {
		providers, e := s.SourceProviders(ctx, typ)
		if e != nil || len(providers) != 1 {
			t.Fatal(providers, e)
		}
		done = policyReply(s, json.RawMessage(`{"items":[{"tmdbId":1,"mediaType":"movie","title":"Demo","year":2026}],"hasMore":false}`), false)
		result, e := s.SourcePage(ctx, providers[0], typ, contracts.SourceRequest{Page: 1, Size: 200})
		if e != nil || len(result.Items) != 1 {
			t.Fatal(result, e)
		}
		if e = <-done; e != nil {
			t.Fatal(e)
		}
	}
}
func TestDiscoveryWireRejectsURLsOwnedAndInvalidIdentity(t *testing.T) {
	tests := []struct{ id, args, result string }{{"media.recognize.convert", `{"candidates":[{"tmdbId":1,"mediaType":"movie"}],"selected":null}`, `{"selected":{"tmdbId":1,"mediaType":"series"}}`}, {"discover.source", `{"page":1,"size":200}`, `{"items":[{"tmdbId":1,"mediaType":"movie","title":"Demo","year":2026,"poster":"/secret"}],"hasMore":false}`}, {"subscribe.episodes.refresh", `{"subscriptionId":"00000000-0000-0000-0000-000000000001","revision":1,"tmdbId":1,"season":1,"expected":[1,2]}`, `{"expected":[1]}`}}
	for _, tc := range tests {
		args := json.RawMessage(`{"type":"` + tc.id + `","schemaVersion":1,"payload":` + tc.args + `}`)
		if validExtensionResult("chain", tc.id, args, json.RawMessage(tc.result)) {
			t.Fatal(tc.id)
		}
	}
}
func TestDiscoveryModuleUsesSameIdentitySchema(t *testing.T) {
	identity := contracts.MediaIdentity{TMDBID: 1, MediaType: "movie"}
	args, _ := json.Marshal(contracts.MediaConversion{Candidates: []contracts.MediaIdentity{identity}})
	if !validExtensionResult("module", "metadata.identify", args, json.RawMessage(`{"selected":{"tmdbId":1,"mediaType":"movie"}}`)) {
		t.Fatal("legal module")
	}
	for _, raw := range []string{`{"selected":{"tmdbId":2,"mediaType":"movie"}}`, `{"selected":{"tmdbId":1,"mediaType":"series"}}`, `{"selected":null,"force":true}`} {
		if validExtensionResult("module", "metadata.identify", args, json.RawMessage(raw)) {
			t.Fatal("unknown module identity", raw)
		}
	}
}
func TestCoverageAndSourcesMissingScopeTimeoutNeverWiden(t *testing.T) {
	s, l := discoveryFixture(t)
	ctx := context.Background()
	q := contracts.EpisodeExpectation{SubscriptionID: jobs.UUID(), Revision: 1, TMDBID: 1, Season: 1, Expected: []int{1, 2}}
	before := time.Now()
	_, expected, e := s.SupplementEpisodes(ctx, q)
	if e != nil || len(expected) != 2 || time.Since(before) > time.Second {
		t.Fatal("expectation timeout", expected, e)
	}
	before = time.Now()
	_, allow, e := s.CheckCoverage(ctx, contracts.CompletionCheck{EpisodeExpectation: q, Owned: []int{1, 2}, Complete: true})
	if e != nil || !allow || time.Since(before) > time.Second {
		t.Fatal("completion timeout fallback", allow, e)
	}
	for _, typ := range []contracts.ChainEventType{contracts.ChainDiscoverSource, contracts.ChainRecommendSource} {
		providers, e := s.SourceProviders(ctx, typ)
		if e != nil || len(providers) != 1 {
			t.Fatal(e)
		}
		before = time.Now()
		v, e := s.SourcePage(ctx, providers[0], typ, contracts.SourceRequest{Page: 1, Size: 200})
		if e == nil || len(v.Items) != 0 || time.Since(before) > time.Second {
			t.Fatal("source timeout", v, e)
		}
	}
	if _, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope LIKE 'events.chain.%'`, l.ID); e != nil {
		t.Fatal(e)
	}
	_, expected, e = s.SupplementEpisodes(ctx, q)
	if e != nil || len(expected) != 2 {
		t.Fatal(expected, e)
	}
	_, allow, e = s.CheckCoverage(ctx, contracts.CompletionCheck{EpisodeExpectation: q, Owned: []int{1}, Complete: false})
	if e != nil || allow {
		t.Fatal("missing scope fabricated completion", allow, e)
	}
	for _, typ := range []contracts.ChainEventType{contracts.ChainDiscoverSource, contracts.ChainRecommendSource} {
		providers, e := s.SourceProviders(ctx, typ)
		if e != nil || len(providers) != 0 {
			t.Fatal("revoked source", providers, e)
		}
	}
}
