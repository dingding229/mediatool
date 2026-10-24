package playeraccess

import (
	"context"
	"errors"
	"fmt"
	"media-tool/internal/platform/database"
	"os"
	"testing"
	"time"
)

func TestPostgresUARecordsWhitelistRevisionAndBoundedStorage(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if db.QueryRow(ctx, `SELECT current_database()`).Scan(&name) != nil || name != "media_tool_test" {
		t.Fatal("isolated database only")
	}
	reset := `TRUNCATE player_ua_rules,player_ua_observations; INSERT INTO player_ua_policy(id)VALUES('main')ON CONFLICT(id)DO UPDATE SET enabled=false,mode='off',revision=1`
	if _, err = db.Exec(ctx, reset); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), reset)
	repo := Repository{DB: db}
	p, err := repo.Policy(ctx)
	if err != nil || p.Enabled || p.Mode != "off" || p.Revision != 1 || len(p.Rules) != 0 {
		t.Fatal(p, err)
	}
	req := Request{UserAgent: "SenPlayer/1.0", Entry: "strm", Method: "GET", ClientIP: "192.0.2.5"}
	d, err := repo.Check(ctx, req)
	if err != nil || !d.Allowed || d.Reason != "restriction_disabled" {
		t.Fatal(d, err)
	}
	req.Method = "HEAD"
	if _, err = repo.Check(ctx, req); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.Observations(ctx, "SenPlayer", "", 1, 25)
	if err != nil || len(rows.Items) != 1 || rows.Items[0].RequestCount != 2 || rows.Items[0].AllowedCount != 2 || rows.Items[0].Whitelisted {
		t.Fatal(rows, err)
	}
	id := rows.Items[0].ID
	p, err = repo.AllowObservation(ctx, id, "TV", p.Revision)
	if err != nil || p.Enabled || len(p.Rules) != 1 || p.Rules[0].UserAgent != req.UserAgent || p.Rules[0].MatchType != "exact" {
		t.Fatal(p, err)
	}
	old := p
	p.Enabled = true
	p.Mode = "whitelist"
	p, err = repo.Save(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Save(ctx, old); !errors.Is(err, ErrConflict) {
		t.Fatal("stale edit", err)
	}
	d, err = repo.Check(ctx, req)
	if err != nil || !d.Allowed || d.Reason != "whitelisted" {
		t.Fatal(d, err)
	}
	req.UserAgent = "senplayer/1.0"
	d, err = repo.Check(ctx, req)
	if err != nil || d.Allowed {
		t.Fatal(d, err)
	}
	req.UserAgent = ""
	d, err = repo.Check(ctx, req)
	if err != nil || d.Allowed {
		t.Fatal("empty UA", d, err)
	}
	if _, err = repo.AllowObservation(ctx, ObservationID(""), "", p.Revision); err == nil {
		t.Fatal("empty rule")
	}
	rows, err = repo.Observations(ctx, "", "unlisted", 1, 25)
	if err != nil || rows.Total != 2 {
		t.Fatal(rows, err)
	}
	rows, err = repo.Observations(ctx, "", "whitelisted", 1, 25)
	if err != nil || rows.Total != 1 || rows.Items[0].LastMethod != "HEAD" || rows.Items[0].RequestCount != 3 {
		t.Fatal(rows, err)
	}
	p.Rules = append(p.Rules, Rule{MatchType: "prefix", UserAgent: "VLC/", Enabled: true})
	p, err = repo.Save(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	req.UserAgent = "VLC/3"
	d, err = repo.Check(ctx, req)
	if err != nil || !d.Allowed {
		t.Fatal(d, err)
	}
	// A current rule can be removed without clearing its historical evidence.
	p.Rules = p.Rules[1:]
	p, err = repo.Save(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	req.UserAgent = "SenPlayer/1.0"
	d, err = repo.Check(ctx, req)
	if err != nil || d.Allowed {
		t.Fatal("rule deletion not immediate", d, err)
	}
	rows, err = repo.Observations(ctx, "SenPlayer/1.0", "", 1, 25)
	if err != nil || rows.Items[0].Whitelisted || rows.Items[0].DeniedCount != 1 {
		t.Fatal(rows, err)
	}
	if err = repo.Clear(ctx, false, p.Revision); err == nil {
		t.Fatal("no confirmation")
	}
	if err = repo.Clear(ctx, true, p.Revision-1); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err = repo.Clear(ctx, true, p.Revision); err != nil {
		t.Fatal(err)
	}
	after, _ := repo.Policy(ctx)
	if len(after.Rules) != 1 || !after.Enabled {
		t.Fatal("clear deleted policy")
	}
	if _, err = repo.AllowObservation(ctx, id, "", p.Revision); !errors.Is(err, ErrNotFound) {
		t.Fatal("cleared record replay", err)
	}
	// Exercise capacity without 2000 network requests or real media records.
	for i := 0; i < MaxObservations; i++ {
		ua := fmt.Sprintf("owned-fixture/%d", i)
		_, err = db.Exec(ctx, `INSERT INTO player_ua_observations(id,user_agent,last_allowed,last_reason,last_entry,last_method,last_client_ip,last_policy_revision,last_seen_at)VALUES($1,$2,false,'owned','strm','GET','192.0.2.5',1,now()-interval '1 day')`, ObservationID(ua), ua)
		if err != nil {
			t.Fatal(err)
		}
	}
	req.UserAgent = "owned-new/1"
	if _, err = repo.Check(ctx, req); err != nil {
		t.Fatal(err)
	}
	var count int
	if db.QueryRow(ctx, `SELECT count(*) FROM player_ua_observations`).Scan(&count) != nil || count != MaxObservations {
		t.Fatal("unbounded aggregate", count)
	}
	if _, err = (Repository{}).Check(ctx, req); !errors.Is(err, ErrUnavailable) {
		t.Fatal("nil database bypass")
	}
}

func TestConcurrentPlaybackChecksDoNotSerializeOnAdministratorLock(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if db.QueryRow(ctx, `SELECT current_database()`).Scan(&name) != nil || name != "media_tool_test" {
		t.Skip("isolated database only")
	}
	if _, err = db.Exec(ctx, `TRUNCATE player_ua_rules,player_ua_observations;UPDATE player_ua_policy SET enabled=false,mode='off',revision=1 WHERE id='main'`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), `TRUNCATE player_ua_rules,player_ua_observations`)
	repo := Repository{DB: db}
	lock, err := db.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	if _, err = lock.Exec(ctx, `SELECT pg_advisory_lock(745)`); err != nil {
		t.Fatal(err)
	}
	defer lock.Exec(ctx, `SELECT pg_advisory_unlock(745)`)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	const n = 20
	errs := make(chan error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go func() {
			<-start
			d, e := repo.Check(ctx, Request{UserAgent: "SenPlayer/6.2.1", Entry: "strm", Method: "GET", ClientIP: "198.51.100.10"})
			if e == nil {
				_, e = repo.RecordEvent(ctx, Request{UserAgent: "SenPlayer/6.2.1", Entry: "strm", Method: "GET"}, d)
			}
			errs <- e
		}()
	}
	close(start)
	for i := 0; i < n; i++ {
		if e := <-errs; e != nil {
			t.Fatal(e)
		}
	}
}
