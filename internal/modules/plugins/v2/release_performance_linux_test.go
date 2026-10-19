//go:build linux

package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"log/slog"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/plugins/v2/sandbox"
	"media-tool/internal/platform/testworker"
	"media-tool/internal/platform/trace"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

// Real receipt/log samples only; failures/fallbacks are measured, never hidden by
// retrying a timed sample. The shared registry barrier is not a policy warm-up.
func TestRRealWorkerColdStartResourcesAndChainSamples(t *testing.T) {
	if os.Getenv("MEDIA_TOOL_PLUGIN_BINARY") == "" {
		t.Skip("native static worker required; no samples")
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
	run, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	start := time.Now()
	var log taskLog
	logger := slog.New(slog.NewJSONHandler(&log, nil))
	go func() {
		done <- (Supervisor{Store: s, Executable: os.Getenv("MEDIA_TOOL_PLUGIN_BINARY"), Logger: logger, MaxWorkers: 1}).Run(run)
	}()
	defer func() {
		stop()
		select {
		case e := <-done:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(15 * time.Second):
			t.Error("reaper timeout")
		}
	}()
	testworker.Ready(t, s.DB, in.ID, func(c context.Context) error { _, e := s.UI(c, in.ID, "form", ""); return e })
	ready := time.Since(start)
	identity := contracts.MediaIdentity{TMDBID: 1, MediaType: "movie"}
	q := contracts.MediaConversion{Candidates: []contracts.MediaIdentity{identity}, Selected: &identity}
	const n = 25
	elapsed := make([]time.Duration, 0, n)
	for range n {
		began := time.Now()
		_, selected, e := s.ConvertMedia(trace.With(ctx, logger), q)
		elapsed = append(elapsed, time.Since(began))
		if e != nil || selected == nil || *selected != identity {
			t.Fatal("builtin/no-op result changed", e)
		}
	}
	first := elapsed[0]
	sort.Slice(elapsed, func(i, j int) bool { return elapsed[i] < elapsed[j] })
	// Allow asynchronous deadline receipts to reach terminal state outside timed samples.
	until := time.Now().Add(2 * time.Second)
	for {
		var pending int
		if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs WHERE instance_id=$1 AND kind='chain' AND state IN('pending','running')`, in.ID).Scan(&pending); e != nil {
			t.Fatal(e)
		}
		if pending == 0 {
			break
		}
		if time.Now().After(until) {
			t.Fatal("timed receipts did not converge")
		}
		time.Sleep(10 * time.Millisecond)
	}
	var samples, timeouts int
	if e = s.DB.QueryRow(ctx, `SELECT count(*),count(*)FILTER(WHERE reason='deadline') FROM plugin_v2_runs WHERE instance_id=$1 AND handler=$2`, in.ID, contracts.HandlerID("chain", string(contracts.ChainMediaRecognizeConvert))).Scan(&samples, &timeouts); e != nil {
		t.Fatal(e)
	}
	fallbacks := strings.Count(log.String(), "资源策略回落内置逻辑") + strings.Count(log.String(), "拒绝越权资源策略结果并回落")
	var pid int
	if e = s.DB.QueryRow(ctx, `SELECT worker_pid FROM plugin_v2_instances WHERE id=$1`, in.ID).Scan(&pid); e != nil {
		t.Fatal(e)
	}
	all, e := sandbox.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	var rss, tasks uint64
	for _, p := range sandbox.Tree(pid, all) {
		rss += p.RSS
		tasks += p.Tasks
	}
	if samples == 0 || pid <= 0 || rss == 0 || tasks == 0 {
		t.Fatal("real samples/resources absent")
	}
	t.Logf("R_PERF realWorker startupToRegistryAndForm=%s firstChain=%s p50=%s p95=%s calls=%d receipts=%d deadlineReceipts=%d fallbackWarnings=%d fallbackRate=%.2f%% rssBytes=%d tasks=%d budget=200ms/2s maxHandlers=8", ready, first, elapsed[n/2], elapsed[n*95/100], n, samples, timeouts, fallbacks, 100*float64(fallbacks)/n, rss, tasks)
}
