package sites

import (
	"context"
	"encoding/json"
	"html"
	"io"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type ControlledRequest struct {
	Origin, Path, Method, Body, SuccessMarker string
	MaxResponseBytes                          int
	Raw                                       bool
}
type ControlledResponse struct {
	Status    int
	State     string
	Matched   bool
	Body      string
	Headers   map[string]string
	Truncated bool
	Encoding  string
}

// Controlled binds every credential to the administrator-configured account's
// exact origin. It has no caller headers, redirect following, cookie jar or auth-material output. The UTF-8 response is bounded and redacted;
// binary bodies are rejected. Challenges require operator action.
func (c Client) Controlled(ctx context.Context, in ControlledRequest) (ControlledResponse, error) {
	out := ControlledResponse{State: "unknown", Headers: map[string]string{}, Encoding: "utf-8"}
	limit := 256 << 10
	if in.MaxResponseBytes < 0 || in.MaxResponseBytes > limit {
		return out, ErrConfig
	}
	if in.MaxResponseBytes > 0 {
		limit = in.MaxResponseBytes
	}
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
	raw, err := io.ReadAll(io.LimitReader(response.Body, int64(limit+1)))
	if err != nil {
		return out, ErrRequest
	}
	out.Truncated = len(raw) > limit
	if out.Truncated {
		raw = raw[:limit]
		for len(raw) > 0 && !utf8.Valid(raw) && len(raw) > limit-4 {
			raw = raw[:len(raw)-1]
		}
	}
	if !utf8.Valid(raw) || strings.ContainsFunc(string(raw), func(r rune) bool { return unicode.IsControl(r) && r != '\t' && r != '\r' && r != '\n' }) {
		return out, ErrRequest
	}
	ct := strings.ToLower(strings.Split(response.Header.Get("Content-Type"), ";")[0])
	if ct != "" && !strings.HasPrefix(ct, "text/") && ct != "application/json" && !strings.HasSuffix(ct, "+json") && ct != "application/xml" && !strings.HasSuffix(ct, "+xml") {
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
			if out.Truncated {
				for _, variant := range []string{secret, url.QueryEscape(secret)} {
					for n := min(len(variant)-1, len(body)); n > 0; n-- {
						if strings.HasSuffix(body, variant[:n]) {
							body = body[:len(body)-n] + "[redacted]"
							break
						}
					}
				}
			}
		}
	}
	out.Body = body
	if len(out.Body) > limit {
		out.Body = out.Body[:limit]
		for !utf8.ValidString(out.Body) {
			out.Body = out.Body[:len(out.Body)-1]
		}
		out.Truncated = true
	}
	// Never expose Set-Cookie, Location, authorization or arbitrary custom headers.
	for _, k := range []string{"Content-Type", "Content-Language", "Retry-After"} {
		if v := response.Header.Get(k); len(v) <= 512 && v != "" {
			safe := true
			for _, secret := range secrets {
				if secret != "" && (strings.Contains(v, secret) || strings.Contains(v, url.QueryEscape(secret))) {
					safe = false
				}
			}
			if safe {
				out.Headers[k] = v
			}
		}
	}

	// JSON escaping can expand text by 6x. Keep the entire typed response below
	// the existing encrypted business receipt budget, without broadening KV quotas.
	for {
		encoded, e := json.Marshal(out)
		if e != nil {
			return out, ErrRequest
		}
		if len(encoded) <= 80<<10 {
			break
		}
		n := len(out.Body) * 3 / 4
		out.Body = out.Body[:n]
		for !utf8.ValidString(out.Body) {
			out.Body = out.Body[:len(out.Body)-1]
		}
		out.Truncated = true
	}

	lower := strings.ToLower(out.Body)
	if response.StatusCode == 401 || response.StatusCode == 403 || strings.Contains(lower, "captcha") || strings.Contains(lower, "cf-chl-") {
		out.State = "needs_action"
		return out, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return out, nil
	}
	if in.SuccessMarker != "" {
		out.Matched = strings.Contains(out.Body, in.SuccessMarker)
		if out.Matched {
			out.State = "matched"
		}
	}
	return out, nil
}
