package httpapi

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"media-tool/internal/modules/emby"
	"media-tool/internal/modules/ipblock"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/settings"
	"media-tool/internal/modules/strm"
	"media-tool/internal/platform/clientip"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

type ipEvidenceKey struct{}
type ipRequestIDKey struct{}
type ipEvidence struct {
	address clientip.Address
	proxies []string
	err     error
}

func publicEvidence(a clientip.Address) clientip.Address {
	if !a.Public {
		a.ClientIP = ""
	}
	a.ConnectionIP = clientip.Sanitize(a.ConnectionIP)
	return a
}
func newIPBlock(db *pgxpool.Pool, log *slog.Logger) *ipblock.Service {
	s := ipblock.New(ipblock.Repository{DB: db}, log)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = s.Reload(ctx)
	return s
}
func mediaIPScope(r *http.Request) bool {
	p := r.URL.Path
	return p == strm.Endpoint || strings.HasPrefix(p, "/emby/") || p == "/emby" && (emby.IsRootOrigin(r.Context()) || r.Method != "GET" && r.Method != "HEAD")
}
func checkMediaIP(ctx context.Context, s *ipblock.Service, db *pgxpool.Pool, r *http.Request) (bool, error) {
	if s == nil {
		return true, nil
	} // Explicit no-database development/unit harness only; production always supplies the service.
	if !s.Status().Available {
		return false, ipblock.ErrUnavailable
	}
	evidence, ok := r.Context().Value(ipEvidenceKey{}).(ipEvidence)
	if !ok {
		c, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		prefs, e := (settings.Repository{DB: db}).Get(c)
		evidence = ipEvidence{proxies: prefs.Value.TrustedProxyCIDRs, err: e}
		evidence.address = clientip.AdmissionAddress(r, evidence.proxies)
		*r = *r.WithContext(context.WithValue(r.Context(), ipEvidenceKey{}, evidence))
	}
	if evidence.err != nil {
		return false, ipblock.ErrUnavailable
	}
	ip, e := netip.ParseAddr(evidence.address.ClientIP)
	if e != nil {
		return false, ipblock.ErrUnavailable
	}
	id, _ := r.Context().Value(ipRequestIDKey{}).(string)
	if id == "" {
		id = jobs.UUID()
	}
	entry := "native"
	if r.URL.Path == strm.Endpoint {
		entry = "strm"
	} else if strings.HasPrefix(r.URL.Path, "/emby") {
		entry = "emby"
	}
	m, e := s.Evaluate(ip, true, "requestId", id, "entry", entry, "path", r.URL.Path, "method", r.Method, "ipSource", evidence.address.Source)
	return !m.Matched, e
}
func (a *API) admitIP(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	*r = *r.WithContext(context.WithValue(r.Context(), ipRequestIDKey{}, w.Header().Get("X-Request-ID")))
	ok, e := checkMediaIP(r.Context(), a.Deps.IPBlock, a.Deps.DB, r)
	if e != nil {
		fail(w, 503, "player_ip_unavailable", ipblock.ErrUnavailable.Error())
		return false
	}
	if !ok {
		w.Header().Set("Cache-Control", "no-store")
		fail(w, 403, "player_ip_denied", "当前 IP 已被拉黑，请联系管理员")
	}
	return ok
}
func ipBlockError(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, ipblock.ErrConflict):
		fail(w, 409, "player_ip_conflict", e.Error())
	case errors.Is(e, ipblock.ErrNotFound):
		fail(w, 404, "player_ip_not_found", e.Error())
	case errors.Is(e, ipblock.ErrUnavailable):
		fail(w, 503, "player_ip_unavailable", e.Error())
	default:
		fail(w, 400, "invalid_player_ip", e.Error())
	}
}

type ipBlockPage struct {
	Items     []ipblock.Rule `json:"items"`
	Total     int            `json:"total"`
	Revision  int64          `json:"revision"`
	Status    ipblock.Status `json:"status"`
	CurrentIP string         `json:"currentIp"`
	MaxRules  int            `json:"maxRules"`
}

func (a *API) listIPBlock(w http.ResponseWriter, r *http.Request) {
	out := ipBlockPage{Items: []ipblock.Rule{}, MaxRules: ipblock.MaxRules, Status: a.Deps.IPBlock.Status()}
	if a.Deps.IPBlock == nil {
		write(w, 200, out)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	p, e := a.Deps.IPBlock.Policy(ctx)
	if e != nil {
		if out.Status.Available {
			fail(w, 503, "player_ip_list_unavailable", "暂时无法读取 IP 拉黑列表；已加载的阻断规则仍然生效")
			return
		}
		write(w, 200, out)
		return
	}
	out.Revision = p.Revision
	prefs, e := (settings.Repository{DB: a.Deps.DB}).Get(ctx)
	if e == nil {
		out.CurrentIP = clientip.AdmissionAddress(r, prefs.Value.TrustedProxyCIDRs).ClientIP
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	paging, e := pagination(r)
	if e != nil {
		fail(w, 400, "invalid_page", "分页无效")
		return
	}
	start, limit := paging.Offset, paging.Limit
	for _, row := range p.Rules {
		if q != "" && !strings.Contains(strings.ToLower(row.Network+" "+row.Note), q) {
			continue
		}
		if out.Total >= start && len(out.Items) < limit {
			out.Items = append(out.Items, row)
		}
		out.Total++
	}
	write(w, 200, out)
}
func (a *API) changeIPBlock(w http.ResponseWriter, r *http.Request) {
	var m ipblock.Mutation
	if !decodeLimit(w, r, &m, 8192) {
		return
	}
	m.ID = r.PathValue("id")
	m.Delete = r.Method == http.MethodDelete
	if m.ID != "" && !isWorkbenchUUID(m.ID) {
		fail(w, 400, "invalid_player_ip", "规则 ID 无效")
		return
	}
	if a.Deps.IPBlock == nil {
		ipBlockError(w, ipblock.ErrUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if e := m.Validate(); e != nil {
		ipBlockError(w, e)
		return
	}
	if !m.Delete {
		pref, e := (settings.Repository{DB: a.Deps.DB}).Get(ctx)
		if e != nil {
			ipBlockError(w, ipblock.ErrUnavailable)
			return
		}
		current := clientip.AdmissionAddress(r, pref.Value.TrustedProxyCIDRs)
		addr, e := netip.ParseAddr(current.ClientIP)
		network, _ := ipblock.Network(m.Network)
		if e == nil && network.Contains(addr.Unmap()) && !m.ConfirmSelf {
			fail(w, 400, "player_ip_self_confirmation", "该规则覆盖当前管理员访问 IP；管理后台仍可解封，请再次明确确认")
			return
		}
	}
	p, e := a.Deps.IPBlock.Change(ctx, m)
	if e != nil {
		ipBlockError(w, e)
		return
	}
	write(w, 200, map[string]int64{"revision": p.Revision})
}
func (a *API) checkIPBlock(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Address string `json:"address"`
		Network string `json:"network"`
	}
	if !decodeLimit(w, r, &in, 1024) {
		return
	}
	ip, e := netip.ParseAddr(strings.TrimSpace(in.Address))
	if e != nil || ip.Zone() != "" {
		fail(w, 400, "invalid_player_ip", "请填写有效的 IPv4 或 IPv6 地址")
		return
	}
	m, e := a.Deps.IPBlock.Evaluate(ip, false)
	if e != nil {
		ipBlockError(w, e)
		return
	}
	out := struct {
		Matched          bool   `json:"matched"`
		RuleID           string `json:"ruleId,omitempty"`
		Network          string `json:"network,omitempty"`
		Revision         int64  `json:"revision"`
		CandidateNetwork string `json:"candidateNetwork,omitempty"`
		AffectsCurrent   bool   `json:"affectsCurrent"`
	}{Matched: m.Matched, RuleID: m.RuleID, Network: m.Network, Revision: m.Revision}
	if in.Network != "" {
		p, e := ipblock.Network(in.Network)
		if e != nil {
			ipBlockError(w, e)
			return
		}
		out.CandidateNetwork = p.String()
		prefs, e := (settings.Repository{DB: a.Deps.DB}).Get(r.Context())
		if e != nil {
			ipBlockError(w, ipblock.ErrUnavailable)
			return
		}
		addr, e := netip.ParseAddr(clientip.AdmissionAddress(r, prefs.Value.TrustedProxyCIDRs).ClientIP)
		out.AffectsCurrent = e == nil && p.Contains(addr.Unmap())
	}
	write(w, 200, out)
}
func (a *API) registerIPBlock(m routeRegistrar) {
	m.HandleFunc("GET /api/v1/player-access/ip-blocklist", a.protected(a.listIPBlock))
	m.HandleFunc("POST /api/v1/player-access/ip-blocklist", a.protected(a.changeIPBlock))
	m.HandleFunc("PUT /api/v1/player-access/ip-blocklist/{id}", a.protected(a.changeIPBlock))
	m.HandleFunc("DELETE /api/v1/player-access/ip-blocklist/{id}", a.protected(a.changeIPBlock))
	m.HandleFunc("POST /api/v1/player-access/ip-blocklist/check", a.protected(a.checkIPBlock))
}
