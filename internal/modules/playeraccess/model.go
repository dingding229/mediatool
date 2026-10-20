// Package playeraccess owns optional UA-based player admission, not media
// identity or file authorization. User-Agent remains client-controlled.
package playeraccess

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"media-tool/internal/modules/playeraccess/clientinfo"
	uuidcheck "media-tool/internal/platform/identity"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxUABytes = clientinfo.MaxUABytes
const MaxRules = 128
const MaxObservations = 2000

var (
	ErrInvalid     = errors.New("invalid player UA policy")
	ErrConflict    = errors.New("player UA policy revision changed")
	ErrNotFound    = errors.New("player UA observation not found")
	ErrUnavailable = errors.New("player UA policy unavailable")
)

type Rule struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	MatchType string `json:"matchType"`
	UserAgent string `json:"userAgent"`
	Enabled   bool   `json:"enabled"`
}
type Policy struct {
	Enabled   bool      `json:"enabled"`
	Revision  int64     `json:"revision"`
	UpdatedAt time.Time `json:"updatedAt"`
	Rules     []Rule    `json:"rules"`
}
type Decision struct {
	Allowed        bool   `json:"allowed"`
	Reason         string `json:"reason"`
	RuleID         string `json:"ruleId"`
	PolicyRevision int64  `json:"policyRevision"`
}
type Request struct {
	UserAgent, Entry, Method, ClientIP, ConnectionIP, IPSource string
	IPPublic                                                   bool
}
type Observation struct {
	ID                 string    `json:"id"`
	UserAgent          string    `json:"userAgent"`
	FirstSeenAt        time.Time `json:"firstSeenAt"`
	LastSeenAt         time.Time `json:"lastSeenAt"`
	RequestCount       int64     `json:"requestCount"`
	AllowedCount       int64     `json:"allowedCount"`
	DeniedCount        int64     `json:"deniedCount"`
	LastAllowed        bool      `json:"lastAllowed"`
	LastReason         string    `json:"lastReason"`
	LastEntry          string    `json:"lastEntry"`
	LastMethod         string    `json:"lastMethod"`
	LastClientIP       string    `json:"lastClientIp"`
	LastRuleID         string    `json:"lastRuleId"`
	LastPolicyRevision int64     `json:"lastPolicyRevision"`
	Whitelisted        bool      `json:"whitelisted"`
	MatchedRuleID      string    `json:"matchedRuleId"`
}
type Page struct {
	Items          []Observation `json:"items"`
	Total          int           `json:"total"`
	Page           int           `json:"page"`
	PageSize       int           `json:"pageSize"`
	MaxRecords     int           `json:"maxRecords"`
	PolicyRevision int64         `json:"policyRevision"`
}

func ValidUA(ua string) bool         { return clientinfo.ValidUA(ua) }
func ObservationID(ua string) string { d := sha256.Sum256([]byte(ua)); return hex.EncodeToString(d[:]) }
func validID(s string) bool          { return uuidcheck.UUID(s) }
func validObservationID(id string) bool {
	if len(id) != 64 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil && strings.ToLower(id) == id
}
func (p Policy) Validate() error {
	if p.Revision < 1 || len(p.Rules) > MaxRules {
		return ErrInvalid
	}
	seen := map[string]bool{}
	ids := map[string]bool{}
	active := 0
	for _, r := range p.Rules {
		if !ValidUA(r.UserAgent) || strings.TrimSpace(r.UserAgent) == "" || r.MatchType != "exact" && r.MatchType != "prefix" || r.MatchType == "prefix" && len(r.UserAgent) < 3 || !utf8.ValidString(r.Label) || utf8.RuneCountInString(r.Label) > 80 || strings.ContainsAny(r.Label, "\x00\r\n") {
			return ErrInvalid
		}
		if r.ID != "" && (!validID(r.ID) || ids[r.ID]) {
			return ErrInvalid
		}
		ids[r.ID] = r.ID != ""
		key := r.MatchType + ":" + r.UserAgent
		if seen[key] {
			return ErrInvalid
		}
		seen[key] = true
		if r.Enabled {
			active++
		}
	}
	if p.Enabled && active == 0 {
		return errors.New("启用播放器限制前，请先添加至少一条启用的UA白名单规则")
	}
	return nil
}
func Match(rules []Rule, ua string) string {
	if ua == "" || !ValidUA(ua) {
		return ""
	}
	// Prefer exact identities over broader prefixes; stable ordering makes the
	// recorded rule deterministic without changing the allow result.
	for _, kind := range []string{"exact", "prefix"} {
		for _, r := range rules {
			if r.Enabled && r.MatchType == kind && (kind == "exact" && ua == r.UserAgent || kind == "prefix" && strings.HasPrefix(ua, r.UserAgent)) {
				return r.ID
			}
		}
	}
	return ""
}
func (p Policy) Decide(ua string) Decision {
	d := Decision{PolicyRevision: p.Revision}
	if !ValidUA(ua) {
		d.Reason = "invalid_ua"
		return d
	}
	d.RuleID = Match(p.Rules, ua)
	if !p.Enabled {
		d.Allowed = true
		d.Reason = "restriction_disabled"
	} else if d.RuleID != "" {
		d.Allowed = true
		d.Reason = "whitelisted"
	} else {
		d.Reason = "ua_not_whitelisted"
	}
	return d
}
