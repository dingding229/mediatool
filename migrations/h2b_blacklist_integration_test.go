package migrations_test

import (
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/mediausers"
	"media-tool/internal/modules/playeraccess"
)

func TestH2bBlacklistUpDownUpAndUserSummary(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 80); e != nil {
		t.Fatal(e)
	}
	const white = "11111111-0000-4000-8000-000000000081"
	f.exec(t, `UPDATE player_ua_policy SET enabled=true`)
	f.exec(t, `INSERT INTO player_ua_rules(id,label,match_type,user_agent)VALUES($1,'原有','prefix','OwnedPlayer/')`, white)
	if _, e := f.provider.UpTo(f.ctx, 81); e != nil {
		t.Fatal(e)
	}
	var mode, list string
	if e := f.db.QueryRowContext(f.ctx, `SELECT p.mode,r.list_type FROM player_ua_policy p CROSS JOIN player_ua_rules r`).Scan(&mode, &list); e != nil || mode != "whitelist" || list != "whitelist" {
		t.Fatal(mode, list, e)
	}
	if _, e := f.provider.Down(f.ctx); e != nil {
		t.Fatal(e)
	}
	var ua string
	var enabled bool
	if e := f.db.QueryRowContext(f.ctx, `SELECT p.enabled,r.user_agent FROM player_ua_policy p CROSS JOIN player_ua_rules r`).Scan(&enabled, &ua); e != nil || !enabled || ua != "OwnedPlayer/" {
		t.Fatal(enabled, ua, e)
	}
	if _, e := f.provider.UpTo(f.ctx, 81); e != nil {
		t.Fatal(e)
	}
	// Reuse this fixture's isolated schema; no production/public data is accessed.
	var schema string
	if e := f.db.QueryRowContext(f.ctx, `SELECT current_schema()`).Scan(&schema); e != nil {
		t.Fatal(e)
	}
	cfg, e := pgxpool.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if e != nil {
		t.Fatal(e)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, e := pgxpool.NewWithConfig(f.ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	repo := playeraccess.Repository{DB: pool}
	p, e := repo.Policy(f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	req := playeraccess.Request{UserAgent: "OwnedPlayer/1", Entry: "strm", Method: "GET", ClientIP: "8.8.8.8"}
	if _, e = repo.Check(f.ctx, req); e != nil {
		t.Fatal(e)
	}
	p, e = repo.AddObservation(f.ctx, playeraccess.ObservationID(req.UserAgent), "拒绝", p.Revision, "blacklist")
	if e != nil {
		t.Fatal(e)
	}
	if p.Mode != "whitelist" || len(p.Rules) != 1 || len(p.BlacklistRules) != 1 {
		t.Fatal(p)
	}
	blackID := p.BlacklistRules[0].ID
	for _, m := range []string{"blacklist", "off", "whitelist", "blacklist"} {
		p, e = repo.Save(f.ctx, playeraccess.Policy{Revision: p.Revision, Mode: m})
		if e != nil {
			t.Fatal(e)
		}
		if len(p.Rules) != 1 || len(p.BlacklistRules) != 1 || p.BlacklistRules[0].ID != blackID {
			t.Fatal("list lost", p)
		}
		d, e := repo.Check(f.ctx, req)
		if e != nil || d.Allowed == (m == "blacklist") {
			t.Fatal(m, d, e)
		}
	}
	observations, e := repo.Observations(f.ctx, "Owned", "blacklisted", 1, 25)
	if e != nil || len(observations.Items) != 1 || !observations.Items[0].Blacklisted || !observations.Items[0].Whitelisted || observations.Items[0].LastReason != "blacklisted" {
		t.Fatal(observations, e)
	}
	stale := p
	stale.Revision--
	if _, e = repo.Save(f.ctx, stale); e != playeraccess.ErrConflict {
		t.Fatal("revision fence", e)
	}
	if _, e = repo.AddObservation(f.ctx, playeraccess.ObservationID(req.UserAgent), "重复", p.Revision, "blacklist"); e != nil {
		t.Fatal(e)
	}
	p, _ = repo.Policy(f.ctx)
	if len(p.BlacklistRules) != 1 {
		t.Fatal("duplicate rule")
	}
	// Contains patterns survive persistence and the SQL observation filter agrees
	// with in-memory admission. A rule ID cannot be moved across lists.
	p.BlacklistRules[0].MatchType = "contains"
	p.BlacklistRules[0].UserAgent = "Player"
	p, e = repo.Save(f.ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	req.UserAgent = "Client Player/contains"
	d, e := repo.Check(f.ctx, req)
	if e != nil || d.Allowed || d.RuleID != blackID {
		t.Fatal(d, e)
	}
	observations, e = repo.Observations(f.ctx, "Client", "blacklisted", 1, 25)
	if e != nil || len(observations.Items) != 1 || observations.Items[0].Whitelisted {
		t.Fatal(observations, e)
	}
	bad := p
	bad.BlacklistRules = []playeraccess.Rule{p.Rules[0]}
	bad.Rules = []playeraccess.Rule{}
	if _, e = repo.Save(f.ctx, bad); e == nil {
		t.Fatal("moved foreign list ID")
	}
	after, e := repo.Policy(f.ctx)
	if e != nil || after.Revision != p.Revision || len(after.Rules) != 1 || len(after.BlacklistRules) != 1 {
		t.Fatal("rejected save was not atomic", after, e)
	}
	// User listing includes aggregate facts without per-user DB requests.
	f.exec(t, `INSERT INTO media_users(id,username,display_name,password_hash)VALUES($1,'h2b-user','家庭用户','not-a-login-fixture')`, userOne)
	f.exec(t, `INSERT INTO media_user_devices(id,user_id,device_key,name,client)VALUES($1,$2,'h2b','设备','播放器')`, deviceOne, userOne)
	f.exec(t, `INSERT INTO media_device_ips(user_id,device_id,ip,first_seen_at,last_seen_at)VALUES($1,$2,'8.8.8.8',now(),now())`, userOne, deviceOne)
	f.exec(t, `INSERT INTO media_playback_events(id,user_id,device_id,item_id,media_type,name,started_at,last_seen_at,state)VALUES($1,$2,$3,123,'movie','测试影片',now(),now(),'stopped')`, sessionID, userOne, deviceOne)
	users := mediausers.Repository{DB: pool, AuthSecret: "h2b-isolated-test"}
	for i := 0; i < 10; i++ {
		if _, e = users.TakeAttempt(f.ctx, "h2b-isolated-test", "user:h2b-user", 10); e != nil {
			t.Fatal(e)
		}
	}
	rows, e := users.AdminPage(f.ctx, 0, 25)
	if e != nil || len(rows) != 1 || rows[0].DeviceCount != 1 || rows[0].LastIP != "8.8.8.8" || rows[0].LastPlayedAt == nil || rows[0].LockedUntil == nil || !rows[0].LockedUntil.After(time.Now()) {
		t.Fatal(rows, e)
	}
	rows, e = users.AdminPage(f.ctx, 25, 25)
	if e != nil || len(rows) != 0 {
		t.Fatal(rows, e)
	}
	f.exec(t, `UPDATE media_user_auth_limits SET window_start=now()-interval '2 minutes'`)
	rows, e = users.AdminPage(f.ctx, 0, 25)
	if e != nil || rows[0].LockedUntil != nil {
		t.Fatal("expired lock", rows, e)
	}

}
