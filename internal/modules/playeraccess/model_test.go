package playeraccess

import (
	"strings"
	"testing"
)

func TestExactPrefixEmptyAndCaseSensitiveUA(t *testing.T) {
	p := Policy{Enabled: true, Revision: 3, Rules: []Rule{{ID: "prefix", MatchType: "prefix", UserAgent: "SenPlayer/", Enabled: true}, {ID: "exact", MatchType: "exact", UserAgent: "SenPlayer/1.0", Enabled: true}, {ID: "disabled", MatchType: "exact", UserAgent: "VLC/3", Enabled: false}}}
	for _, c := range []struct {
		ua      string
		allowed bool
		rule    string
	}{{"SenPlayer/1.0", true, "exact"}, {"SenPlayer/2.0", true, "prefix"}, {"senplayer/1.0", false, ""}, {"Mozilla/5.0 SenPlayer/1.0", false, ""}, {"", false, ""}, {"VLC/3", false, ""}, {"SenPlayer/1.0\x00", false, ""}} {
		d := p.Decide(c.ua)
		if d.Allowed != c.allowed || d.RuleID != c.rule || d.PolicyRevision != 3 {
			t.Fatal(c, d)
		}
	}
	p.Enabled = false
	if !p.Decide("").Allowed {
		t.Fatal("disabled policy changed existing behavior")
	}
	if p.Decide(strings.Repeat("x", 4097)).Allowed {
		t.Fatal("invalid UA allowed")
	}
	if ObservationID("UA") != ObservationID("UA") || ObservationID("UA") == ObservationID("ua") {
		t.Fatal("UA identity lost")
	}
}
func TestPolicyInputValidation(t *testing.T) {
	for _, p := range []Policy{{Revision: 0}, {Enabled: true, Revision: 1}, {Revision: 1, Rules: []Rule{{MatchType: "regex", UserAgent: ".*"}}}, {Revision: 1, Rules: []Rule{{MatchType: "prefix", UserAgent: "a"}}}, {Revision: 1, Rules: []Rule{{MatchType: "exact", UserAgent: "a\nb"}}}, {Revision: 1, Rules: []Rule{{MatchType: "exact", UserAgent: strings.Repeat("x", 4097)}}}, {Revision: 1, Rules: []Rule{{MatchType: "exact", UserAgent: "UA"}, {MatchType: "exact", UserAgent: "UA"}}}, {Revision: 1, Rules: []Rule{{ID: "invalid", MatchType: "exact", UserAgent: "UA"}}}} {
		if p.Validate() == nil {
			t.Fatal("invalid policy", p)
		}
	}
	p := Policy{Enabled: true, Revision: 1, Rules: []Rule{{MatchType: "exact", UserAgent: " UA ", Enabled: true}}}
	if p.Validate() != nil {
		t.Fatal("valid policy")
	}
	if !ValidUA("中文播放器/1.0") || ValidUA("UA\t") {
		t.Fatal("validation")
	}
}
func TestPlaybackEntryBoundary(t *testing.T) {
	for _, c := range []struct{ path, entry string }{{"/api/v1/cloudStorage/strm302", "strm"}, {"/api/v1/member/libraries/movies/items/asset/playback", "member"}, {"/emby/Items/11/PlaybackInfo", "emby"}, {"/emby/Videos/11/original.mkv", "emby"}, {"/emby/EmBy//videos/11/master.m3u8", "emby"}, {"/emby/Audio/11/universal", "emby"}, {"/emby/Items/11/Download", "emby"}, {"/emby/LiveTv/LiveStream/id/stream.ts", "emby"}, {"/emby/Sync/id/File", "emby"}, {"/emby/Videos/11/Subtitles/0/Stream.srt", ""}, {"/emby/Audio/11/Attachments/a", ""}, {"/emby/Items/11", ""}, {"/emby/Users/AuthenticateByName", ""}, {"/emby/web/index.html", ""}, {"/api/v1/media-server/providers", ""}, {"/watch", ""}, {"/api/v1/player-access/policy", ""}, {"/embyNotMount/Videos/11/stream", ""}} {
		if got := Entry(c.path); got != c.entry {
			t.Fatal(c, got)
		}
	}
}
