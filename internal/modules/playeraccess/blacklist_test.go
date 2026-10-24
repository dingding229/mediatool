package playeraccess

import "testing"

func TestThreeModesKeepIndependentExactPrefixContainsRules(t *testing.T) {
	for _, kind := range []string{"exact", "prefix", "contains"} {
		t.Run(kind, func(t *testing.T) {
			pattern := "OwnedPlayer"
			matched := "OwnedPlayer"
			if kind == "prefix" {
				matched += "/2"
			}
			if kind == "contains" {
				matched = "Client OwnedPlayer/2"
			}
			p := Policy{Revision: 9, Rules: []Rule{{ID: "white", MatchType: kind, UserAgent: pattern, Enabled: true}}, BlacklistRules: []Rule{{ID: "black", MatchType: kind, UserAgent: pattern, Enabled: true}}}
			for _, mode := range []string{"off", "whitelist", "blacklist"} {
				p.Mode = mode
				for _, ua := range []string{matched, "OtherPlayer", "", "ownedplayer"} {
					d := p.Decide(ua)
					match := ua == matched
					want := mode == "off" || mode == "whitelist" && match || mode == "blacklist" && !match
					if d.Allowed != want || d.PolicyRevision != 9 {
						t.Fatalf("%s %q: %+v", mode, ua, d)
					}
					if mode == "blacklist" && match && (d.Reason != "blacklisted" || d.RuleID != "black") {
						t.Fatal(d)
					}
				}
				if p.Decide("UA\nscript").Allowed {
					t.Fatal("invalid UA bypassed validation")
				}
			}
			p.Mode = "blacklist"
			p.BlacklistRules[0].Enabled = false
			if !p.Decide(matched).Allowed {
				t.Fatal("disabled rule matched")
			}
			if len(p.Rules) != 1 || len(p.BlacklistRules) != 1 {
				t.Fatal("switch lost rules")
			}
		})
	}
}
func TestBlacklistValidationAndDeterministicPriority(t *testing.T) {
	p := Policy{Revision: 1, Mode: "blacklist"}
	if p.Validate() != nil {
		t.Fatal("empty blacklist should allow all")
	}
	p.Mode = "invalid"
	if p.Validate() == nil {
		t.Fatal("unknown mode")
	}
	p.Mode = "blacklist"
	p.BlacklistRules = []Rule{{MatchType: "contains", UserAgent: "ab", Enabled: true}}
	if p.Validate() == nil {
		t.Fatal("short contains")
	}
	p.BlacklistRules[0].UserAgent = "Player"
	if p.Validate() != nil {
		t.Fatal("valid contains")
	}
	p.Rules = append([]Rule{}, p.BlacklistRules...)
	if p.Validate() != nil {
		t.Fatal("same pattern in two lists is valid")
	}
	rules := []Rule{{ID: "contains", Enabled: true, MatchType: "contains", UserAgent: "Player"}, {ID: "prefix", Enabled: true, MatchType: "prefix", UserAgent: "Player"}, {ID: "exact", Enabled: true, MatchType: "exact", UserAgent: "Player/1"}}
	if Match(rules, "Player/1") != "exact" || Match(rules, "Player/2") != "prefix" || Match(rules, "Client Player/1") != "contains" {
		t.Fatal("priority unstable")
	}
}
