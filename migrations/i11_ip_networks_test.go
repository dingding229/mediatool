package migrations_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"media-tool/internal/modules/ipblock"
	"media-tool/internal/platform/auth"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/httpapi"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestI11MigrationMergeUpDownUp(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 91); e != nil {
		t.Fatal(e)
	}
	f.exec(t, `INSERT INTO player_ip_block_rules(id,network,note,created_at,updated_at,hit_count,last_hit_at)VALUES
 ('11000000-0000-4000-8000-000000000001','192.0.2.1','最早','2026-01-01','2026-03-01',2,'2026-03-01'),
 ('11000000-0000-4000-8000-000000000002','192.0.2.2','较新','2026-02-01','2026-04-01',3,'2026-04-01'),
 ('11000000-0000-4000-8000-000000000003','192.0.2.0/24','较新','2026-02-01','2026-02-01',4,NULL),
 ('11000000-0000-4000-8000-000000000004','192.0.0.0/16','更宽保留','2026-01-01','2026-01-01',0,NULL),
 ('11000000-0000-4000-8000-000000000005','2001:db8::1','','2026-01-01','2026-01-01',1,NULL),
 ('11000000-0000-4000-8000-000000000006','2001:db8::2','IPv6','2026-02-01','2026-02-01',2,NULL)`)
	for _, v := range []int64{92, 91, 92} {
		var e error
		if v == 91 {
			_, e = f.provider.DownTo(f.ctx, v)
		} else {
			_, e = f.provider.UpTo(f.ctx, v)
		}
		if e != nil {
			t.Fatal(e)
		}
		f.version(t, v)
		if f.integer(t, `SELECT count(*) FROM player_ip_block_rules`) != 3 {
			t.Fatal("merge count")
		}
		if f.text(t, `SELECT note FROM player_ip_block_rules WHERE network='192.0.2.0/24'`) != "较新\n较新\n最早" {
			t.Fatal("list-order notes not retained")
		}
		if f.text(t, `SELECT id::text FROM player_ip_block_rules WHERE network='192.0.2.0/24'`) != "11000000-0000-4000-8000-000000000001" {
			t.Fatal("oldest survivor")
		}
		if f.integer(t, `SELECT hit_count FROM player_ip_block_rules WHERE network='192.0.2.0/24'`) != 9 {
			t.Fatal("counter sum")
		}
		if f.integer(t, `SELECT count(*) FROM player_ip_block_rules WHERE network='192.0.2.0/24' AND created_at='2026-01-01' AND updated_at='2026-04-01' AND last_hit_at='2026-04-01'`) != 1 {
			t.Fatal("timestamps")
		}
		if f.text(t, `SELECT note FROM player_ip_block_rules WHERE network='2001:db8::/64'`) != "IPv6" {
			t.Fatal("v6 notes")
		}
		if f.integer(t, `SELECT revision FROM player_ip_block_policy`) != 2 {
			t.Fatal("unexpected revision change")
		}
		t.Logf("schema=%d rules=3 revision=2; Down never reconstructs original singles", v)
	}
	f.rejects(t, "23514", `UPDATE player_ip_block_rules SET note=repeat('字',501)`)
}

func TestI11MigrationRejectsOversizedNotesAndHitOverflowAtomically(t *testing.T) {
	for _, kind := range []string{"note", "hits"} {
		t.Run(kind, func(t *testing.T) {
			f := newNativeFixture(t)
			if _, e := f.provider.UpTo(f.ctx, 91); e != nil {
				t.Fatal(e)
			}
			note, hits := "repeat('字',260)", "0"
			if kind == "hits" {
				note = "''"
				hits = "9223372036854775807"
			}
			f.exec(t, fmt.Sprintf(`INSERT INTO player_ip_block_rules(id,network,note,hit_count)VALUES('11000000-0000-4000-8000-000000000001','192.0.2.1',%s,%s),('11000000-0000-4000-8000-000000000002','192.0.2.2',%s,1)`, note, hits, note))
			before := f.text(t, `SELECT jsonb_agg(to_jsonb(r) ORDER BY id)::text FROM player_ip_block_rules r`)
			if _, e := f.provider.UpTo(f.ctx, 92); e == nil {
				t.Fatal("expected whole migration abort")
			}
			f.version(t, 91)
			if f.text(t, `SELECT jsonb_agg(to_jsonb(r) ORDER BY id)::text FROM player_ip_block_rules r`) != before || f.integer(t, `SELECT revision FROM player_ip_block_policy`) != 1 {
				t.Fatal("partial migration committed")
			}
		})
	}
}

func TestI11EmptyMigrationDoesNotChangePolicy(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 91); e != nil {
		t.Fatal(e)
	}
	f.exec(t, `UPDATE player_ip_block_policy SET revision=17`)
	if _, e := f.provider.UpTo(f.ctx, 92); e != nil {
		t.Fatal(e)
	}
	if f.integer(t, `SELECT revision FROM player_ip_block_policy`) != 17 {
		t.Fatal("empty migration touched business revision")
	}
}

func TestI11PreviewAndTransactionalCoverage(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 92); e != nil {
		t.Fatal(e)
	}
	db := catalogTestPool(t, f)
	defer db.Close()
	f.exec(t, `INSERT INTO application_settings DEFAULT VALUES ON CONFLICT(singleton) DO UPDATE SET revision=application_settings.revision`)
	repo := ipblock.Repository{DB: db}
	gate := ipblock.New(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e := gate.Reload(f.ctx); e != nil {
		t.Fatal(e)
	}
	c := config.Config{Environment: "development", SessionKey: strings.Repeat("t", 64)}
	main := httpapi.New(c, httpapi.Dependencies{DB: db, IPBlock: gate, AuthSessions: h3Admin{}})
	token, _ := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	call := func(method, path string, body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
		r.RemoteAddr = "198.51.100.17:1234"
		r.Header.Set("X-Requested-With", "media-tool")
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		w := httptest.NewRecorder()
		main.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s status=%d want=%d %s", path, w.Code, want, w.Body.String())
		}
		var out map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	base := "/api/v1/player-access/ip-blocklist"
	preview := func(network, id string) map[string]any {
		return call("POST", base+"/check", map[string]any{"address": "192.0.2.7", "network": network, "excludeId": id}, 200)["data"].(map[string]any)
	}
	if preview("192.0.2.7", "")["candidateNetwork"] != "192.0.2.0/24" {
		t.Fatal("preview normalization")
	}
	call("POST", base, map[string]any{"network": "192.0.2.7", "revision": 1, "confirm": true}, 200)
	p, _ := repo.Load(f.ctx)
	id := p.Rules[0].ID
	if preview("192.0.2.201/32", "")["coveredBy"] != "192.0.2.0/24" {
		t.Fatal("whole subnet coverage")
	}
	call("POST", base, map[string]any{"network": "192.0.2.8", "revision": 1, "confirm": true}, 409)
	out := call("POST", base, map[string]any{"network": "192.0.2.8/32", "revision": 2, "confirm": true}, 400)
	if !strings.Contains(fmt.Sprint(out), "已在 192.0.2.0/24 中") {
		t.Fatal(out)
	}
	if f.integer(t, `SELECT revision FROM player_ip_block_policy`) != 2 || f.integer(t, `SELECT count(*) FROM audit_logs WHERE action='player_ip.rules_changed'`) != 1 {
		t.Fatal("duplicate had side effects")
	}
	// A broader new prefix is allowed; old narrow rules are retained.
	call("POST", base, map[string]any{"network": "192.0.9.9/16", "revision": 2, "confirm": true}, 200)
	if preview("192.0.2.0/24", id)["coveredBy"] != nil {
		t.Fatal("note-only edit blocked")
	}
	call("PUT", base+"/"+id, map[string]any{"network": "192.0.2.0/24", "note": "保留备注编辑", "revision": 3, "confirm": true}, 200)
	if f.integer(t, `SELECT count(*) FROM player_ip_block_rules`) != 2 {
		t.Fatal("broader prefix deleted narrow")
	}
	_, e := repo.Change(f.ctx, ipblock.Mutation{Network: "192.0.8.0/24", Revision: 4, Confirm: true})
	if !errors.Is(e, ipblock.ErrDuplicate) {
		t.Fatal("repository failed independent coverage check", e)
	}
	self := preview("198.51.100.17", "")
	if self["affectsCurrent"] != true || self["candidateNetwork"] != "198.51.100.0/24" {
		t.Fatal(self)
	}
	call("POST", base, map[string]any{"network": "198.51.100.17", "revision": 4, "confirm": true}, 400)
	call("POST", base+"/check", map[string]any{"address": "192.0.2.7", "network": "192.0.2.7", "excludeId": "invalid"}, 400)
}
