package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"log/slog"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/plugins/v2/sandbox"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/outbox"
	"media-tool/internal/platform/testworker"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestRealSameBinaryDiscoveryCoverageAndMetadata(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("MEDIA_TOOL_PLUGIN_BINARY") == "" {
		t.Skip("mandatory native AMD64 static worker gate")
	}
	if os.Getenv("MEDIA_TOOL_TEST_PLUGIN_UID") == "65532" && os.Getuid() != 65532 {
		t.Fatal("native host UID")
	}
	if e := sandbox.Parent(); e != nil {
		t.Fatal(e)
	}
	s := fixture(t)
	ctx := context.Background()
	defs, e := Catalog()
	if e != nil {
		t.Fatal(e)
	}
	d := defs[0]
	in, e := s.Apply(ctx, "test", Request{Kind: Install, PluginID: d.Manifest.ID, Fingerprint: d.Fingerprint, Scopes: scopes(d), OperationKey: jobs.UUID(), Confirm: true, Config: json.RawMessage(`{"scheduled":false}`)})
	if e != nil {
		t.Fatal(e)
	}
	in, e = s.Apply(ctx, "test", Request{Kind: Enable, InstanceID: in.ID, ExpectedRevision: in.Revision, Fingerprint: d.Fingerprint, OperationKey: jobs.UUID(), Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	run, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- (Supervisor{Store: s, Executable: os.Getenv("MEDIA_TOOL_PLUGIN_BINARY"), Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)), MaxWorkers: 1}).Run(run)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case e := <-done:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(15 * time.Second):
			t.Error("worker reaper timeout")
		}
	})
	wait := func(name string, fn func() bool) {
		t.Helper()
		until := time.Now().Add(15 * time.Second)
		for time.Now().Before(until) {
			if fn() {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Fatal(name)
	}
	testworker.Ready(t, s.DB, in.ID, func(c context.Context) error { _, err := s.UI(c, in.ID, "form", ""); return err })
	proofUntil := time.Now().Add(15 * time.Second)
	identity := contracts.MediaIdentity{TMDBID: 1, MediaType: "series"}
	proveRealPolicy(t, s, in.ID, contracts.ChainMediaRecognizeConvert, proofUntil, func() bool {
		prepared, selected, e := s.ConvertMedia(ctx, contracts.MediaConversion{Candidates: []contracts.MediaIdentity{identity}, Selected: &identity})
		if e != nil || selected == nil || *selected != identity {
			t.Fatal(selected, e)
		}
		if e = dbutil.CheckPrepared(prepared, s.DB); e != nil {
			t.Fatal(e)
		}
		return true
	})
	expectation := contracts.EpisodeExpectation{SubscriptionID: jobs.UUID(), Revision: 1, TMDBID: 1, Season: 1, Expected: []int{1, 2}}
	proveRealPolicy(t, s, in.ID, contracts.ChainSubscribeEpisodesRefresh, proofUntil, func() bool {
		prepared, numbers, e := s.SupplementEpisodes(ctx, expectation)
		if e != nil || len(numbers) != 2 {
			t.Fatal(numbers, e)
		}
		if e = dbutil.CheckPrepared(prepared, s.DB); e != nil {
			t.Fatal(e)
		}
		return true
	})
	check := contracts.CompletionCheck{EpisodeExpectation: expectation, Owned: []int{1, 2}, Complete: true}
	proveRealPolicy(t, s, in.ID, contracts.ChainSubscribeCompletionCheck, proofUntil, func() bool {
		prepared, allow, e := s.CheckCoverage(ctx, check)
		if e != nil || !allow {
			t.Fatal(allow, e)
		}
		if e = dbutil.CheckPrepared(prepared, s.DB); e != nil {
			t.Fatal(e)
		}
		return true
	})
	for _, typ := range []contracts.ChainEventType{contracts.ChainDiscoverSource, contracts.ChainRecommendSource} {
		proveRealPolicy(t, s, in.ID, typ, proofUntil, func() bool {
			providers, e := s.SourceProviders(ctx, typ)
			if e != nil {
				t.Fatal(e)
			}
			if len(providers) == 0 {
				return false
			} // A not-current worker legally exposes no sources.
			if len(providers) != 1 {
				t.Fatal("ambiguous provider registry", providers)
			}
			result, e := s.SourcePage(ctx, providers[0], typ, contracts.SourceRequest{Page: 1, Size: 200})
			if policyTimedOut(e) {
				return false
			}
			if e != nil || len(result.Items) != 0 {
				t.Fatal("no-op provider", result, e)
			}
			return true
		})
	}
	var n int
	if e = s.DB.QueryRow(ctx, `SELECT count(DISTINCT handler) FROM plugin_v2_runs WHERE instance_id=$1 AND kind='chain' AND state='succeeded'`, in.ID).Scan(&n); e != nil || n != 5 {
		t.Fatal("five real chains", n, e)
	}
	facts := map[string]any{"metadata.scraped": contracts.MetadataScraped{ObjectID: jobs.UUID(), Kind: "asset", MediaIdentity: identity, Revision: "1"}, "subscription.coverage.completed": contracts.CoverageObservation{CompletionCheck: check, State: "complete", Evidence: "tmdb_season_exposable_assets", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer rollback(ctx, tx)
	for typ, fact := range facts {
		if e = outbox.EmitVersionTx(ctx, tx, typ, expectation.SubscriptionID, "v1", fact); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	wait("two committed facts in real Go worker KV", func() bool {
		if e = s.Bridge(ctx); e != nil {
			t.Fatal(e)
		}
		if e = s.Deliver(ctx); e != nil {
			t.Fatal(e)
		}
		e = s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_kv WHERE instance_id=$1 AND key LIKE 'acquisition-%'`, in.ID).Scan(&n)
		return e == nil && n == 2
	})
	current, e := s.List(ctx, 0, 200)
	if e != nil || len(current) != 1 {
		t.Fatal(e)
	}
	if _, e = s.Apply(ctx, "test", Request{Kind: Disable, InstanceID: in.ID, ExpectedRevision: current[0].Revision, Fingerprint: d.Fingerprint, OperationKey: jobs.UUID(), Confirm: true}); e != nil {
		t.Fatal(e)
	}
	wait("disabled/reaped", func() bool {
		var stopped bool
		e = s.DB.QueryRow(ctx, `SELECT runtime_state='stopped' AND worker_pid=0 AND generation IS NULL FROM plugin_v2_instances WHERE id=$1`, in.ID).Scan(&stopped)
		return e == nil && stopped
	})
	if _, _, e = s.Descriptor(ctx, in.ID, "chain", "discover.source"); e == nil {
		t.Fatal("disabled source callable")
	}
	t.Log("REAL_AMD64_F4C: five typed no-op chains + two committed facts -> real Go worker KV; disabled and reaped")
}
