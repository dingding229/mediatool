package operator

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"media-tool/internal/modules/playback"
	"media-tool/internal/platform/envfile"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

var ErrNeedsConfiguration = errors.New("needs_configuration; no playback performed")
var ErrVerificationFailed = errors.New("strict 302 verification failed; raw URLs and credentials omitted")

func ExitStatus(err error) int {
	if errors.Is(err, ErrNeedsConfiguration) || errors.Is(err, ErrVerificationFailed) {
		return 2
	}
	return 1
}

func origin(raw string, auth bool) (string, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("HTTP(S) origin required")
	}
	if auth && u.Scheme == "http" && !slices.Contains([]string{"localhost", "127.0.0.1", "::1"}, u.Hostname()) {
		return "", errors.New("HTTPS required outside loopback")
	}
	return strings.TrimSuffix(u.String(), "/"), nil
}
func noRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
func reportFile(root, path string, v any) error {
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	if e := Atomic(path, jsonBytes(v)); e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(v)
}

type diagnostic struct {
	HTTP, CDN *http.Client
	Agent     string
}

func (d diagnostic) response(ctx context.Context, raw, method string, body any, h http.Header) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(jsonBytes(body))
	}
	r, e := http.NewRequestWithContext(ctx, method, raw, reader)
	if e != nil {
		return nil, errors.New("invalid owned request")
	}
	r.Header = h.Clone()
	if r.Header == nil {
		r.Header = make(http.Header)
	}
	r.Header.Set("User-Agent", d.Agent)
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	return d.HTTP.Do(r)
}
func (d diagnostic) request(ctx context.Context, raw, method string, body any, h http.Header) (int, http.Header, []byte, error) {
	res, e := d.response(ctx, raw, method, body, h)
	if e != nil {
		return 0, nil, nil, errors.New("owned diagnostic request failed")
	}
	defer func() { _ = res.Body.Close() }()
	b, e := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if len(b) > 1<<20 {
		return 0, nil, nil, errors.New("diagnostic response too large")
	}
	return res.StatusCode, res.Header, b, e
}
func (d diagnostic) api(ctx context.Context, base, path, method string, body any, h http.Header) (map[string]any, bool) {
	status, _, b, e := d.request(ctx, base+"/api/v1"+path, method, body, h)
	var v struct{ Data map[string]any }
	if e != nil || status != 200 || json.Unmarshal(b, &v) != nil {
		return nil, false
	}
	return v.Data, true
}

// Resolver must emit 302, never relay. CDN gets only UA/Range, never cookies or
// user/admin tokens; public DNS is checked again at the pinned transport dial.
func (d diagnostic) resolver(ctx context.Context, raw, method string, h http.Header) (int, http.Header, error) {
	res, e := d.response(ctx, raw, method, nil, h)
	if e != nil {
		return 0, nil, errors.New("resolver unavailable")
	}
	defer func() { _ = res.Body.Close() }()
	return res.StatusCode, res.Header, nil
}
func (d diagnostic) redirect(ctx context.Context, raw string, h http.Header, emby bool) bool {
	status, headers, e := d.resolver(ctx, raw, "HEAD", h)
	if e != nil || status != 302 || (emby && headers.Get("X-Media-Gateway") != "emby-static") {
		return false
	}
	status, headers, e = d.resolver(ctx, raw, "GET", h)
	if e != nil || status != 302 {
		return false
	}
	target, e := url.Parse(raw)
	if e != nil {
		return false
	}
	for range 5 {
		next, e := target.Parse(headers.Get("Location"))
		if e != nil || headers.Get("Location") == "" {
			return false
		}
		target = next
		r, e := http.NewRequestWithContext(ctx, "GET", target.String(), nil)
		if e != nil {
			return false
		}
		r.Header.Set("User-Agent", d.Agent)
		r.Header.Set("Range", "bytes=0-1023")
		res, e := d.CDN.Do(r)
		if e != nil {
			return false
		}
		b, e := io.ReadAll(io.LimitReader(res.Body, 1024))
		ce := res.Body.Close()
		if e != nil || ce != nil {
			return false
		}
		headers = res.Header
		if slices.Contains([]int{301, 302, 303, 307, 308}, res.StatusCode) {
			continue
		}
		content := strings.Split(res.Header.Get("Content-Type"), ";")[0]
		return res.StatusCode == 206 && len(b) > 0 && (strings.HasPrefix(content, "video/") || strings.HasPrefix(content, "audio/") || content == "application/octet-stream")
	}
	return false
}
func playbackCheck(root string, args []string) error {
	f := flag.NewFlagSet("playback-check", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	base := f.String("base-url", "http://localhost:8080", "owned origin")
	gateway := f.String("gateway-url", "", "owned Emby gateway origin")
	user := f.String("username", "", "registered administrator")
	passwordFile := f.String("password-file", "", "private password file")
	asset := f.String("asset-id", "", "existing indexed asset")
	item := f.String("item-id", "", "existing item")
	path := f.String("path", "", "existing cloud path")
	agent := f.String("user-agent", "MediaTool-DirectCheck/1.0", "actual player UA")
	report := f.String("report", "data/diagnostics/playback-check.json", "private report")
	if e := f.Parse(args); e != nil {
		return e
	}
	if f.NArg() != 0 {
		return errors.New("unexpected diagnostic arguments")
	}
	values, _ := envfile.Read(filepath.Join(root, ".env"))
	if values == nil {
		values = map[string]string{}
	}
	get := func(k string) string {
		if v, ok := os.LookupEnv(k); ok {
			return v
		}
		return values[k]
	}
	fallback := func(v *string, key string) {
		if *v == "" {
			*v = get(key)
		}
	}
	fallback(user, "ADMIN_USERNAME")
	fallback(passwordFile, "ADMIN_PASSWORD_FILE")
	fallback(asset, "M0_ASSET_ID")
	fallback(item, "M0_EMBY_ITEM_ID")
	fallback(path, "M0_MEDIA_PATH")
	b, e := origin(*base, true)
	if e != nil {
		return e
	}
	g := b + "/emby"
	if *gateway != "" {
		g, e = origin(*gateway, true)
		if e != nil {
			return e
		}
	}
	if strings.ContainsAny(*agent, "\r\n\x00") || len(*agent) > 512 {
		return errors.New("invalid player UA")
	}
	reportValue := map[string]any{"checkedAt": time.Now().UTC(), "status": "needs_configuration", "serverChainVerified": false, "realPlayerVerified": false}
	checks := []map[string]any{}
	record := func(n string, ok bool) { checks = append(checks, map[string]any{"name": n, "passed": ok}) }
	password := ""
	if *passwordFile != "" {
		st, e := os.Lstat(*passwordFile)
		if e != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
			return errors.New("password file must be private regular file")
		}
		raw, e := os.ReadFile(*passwordFile)
		if e != nil {
			return errors.New("password file unavailable")
		}
		password = strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	} else if *user != "" {
		password, e = promptPassword()
		if e != nil {
			return e
		}
	}
	if *user == "" || password == "" || *item == "" || get("M0_EMBY_USER_TOKEN") == "" || (*asset == "" && (*path == "" || get("STRM_API_KEY") == "")) {
		reportValue["checks"] = checks
		if e := reportFile(root, *report, reportValue); e != nil {
			return e
		}
		return ErrNeedsConfiguration
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: noRedirect}
	policy := playback.Policy{}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = policy.DialContext
	defer transport.CloseIdleConnections()
	cdn := &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	d := diagnostic{HTTP: client, CDN: cdn, Agent: *agent}
	// Validate every CDN target before its connection, including redirect hops.
	cdn.Transport = checkedTransport{base: transport, policy: policy}
	status, headers, _, err := d.request(ctx, b+"/api/v1/auth/login", "POST", map[string]string{"username": *user, "password": password}, http.Header{"X-Requested-With": []string{"media-tool"}})
	password = ""
	h := http.Header{"X-Requested-With": []string{"media-tool"}}
	if err == nil && status == 200 {
		for _, c := range (&http.Response{Header: headers}).Cookies() {
			if c.Name == "media_tool_session" {
				h.Set("Cookie", c.Name+"="+c.Value)
			}
		}
	}
	logged := h.Get("Cookie") != ""
	record("administrator_login", logged)
	if logged {
		defer func() {
			_, _, _, _ = d.request(context.WithoutCancel(ctx), b+"/api/v1/auth/logout", "POST", map[string]any{}, h)
		}()
		pre, ok := d.api(ctx, b, "/diagnostics/playback/preflight", "GET", nil, h)
		record("runtime_preflight", ok && pre["cloudConfigured"] == true && pre["embyConfigured"] == true && pre["gatewayEnabled"] == true)
		stable := ""
		if *asset != "" {
			status, _, raw, e := d.request(ctx, b+"/api/v1/media/assets/"+url.PathEscape(*asset)+"/strm", "GET", nil, h)
			u, parseErr := url.Parse(strings.TrimSpace(string(raw)))
			if e == nil && status == 200 && len(raw) <= 8192 && parseErr == nil && u.User == nil && u.Fragment == "" && u.Path == "/api/v1/cloudStorage/strm302" && u.Query().Get("assetId") == *asset && singleQuery(u.Query()) {
				stable = b + u.RequestURI()
			}
			ref, ok := d.api(ctx, b, "/media/assets/"+url.PathEscape(*asset)+"/validation-ref", "GET", nil, h)
			if ok {
				*path, _ = ref["path"].(string)
			}
		} else {
			stable = b + "/api/v1/cloudStorage/strm302?" + url.Values{"apiKey": []string{get("STRM_API_KEY")}, "path": []string{*path}}.Encode()
		}
		record("managed_strm", stable != "" && *path != "")
		cloud, ok := d.api(ctx, b, "/cloudStorage/test", "POST", map[string]string{"path": *path}, h)
		record("cloud_read_access", ok && cloud["pathReadable"] == true)
		diag, ok := d.api(ctx, b, "/cloudStorage/diagnose", "POST", map[string]string{"path": *path}, h)
		p, _ := diag["playback"].(map[string]any)
		record("cloud_link_resolution", ok && p["targetValidated"] == true)
		_, ok = d.api(ctx, b, "/emby/test", "POST", map[string]any{}, h)
		record("emby_reachable", ok)
		mapping, ok := d.api(ctx, b, "/emby/check", "POST", map[string]string{"itemId": *item, "mediaSourceId": get("M0_EMBY_MEDIA_SOURCE_ID"), "userToken": get("M0_EMBY_USER_TOKEN")}, h)
		record("emby_permission_mapping", ok && mapping["canPlay"] == true && mapping["sourceMapped"] == true)
		record("strm_302_cdn", stable != "" && d.redirect(ctx, stable, http.Header{"Range": []string{"bytes=0-1023"}}, false))
		stream := g + "/Videos/" + url.PathEscape(*item) + "/stream?" + url.Values{"Static": []string{"true"}, "MediaSourceId": []string{get("M0_EMBY_MEDIA_SOURCE_ID")}}.Encode()
		record("emby_302_cdn", d.redirect(ctx, stream, http.Header{"X-Emby-Token": []string{get("M0_EMBY_USER_TOKEN")}, "Range": []string{"bytes=0-1023"}}, true))
	}
	passed := len(checks) > 0
	for _, v := range checks {
		passed = passed && v["passed"] == true
	}
	reportValue["checks"] = checks
	reportValue["serverChainVerified"] = passed
	reportValue["status"] = "verification_failed"
	if passed {
		reportValue["status"] = "server_chain_verified"
	}
	if e = reportFile(root, *report, reportValue); e != nil {
		return e
	}
	if !passed {
		return ErrVerificationFailed
	}
	return nil
}
func singleQuery(q url.Values) bool {
	for _, v := range q {
		if len(v) != 1 {
			return false
		}
	}
	return len(q["apiKey"]) == 1
}

type checkedTransport struct {
	base   http.RoundTripper
	policy playback.Policy
}

func (t checkedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if e := t.policy.Check(r.Context(), r.URL.String()); e != nil {
		return nil, e
	}
	return t.base.RoundTrip(r)
}
func readPassword(r io.Reader) (string, error) {
	line, e := bufio.NewReader(io.LimitReader(r, 4097)).ReadString('\n')
	if len(line) > 4096 {
		return "", errors.New("password input too long")
	}
	if e != nil && e != io.EOF {
		return "", e
	}
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
}
func promptPassword() (string, error) {
	st, e := os.Stdin.Stat()
	if e != nil || st.Mode()&os.ModeCharDevice == 0 {
		return "", errors.New("private password file required without terminal")
	}
	cmd := exec.Command("stty", "-echo")
	cmd.Stdin = os.Stdin
	if e = cmd.Run(); e != nil {
		return "", errors.New("cannot disable terminal echo")
	}
	defer func() { cmd := exec.Command("stty", "echo"); cmd.Stdin = os.Stdin; _ = cmd.Run() }()
	_, _ = fmt.Fprint(os.Stderr, "管理员密码: ")
	line, e := readPassword(os.Stdin)
	_, _ = fmt.Fprintln(os.Stderr)
	return line, e
}
func loadCheck(root string, args []string) error {
	f := flag.NewFlagSet("load-check", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	base := f.String("base-url", "http://localhost:8080", "owned origin")
	requests := f.Int("requests", 500, "1..10000")
	workers := f.Int("concurrency", 16, "1..64")
	report := f.String("report", "data/diagnostics/readiness-load.json", "private report")
	if e := f.Parse(args); e != nil {
		return e
	}
	b, e := origin(*base, false)
	if e != nil || *requests < 1 || *requests > 10000 || *workers < 1 || *workers > 64 || f.NArg() != 0 {
		return errors.New("bounded readiness requests/concurrency and origin required")
	}
	lat := make([]float64, *requests)
	succeeded := make([]bool, *requests)
	queue := make(chan int)
	var wg sync.WaitGroup
	start := time.Now()
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: noRedirect}
	for range *workers {
		wg.Go(func() {
			for i := range queue {
				s := time.Now()
				res, e := client.Get(b + "/readyz")
				if e == nil {
					var v struct{ Data struct{ Status string } }
					e = json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&v)
					_ = res.Body.Close()
					succeeded[i] = e == nil && res.StatusCode == 200 && v.Data.Status == "ready"
				}
				lat[i] = float64(time.Since(s).Microseconds()) / 1000
			}
		})
	}
	for i := range *requests {
		queue <- i
	}
	close(queue)
	wg.Wait()
	elapsed := time.Since(start).Seconds()
	success := 0
	for _, v := range succeeded {
		if v {
			success++
		}
	}
	slices.Sort(lat)
	median := (lat[(*requests-1)/2] + lat[*requests/2]) / 2
	v := map[string]any{"checkedAt": time.Now().UTC(), "scope": "readiness API only; no media/CDN/client throughput claim", "requests": *requests, "concurrency": *workers, "succeeded": success, "failed": *requests - success, "elapsedSeconds": elapsed, "requestsPerSecond": float64(*requests) / elapsed, "latencyMs": map[string]float64{"median": median, "p95": lat[int(float64(*requests-1)*.95)], "max": lat[*requests-1]}}
	if e = reportFile(root, *report, v); e != nil {
		return e
	}
	if success != *requests {
		return errors.New("readiness requests failed")
	}
	return nil
}
