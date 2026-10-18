package sites

import (
	"context"
	"html"
	"io"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ControlledRequest struct {
	Origin, Path, Method, Body, SuccessMarker string
	Raw                                       bool
}
type ControlledResponse struct {
	Status  int
	State   string
	Matched bool
}

// Controlled binds every credential to the administrator-configured account's
// exact origin. It has no caller headers, redirect following, cookie jar, raw
// response or auth-material output. Challenges require operator action.
func (c Client) Controlled(ctx context.Context, in ControlledRequest) (ControlledResponse, error) {
	out := ControlledResponse{State: "unknown"}
	base, err := url.Parse(c.Account.Endpoint)
	origin, oe := url.Parse(in.Origin)
	rel, re := url.Parse(in.Path)
	if err != nil || oe != nil || re != nil || base.User != nil || origin.User != nil || origin.Path != "" && origin.Path != "/" || origin.RawQuery != "" || origin.Fragment != "" || !sameRequestOrigin(base, origin) || rel.IsAbs() || rel.Host != "" || rel.User != nil || rel.Fragment != "" || !strings.HasPrefix(in.Path, "/") || strings.HasPrefix(in.Path, "//") || strings.ContainsAny(in.Path, "\\\x00\r\n") || len(in.Path) > 2048 || len(in.Body) > 16384 || len(in.SuccessMarker) > 200 || in.Raw {
		return out, ErrConfig
	}
	if in.Method != "GET" && in.Method != "POST" || in.Method == "GET" && in.Body != "" {
		return out, ErrConfig
	}
	target := base.ResolveReference(rel)
	if !sameRequestOrigin(base, target) {
		return out, ErrConfig
	}
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(bounded, in.Method, target.String(), strings.NewReader(in.Body))
	if err != nil {
		return out, ErrRequest
	}
	headers, err := normalizedAccountHeaders(c.Account.RequestHeaders)
	if err != nil {
		return out, ErrConfig
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if c.Account.Cookie != "" {
		req.Header.Set("Cookie", c.Account.Cookie)
	}
	if c.Account.APIKey != "" {
		req.Header.Set("X-Api-Key", c.Account.APIKey)
	}
	req.Header.Set("User-Agent", c.Account.UserAgent)
	if in.Method == "POST" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	copy := *client
	copy.Timeout = 15 * time.Second
	copy.Jar = nil
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := copy.Do(req)
	if err != nil {
		return out, ErrRequest
	}
	defer deferutil.Close(response.Body, "Bounded controlled-site response descriptor")
	out.Status = response.StatusCode
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return out, ErrRequest
	}
	body := html.UnescapeString(string(raw))
	secrets := []string{c.Account.Cookie, c.Account.APIKey, c.Account.Passkey}
	for _, part := range strings.Split(c.Account.Cookie, ";") {
		if _, value, ok := strings.Cut(part, "="); ok {
			secrets = append(secrets, strings.TrimSpace(value))
		}
	}
	for _, v := range c.Account.RequestHeaders {
		secrets = append(secrets, v)
		if _, token, ok := strings.Cut(v, " "); ok {
			secrets = append(secrets, token)
		}
	}
	for _, secret := range secrets {
		if secret != "" {
			body = strings.ReplaceAll(body, secret, "[redacted]")
			body = strings.ReplaceAll(body, url.QueryEscape(secret), "[redacted]")
		}
	}
	lower := strings.ToLower(body)
	if response.StatusCode == 401 || response.StatusCode == 403 || strings.Contains(lower, "captcha") || strings.Contains(lower, "cf-chl-") {
		out.State = "needs_action"
		return out, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return out, nil
	}
	if in.SuccessMarker != "" {
		out.Matched = strings.Contains(body, in.SuccessMarker)
		if out.Matched {
			out.State = "matched"
		}
	}
	return out, nil
}
