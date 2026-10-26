package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/settings"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (a *API) access(ctx context.Context) settings.Access {
	v := settings.Access{ManagementURL: a.Config.PublicURL, PlaybackURL: a.Config.PublicURL, MemberURL: a.Config.MemberPublicURL}
	if v.MemberURL == "" && a.Config.MemberAddress != "" {
		u, _ := url.Parse(a.Config.PublicURL)
		if u != nil {
			_, port, _ := net.SplitHostPort(a.Config.MemberAddress)
			u.Host = net.JoinHostPort(u.Hostname(), port)
			v.MemberURL = u.String()
		}
	}
	r, err := (settings.Repository{DB: a.Deps.DB}).Get(ctx)
	if err != nil {
		return v
	}
	c := r.Value.Access
	if c.ManagementURL != "" {
		v.ManagementURL = c.ManagementURL
	}
	if c.PlaybackURL != "" {
		v.PlaybackURL = c.PlaybackURL
	}
	if c.MemberURL != "" {
		v.MemberURL = c.MemberURL
	}

	return v
}
func (a *API) sameOrigin(r *http.Request) bool {
	// Explicit settings augment the fixed deployment origin. No request Host or
	// forwarding header is treated as authority, preventing arbitrary CSRF origins.
	check := a.Auth
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	v := a.access(ctx)
	origin := v.ManagementURL
	check.AllowedOrigins = append(append([]string{}, check.AllowedOrigins...), origin, settings.OriginKey(origin))
	return check.SameOrigin(r)
}
func (a *API) accessSettings(w http.ResponseWriter, r *http.Request) {
	repo := settings.Repository{DB: a.Deps.DB}
	if r.Method == http.MethodPut {
		var in struct {
			Value    settings.Access `json:"value"`
			Revision int64           `json:"revision"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := settings.ValidateAccess(in.Value); err != nil {
			fail(w, 400, "invalid_domain", err.Error())
			return
		}
		if !a.Config.Development() {
			for _, v := range []string{in.Value.ManagementURL, in.Value.PlaybackURL, in.Value.MemberURL} {
				if v != "" && !strings.HasPrefix(v, "https://") {
					fail(w, 400, "https_required", "生产环境域名必须使用HTTPS")
					return
				}
			}
		}
		current, err := repo.Get(r.Context())
		if err != nil {
			fail(w, 503, "settings_unavailable", "设置不可用")
			return
		}
		if current.Revision != in.Revision {
			fail(w, 409, "settings_changed", "设置已变更，请重新读取")
			return
		}
		current.Value.Access = in.Value
		if _, err = repo.Save(r.Context(), current.Value, current.Revision); err != nil {
			if errors.Is(err, settings.ErrRevision) {
				fail(w, 409, "settings_changed", "设置已变更，请重新读取")
			} else {
				fail(w, 400, "invalid_settings", err.Error())
			}
			return
		}
		a.Deps.Logger.Info("public domain settings updated", "managementUrl", in.Value.ManagementURL, "playbackUrl", in.Value.PlaybackURL, "memberUrl", in.Value.MemberURL)
	}
	current, err := repo.Get(r.Context())
	if err != nil {
		fail(w, 503, "settings_unavailable", "设置不可用")
		return
	}
	write(w, 200, map[string]any{"value": current.Value.Access, "revision": current.Revision, "effective": a.access(r.Context()), "listeners": map[string]string{"management": a.Config.Address, "member": a.Config.MemberAddress}})
}
