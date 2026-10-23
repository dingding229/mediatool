package migrations_test

import (
	"context"
	"fmt"
	"log/slog"
	"media-tool/internal/modules/playbackstats"
	"strings"
	"testing"
	"time"
)

func TestF7RealPlaybackSequenceAndStatisticsFailureDoesNotChange302(t *testing.T) {
	f := newReportFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 79); e != nil {
		t.Fatal(e)
	}
	recorder := playbackstats.New(f.pool, slog.Default())
	ctx, cancel := context.WithCancel(f.ctx)
	done := make(chan struct{})
	go func() { defer close(done); recorder.Run(ctx, func(context.Context) (int, error) { return 180, nil }) }()
	defer func() { cancel(); <-done }()
	f.service.Stats = recorder
	id := reportInfo(t, f)
	base := fmt.Sprintf(`"ItemId":%q,"MediaSourceId":%q,"PlaySessionId":%q`, f.item, f.a.asset, id)
	mustReport(t, f, "/Sessions/Playing", "{"+base+`,"PositionTicks":0}`)
	for _, ua := range []string{"SenPlayer/1", "Infuse/8", "Generic-Player/1"} {
		w := f.call("GET", "/Videos/"+f.item+"/stream.mkv?MediaSourceId="+f.a.asset, f.token, ua)
		if w.Code != 302 {
			t.Fatal(ua, w.Code)
		}
	}
	mustReport(t, f, "/Sessions/Playing/Stopped", "{"+base+`,"PositionTicks":300000000}`)
	deadline := time.Now().Add(3 * time.Second)
	for {
		if f.integer(t, `SELECT count(*) FROM media_playback_events WHERE state='stopped' AND redirect_count=3`) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("async projection missing")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if f.integer(t, `SELECT count(*) FROM media_playback_events`) != 1 {
		t.Fatal("reports/302 multiplied sessions")
	}
	// Inject a database analytics failure without modifying playback tables.
	if _, e := f.db.ExecContext(f.ctx, `ALTER TABLE media_playback_events RENAME TO f7_events_unavailable`); e != nil {
		t.Fatal(e)
	}
	defer func() {
		if _, e := f.db.ExecContext(f.ctx, `ALTER TABLE f7_events_unavailable RENAME TO media_playback_events`); e != nil {
			t.Error(e)
		}
	}()
	for _, ua := range []string{"SenPlayer/1", "Infuse/8", "Generic-Player/1"} {
		w := f.call("GET", "/Videos/"+f.item+"/stream.mkv?MediaSourceId="+f.a.asset, f.token, ua)
		if w.Code != 302 || !strings.HasPrefix(w.Header().Get("Location"), "https://cdn.synthetic.invalid/") {
			t.Fatal("analytics broke playback", ua, w.Code)
		}
	}
}
