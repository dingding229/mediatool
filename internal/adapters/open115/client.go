// Package open115 implements only the documented 115 Open Platform APIs.
// Fixed HTTPS origins, caller-UA download requests and no Cookie/app-ID borrowing.
package open115

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/requestbudget"
)

const APIOrigin = "https://proapi.115.com"
const AuthOrigin = "https://passportapi.115.com"
const QRStatusURL = "https://qrcodeapi.115.com/get/status/"

var ErrAuthorization = errors.New("115 official authorization required")
var ErrRateLimited = errors.New("115 official API rate limited; retry later")
var ErrDirectRejected = errors.New("115 CDN rejected direct playback conditions")
var pickPattern = regexp.MustCompile(`^[A-Za-z0-9]{8,64}$`)

// APIError intentionally omits raw provider messages, URLs and credentials.
type APIError struct {
	Code       int64
	HTTPStatus int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("115 official API rejected request (code=%d, status=%d)", e.Code, e.HTTPStatus)
}
func (e *APIError) SafeDiagnostic() (int64, int) { return e.Code, e.HTTPStatus }
func (e *APIError) TerminalRefresh() bool {
	switch e.Code {
	case 40140114, 40140115, 40140116, 40140119, 40140120, 40140137:
		return true
	}
	return false
}
func (e *APIError) ExpiredAccess() bool {
	return e.HTTPStatus == 401 || e.Code == 40140125 || e.Code == 40140126
}
func (e *APIError) RateLimited() bool { return e.HTTPStatus == 429 || e.Code == 40140117 }

type State bool

func (s *State) UnmarshalJSON(b []byte) error {
	switch string(b) {
	case "true", "1":
		*s = true
	case "false", "0":
		*s = false
	default:
		return errors.New("invalid official API state")
	}
	return nil
}

type ID string

func (s *ID) UnmarshalJSON(b []byte) error {
	var text string
	if json.Unmarshal(b, &text) == nil {
		*s = ID(text)
		return nil
	}
	var n json.Number
	if json.Unmarshal(b, &n) != nil {
		return errors.New("invalid identifier")
	}
	*s = ID(n.String())
	return nil
}

type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}
type Device struct {
	UID    string `json:"uid"`
	Sign   string `json:"sign"`
	Time   int64  `json:"time"`
	QRCode string `json:"qrcode"`
}
type Poll struct {
	Valid  bool
	Status int
}
type FileInfo struct {
	ID       ID     `json:"file_id"`
	Name     string `json:"file_name"`
	Size     int64  `json:"size_byte"`
	PickCode string `json:"pick_code"`
	Category ID     `json:"file_category"`
	SHA1     string `json:"sha1"`
}
type Download struct {
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
	PickCode string `json:"pick_code"`
	URL      struct {
		URL string `json:"url"`
	} `json:"url"`
}

// Share bounded keep-alive connections across path lookup, download and token
// calls. A new Transport per call discards pooling and retains idle sockets.
// Request authorization remains on the request; there is no shared cookie jar.
func newOfficialTransport() *http.Transport {
	return &http.Transport{
		Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2: true, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout: 60 * time.Second, MaxIdleConns: 32, MaxIdleConnsPerHost: 8, MaxConnsPerHost: 16,
		MaxResponseHeaderBytes: 64 << 10,
	}
}

var officialTransport = newOfficialTransport()

type Client struct{ HTTP *http.Client }

func (c Client) client() *http.Client {
	var h http.Client
	if c.HTTP != nil {
		h = *c.HTTP
	} else {
		h = http.Client{Timeout: 12 * time.Second, Transport: officialTransport}
	}
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if h.Timeout == 0 {
		h.Timeout = 12 * time.Second
	}
	return &h
}
func (c Client) call(ctx context.Context, method, endpoint, token, agent string, body io.Reader, typ string, out any) error {
	return c.callAttempt(ctx, method, endpoint, token, agent, body, typ, out, 1)
}
func (c Client) callAttempt(ctx context.Context, method, endpoint, token, agent string, body io.Reader, typ string, out any, attempt int) (callErr error) {
	started := time.Now()
	trace := newHTTPTrace(started, attempt)
	var stage storage.ResolveStage
	switch endpoint {
	case APIOrigin + "/open/folder/get_info":
		stage = storage.StageOfficialHTTPPath
	case APIOrigin + "/open/ufile/downurl":
		stage = storage.StageOfficialHTTPDownload
	case AuthOrigin + "/open/refreshToken":
		stage = storage.StageOfficialHTTPRefresh
	}
	defer func() { storage.ObserveResolveHTTP(ctx, stage, started, callErr, trace.snapshot()) }()
	req, err := http.NewRequestWithContext(trace.traceContext(ctx), method, endpoint, body)
	if err != nil {
		return storage.ErrInvalidResponse
	}
	if token != "" {
		if strings.ContainsAny(token, "\r\n\x00") {
			return ErrAuthorization
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if agent == "" {
		agent = "MediaTool-115-Open/1.0"
	}
	if len(agent) > 512 || strings.ContainsAny(agent, "\r\n\x00") {
		return storage.ErrInvalidResponse
	}
	req.Header.Set("User-Agent", agent)
	if typ != "" {
		req.Header.Set("Content-Type", typ)
	}
	res, err := c.client().Do(req)
	if res != nil {
		trace.update(func() { trace.timing.ResponseReceived = true })
	}
	if err != nil {
		if res != nil && res.Body != nil {
			if closeErr := res.Body.Close(); closeErr != nil {
				// Preserve the transport error; the response was unusable already.
			}
		}
		failure := networkFailure(ctx, err)
		if !trace.receivedResponse() && ctx.Err() == nil {
			return &noHTTPResponseError{cause: failure}
		}
		return failure
	}
	defer func() {
		if closeErr := res.Body.Close(); closeErr != nil {
			// The response body was already consumed/failed; closing cannot change the sanitized result.
		}
	}()
	raw, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var timed net.Error
		if errors.As(err, &timed) && timed.Timeout() {
			return errors.Join(storage.ErrUnavailable, storage.ErrRequestTimeout)
		}
		return storage.ErrInvalidResponse
	}
	if len(raw) > 1<<20 {
		return storage.ErrInvalidResponse
	}
	var envelope struct {
		State State           `json:"state"`
		Code  int64           `json:"code"`
		Errno int64           `json:"errno"`
		Data  json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return &APIError{HTTPStatus: res.StatusCode}
	}
	code := envelope.Code
	if code == 0 {
		code = envelope.Errno
	}
	if res.StatusCode != 200 || !envelope.State || code != 0 {
		return &APIError{Code: code, HTTPStatus: res.StatusCode}
	}
	if out != nil && json.Unmarshal(envelope.Data, out) != nil {
		return storage.ErrInvalidResponse
	}
	return nil
}

// Preserve timeout/cancellation identity without retaining URL-bearing net errors.
func networkFailure(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var timed net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timed) && timed.Timeout() {
		return errors.Join(storage.ErrUnavailable, storage.ErrRequestTimeout)
	}
	if connectionReset(err) {
		return errors.Join(storage.ErrUnavailable, errTransportReset)
	}
	return storage.ErrUnavailable
}
func multipartBody(values map[string]string) (*bytes.Buffer, string, error) {
	b := &bytes.Buffer{}
	w := multipart.NewWriter(b)
	for k, v := range values {
		if err := w.WriteField(k, v); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return b, w.FormDataContentType(), nil
}
func (c Client) form(ctx context.Context, endpoint string, values url.Values, out any) error {
	return c.call(ctx, "POST", endpoint, "", "", strings.NewReader(values.Encode()), "application/x-www-form-urlencoded", out)
}
func (c Client) DeviceCode(ctx context.Context, appID, challenge string) (Device, error) {
	var d Device
	err := c.form(ctx, AuthOrigin+"/open/authDeviceCode", url.Values{"client_id": {appID}, "code_challenge": {challenge}, "code_challenge_method": {"sha256"}}, &d)
	if err == nil && (d.UID == "" || d.Sign == "" || d.QRCode == "" || len(d.QRCode) > 2048) {
		err = storage.ErrInvalidResponse
	}
	return d, err
}
func (c Client) Exchange(ctx context.Context, uid, verifier string) (Tokens, error) {
	var t Tokens
	err := c.form(ctx, AuthOrigin+"/open/deviceCodeToToken", url.Values{"uid": {uid}, "code_verifier": {verifier}}, &t)
	return validateTokens(t, err)
}
func (c Client) Refresh(ctx context.Context, refresh string) (Tokens, error) {
	var t Tokens
	err := c.form(ctx, AuthOrigin+"/open/refreshToken", url.Values{"refresh_token": {refresh}}, &t)
	return validateTokens(t, err)
}
func validateTokens(t Tokens, err error) (Tokens, error) {
	if err == nil && (t.AccessToken == "" || t.RefreshToken == "" || t.ExpiresIn < 60 || t.ExpiresIn > 86400*7 || strings.ContainsAny(t.AccessToken+t.RefreshToken, "\r\n\x00")) {
		err = storage.ErrInvalidResponse
	}
	return t, err
}
func NewPKCE() (string, string, error) {
	b := make([]byte, 48)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)
	hash := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(hash[:]), nil
}
func (c Client) Poll(ctx context.Context, d Device) (Poll, error) {
	endpoint := QRStatusURL + "?" + url.Values{"uid": {d.UID}, "time": {strconv.FormatInt(d.Time, 10)}, "sign": {d.Sign}}.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return Poll{}, storage.ErrInvalidResponse
	}
	res, err := c.client().Do(req)
	if err != nil {
		var n net.Error
		if errors.As(err, &n) && n.Timeout() && ctx.Err() == nil {
			return Poll{Valid: true}, nil
		}
		return Poll{}, storage.ErrUnavailable
	}
	defer func() {
		if closeErr := res.Body.Close(); closeErr != nil {
			// The response body was already consumed/failed; closing cannot change the sanitized result.
		}
	}()
	var v struct {
		State State `json:"state"`
		Code  int64 `json:"code"`
		Data  struct {
			Status int `json:"status"`
		} `json:"data"`
	}
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&v) != nil {
		return Poll{}, storage.ErrInvalidResponse
	}
	if v.Code != 0 {
		return Poll{}, &APIError{Code: v.Code, HTTPStatus: res.StatusCode}
	}
	return Poll{Valid: bool(v.State), Status: v.Data.Status}, nil
}
func (c Client) Probe(ctx context.Context, token string) error {
	var info struct {
		ID ID `json:"user_id"`
	}
	err := c.call(ctx, "GET", APIOrigin+"/open/user/info", token, "", nil, "", &info)
	if err == nil && info.ID == "" {
		return storage.ErrInvalidResponse
	}
	return err
}
func (c Client) FileByPath(ctx context.Context, token, fullPath string) (FileInfo, error) {
	var f FileInfo
	if !strings.HasPrefix(fullPath, "/") || path.Clean(fullPath) != fullPath || strings.ContainsAny(fullPath, "\r\n\x00") {
		return f, storage.ErrInvalidResponse
	}
	// Cap standalone callers as well as honoring the enclosing playback deadline.
	ctx, cancel := context.WithTimeout(ctx, requestbudget.DefaultResolve)
	defer cancel()
	attempt := func(number int) (FileInfo, error) {
		var current FileInfo
		b, typ, err := multipartBody(map[string]string{"path": fullPath})
		if err != nil {
			return current, storage.ErrInvalidResponse
		}
		err = c.callAttempt(ctx, "POST", APIOrigin+"/open/folder/get_info", token, "", b, typ, &current, number)
		if err == nil && (current.Category != "1" || current.ID == "" || !pickPattern.MatchString(current.PickCode) || current.Name != path.Base(fullPath)) {
			return current, storage.ErrInvalidResponse
		}
		return current, err
	}
	f, err := attempt(1)
	if retryableNoResponse(err) && enoughForRetry(ctx, c.client().Timeout) {
		timer := time.NewTimer(pathRetryBackoff)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return f, ctx.Err()
		case <-timer.C:
		}
		// Recheck after sleeping; scheduling/parent cancellation may consume budget.
		if ctx.Err() != nil {
			return f, ctx.Err()
		}
		deadline, ok := ctx.Deadline()
		if ok && ctx.Err() == nil && time.Until(deadline) > c.client().Timeout {
			f, err = attempt(2)
		}
	}
	return f, err
}
func (c Client) Download(ctx context.Context, token, pickCode, agent string) (Download, error) {
	var selected Download
	if !pickPattern.MatchString(pickCode) {
		return selected, storage.ErrInvalidResponse
	}
	b, typ, err := multipartBody(map[string]string{"pick_code": pickCode})
	if err != nil {
		return selected, storage.ErrInvalidResponse
	}
	var data map[string]Download
	if err = c.call(ctx, "POST", APIOrigin+"/open/ufile/downurl", token, agent, b, typ, &data); err != nil {
		return selected, err
	}
	if len(data) != 1 {
		return selected, storage.ErrInvalidResponse
	}
	for _, entry := range data {
		selected = entry
	}
	u, err := url.Parse(selected.URL.URL)
	if err != nil || u.User != nil || u.Scheme != "https" || u.Hostname() == "" || u.Fragment != "" || selected.PickCode != pickCode || strings.ContainsAny(selected.URL.URL, "\r\n\x00") {
		return Download{}, storage.ErrInvalidResponse
	}
	return selected, nil
}
