package migrations_test

import (
	"media-tool/internal/modules/playeraccess"
	"strings"
	"testing"
)

func TestH2bNativePlaybackInfoAndRedirectShareThreeModes(t *testing.T) {
	f := newPlaybackFixture(t)
	repo := playeraccess.Repository{DB: f.pool}
	p, e := repo.Policy(f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	info := "/Items/" + f.item + "/PlaybackInfo?MediaSourceId=" + f.a.asset
	stream := "/Videos/" + f.item + "/stream.mkv?MediaSourceId=" + f.a.asset
	for _, kind := range []string{"exact", "prefix", "contains"} {
		pattern, matched := "BlockedNative/1", "BlockedNative/1"
		if kind == "prefix" {
			pattern = "BlockedNative/"
		}
		if kind == "contains" {
			pattern = "BlockedNative"
			matched = "Client BlockedNative/1"
		}
		p.Mode = "off"
		p.Rules = []playeraccess.Rule{{MatchType: kind, UserAgent: pattern, Enabled: true}}
		p.BlacklistRules = []playeraccess.Rule{{MatchType: kind, UserAgent: pattern, Enabled: true}}
		p, e = repo.Save(f.ctx, p)
		if e != nil {
			t.Fatal(e)
		}
		for _, mode := range []string{"off", "whitelist", "blacklist"} {
			p, e = repo.Save(f.ctx, playeraccess.Policy{Revision: p.Revision, Mode: mode})
			if e != nil {
				t.Fatal(e)
			}
			for _, ua := range []string{matched, "AllowedNative/1"} {
				allowed := mode == "off" || mode == "whitelist" && ua == matched || mode == "blacklist" && ua != matched
				for _, path := range []string{info, stream} {
					before := f.downloads.Load()
					w := f.call("GET", path, f.token, ua)
					want := 403
					if allowed {
						want = 302
						if path == info {
							want = 200
						}
					}
					if w.Code != want {
						t.Fatal(kind, mode, ua, path, w.Code, w.Body.String())
					}
					if !allowed && (w.Header().Get("Location") != "" || f.downloads.Load() != before || !strings.Contains(w.Body.String(), "未通过准入策略")) {
						t.Fatal("denial leaked URL or bypassed gate", w.Body.String())
					}
				}
			}
		}
		page, e := repo.Observations(f.ctx, matched, "blacklisted", 1, 25)
		if e != nil || len(page.Items) != 1 || page.Items[0].LastReason != "blacklisted" || page.Items[0].DeniedCount < 2 {
			t.Fatal("native denial missing observation", page, e)
		}
	}
}
