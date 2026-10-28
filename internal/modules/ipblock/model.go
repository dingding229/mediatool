// Package ipblock owns media-only IP admission. A match never grants media access.
package ipblock

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxRules = 10000

var (
	ErrCommitUnknown = errors.New("IP 黑名单规则提交结果待核对")
	ErrUnavailable   = errors.New("IP 黑名单策略暂不可用，媒体请求未放行；正在自动重试")
	ErrConflict      = errors.New("IP 黑名单规则已修改，请刷新后重试")
	ErrNotFound      = errors.New("IP 黑名单规则不存在，请刷新后重试")
	ErrLimit         = errors.New("IP 黑名单规则最多 10000 条，请先删除不需要的规则")
	ErrDuplicate     = errors.New("该 IP 或网段已在 IP 黑名单中")
)

// CoveredError identifies a whole-prefix duplicate, not a partial overlap.
type CoveredError struct{ Network string }

func (e CoveredError) Error() string { return fmt.Sprintf("已在 %s 中", e.Network) }
func (e CoveredError) Unwrap() error { return ErrDuplicate }

type Rule struct {
	ID        string     `json:"id"`
	Network   string     `json:"network"`
	Note      string     `json:"note"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	HitCount  int64      `json:"hitCount"`
	LastHitAt *time.Time `json:"lastHitAt"`
}
type Policy struct {
	Revision int64  `json:"revision"`
	Rules    []Rule `json:"-"`
}
type Mutation struct {
	ConfirmSelf bool   `json:"confirmSelf"`
	ID          string `json:"-"`
	Network     string `json:"network"`
	Note        string `json:"note"`
	Revision    int64  `json:"revision"`
	Delete      bool   `json:"-"`
	Confirm     bool   `json:"confirm"`
}
type Match struct {
	Matched  bool   `json:"matched"`
	RuleID   string `json:"ruleId,omitempty"`
	Network  string `json:"network,omitempty"`
	Revision int64  `json:"revision"`
}
type Status struct {
	Available      bool   `json:"available"`
	Message        string `json:"message,omitempty"`
	RetrySeconds   int64  `json:"retrySeconds"`
	CounterWarning bool   `json:"counterWarning"`
}
type Delta struct {
	Count int64
	Last  time.Time
}

func Network(raw string) (netip.Prefix, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) > 128 {
		return netip.Prefix{}, errors.New("IP 或网段不能超过 128 字符")
	}
	if a, e := netip.ParseAddr(raw); e == nil && a.Zone() == "" {
		a = a.Unmap()
		if !a.IsUnspecified() && !a.IsMulticast() {
			bits := 64
			if a.Is4() {
				bits = 24
			}
			return netip.PrefixFrom(a, bits).Masked(), nil
		}
	}
	p, e := netip.ParsePrefix(raw)
	if e != nil || p.Bits() == 0 || p.Addr().Is4In6() || p.Addr().IsMulticast() {
		return netip.Prefix{}, errors.New("请填写有效 IP 或 CIDR 网段，不允许 /0、组播或带 zone 的地址")
	}
	return p.Masked(), nil
}
func (m *Mutation) Validate() error {
	if m.Revision < 1 {
		return ErrConflict
	}
	if !m.Confirm {
		return errors.New("请先确认 IP 黑名单操作")
	}
	if m.Delete {
		return nil
	}
	p, e := Network(m.Network)
	if e != nil {
		return e
	}
	m.Network = p.String()
	m.Note = strings.TrimSpace(m.Note)
	if strings.ContainsRune(m.Note, 0) {
		return errors.New("备注不能包含空字符")
	}
	if !utf8.ValidString(m.Note) || utf8.RuneCountInString(m.Note) > 500 {
		return errors.New("备注不能超过 500 个字符")
	}
	return nil
}
