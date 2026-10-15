package config

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"media-tool/internal/platform/logging"
	"net"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"media-tool/internal/platform/requestbudget"
)

type Config struct {
	LogDir                                                   string
	MaintenanceDir                                           string
	PasswordResetFile                                        string
	LocalBrowseRoots                                         []string
	AuthAllowedOrigins                                       []string
	HostProcDir                                              string
	LogLevel                                                 string
	EmbyGatewayAddress                                       string
	EmbyPublicURL                                            string
	Environment, Address, PublicURL, SessionKey, STRMKey     string
	SecretKey                                                string
	DatabaseURL, RedisURL, STRMOutputDir                     string
	STRMAllowedRoots                                         []string
	CloudDriveEndpoint, CloudDriveToken                      string
	CloudDriveUsername, CloudDrivePassword, CloudDriveCAFile string
	CloudDriveRequestTimeout                                 time.Duration
	PlaybackResolveTimeout                                   time.Duration
	CloudDriveMaxListFiles                                   int
	STRMProxyFallback, STRMForceProxy, PlaybackAllowHTTP     bool
	LegacyPlaybackEnabled                                    bool
	PlaybackAllowedHosts                                     []string
	Emby302Enabled                                           bool
	EmbyDirectPlayOnly                                       bool
	EmbySTRMPathPrefix                                       string
	EmbyResolverOrigins                                      []string
	EmbyUpstream                                             string
	EmbyProxyEnabled                                         bool
	WorkerPollInterval, WorkerLease                          time.Duration
}

func Load() (Config, error) { return FromLookup(os.LookupEnv) }
func FromLookup(lookup func(string) (string, bool)) (Config, error) {
	get := func(key, fallback string) string {
		if v, ok := lookup(key); ok {
			return strings.TrimSpace(v)
		}
		return fallback
	}
	secret := func(key string) string { value, _ := lookup(key); return value }
	c := Config{
		LogDir: get("APP_LOG_DIR", "./data/logs"), PasswordResetFile: get("PASSWORD_RESET_FILE", "./data/password"), LocalBrowseRoots: strings.Split(get("LOCAL_BROWSE_ROOTS", "./data"), ","),
		LogLevel:       get("LOG_LEVEL", "info"),
		HostProcDir:    get("HOST_PROC_DIR", "/proc"),
		MaintenanceDir: get("OPS_DATA_DIR", "/data/maintenance"),
		EmbyPublicURL:  get("EMBY_PUBLIC_URL", ""), EmbyGatewayAddress: get("EMBY_GATEWAY_ADDR", "127.0.0.1:8091"), Environment: get("APP_ENV", "development"), Address: get("APP_ADDR", "127.0.0.1:8080"),
		PublicURL: get("APP_PUBLIC_URL", "http://localhost:8080"), SessionKey: secret("SESSION_SECRET_KEY"), STRMKey: get("STRM_API_KEY", ""),
		SecretKey: secret("APP_SECRET_KEY"), DatabaseURL: get("DATABASE_URL", ""), RedisURL: get("REDIS_URL", ""), STRMOutputDir: get("STRM_OUTPUT_DIR", "./data/strm"),
		CloudDriveEndpoint: get("CLOUDDRIVE_ENDPOINT", ""), CloudDriveToken: get("CLOUDDRIVE_TOKEN", ""), EmbyUpstream: get("EMBY_UPSTREAM_URL", ""),
		CloudDriveUsername: get("CLOUDDRIVE_USERNAME", ""), CloudDrivePassword: secret("CLOUDDRIVE_PASSWORD"), CloudDriveCAFile: get("CLOUDDRIVE_CA_FILE", ""),
		EmbySTRMPathPrefix: get("EMBY_STRM_PATH_PREFIX", ""),
	}
	if _, err := logging.ParseLevel(c.LogLevel); err != nil {
		return c, err
	}
	if c.Environment != "development" && c.Environment != "production" {
		return c, errors.New("APP_ENV must be development or production")
	}
	if c.Address == "" {
		return c, errors.New("APP_ADDR is required")
	}
	if c.SessionKey == "" {
		key, err := hex.DecodeString(c.SecretKey)
		if err != nil || len(key) != 32 {
			return c, errors.New("SESSION_SECRET_KEY or a valid APP_SECRET_KEY is required; run scripts/init-env.py")
		}
		h := hmac.New(sha256.New, key)
		h.Write([]byte("media-tool:administrator-signing:v2"))
		c.SessionKey = hex.EncodeToString(h.Sum(nil))
	}
	signingKey, keyErr := hex.DecodeString(c.SessionKey)
	if keyErr != nil || len(signingKey) != 32 {
		return c, errors.New("SESSION_SECRET_KEY must be 32 bytes encoded as hexadecimal")
	}
	if len(c.STRMKey) < 32 || strings.Contains(c.STRMKey, "CHANGE_ME") {
		return c, errors.New("STRM_API_KEY must contain at least32 characters; run scripts/init-env.py")
	}
	if c.SessionKey == c.STRMKey {
		return c, errors.New("session signing and STRM keys must differ")
	}
	if err := validURL(c.PublicURL); err != nil {
		return c, fmt.Errorf("APP_PUBLIC_URL: %w", err)
	}
	u, _ := url.Parse(c.PublicURL)
	if u.Path != "" && u.Path != "/" {
		return c, errors.New("APP_PUBLIC_URL must be an origin without a path")
	}
	if c.Environment == "production" && u.Scheme != "https" {
		return c, errors.New("production APP_PUBLIC_URL requires https")
	}
	c.PublicURL = strings.TrimRight(c.PublicURL, "/")
	for _, raw := range strings.Split(get("AUTH_ALLOWED_ORIGINS", ""), ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if err := validURL(raw); err != nil {
			return c, fmt.Errorf("AUTH_ALLOWED_ORIGINS: %w", err)
		}
		u, _ := url.Parse(raw)
		if u.Path != "" && u.Path != "/" {
			return c, errors.New("AUTH_ALLOWED_ORIGINS must contain origins without paths")
		}
		c.AuthAllowedOrigins = append(c.AuthAllowedOrigins, strings.TrimRight(raw, "/"))
	}
	for _, r := range strings.Split(get("STRM_ALLOWED_ROOTS", "/115/Media"), ",") {
		r = strings.TrimSpace(r)
		if !strings.HasPrefix(r, "/") || strings.Contains(r, "\\") || strings.ContainsAny(r, "\x00\r\n") {
			return c, errors.New("STRM_ALLOWED_ROOTS must contain absolute cloud paths")
		}
		for _, segment := range strings.Split(r, "/") {
			if segment == ".." {
				return c, errors.New("STRM_ALLOWED_ROOTS cannot contain traversal")
			}
		}
		c.STRMAllowedRoots = append(c.STRMAllowedRoots, path.Clean(r))
	}
	if c.EmbyPublicURL != "" {
		if err := validURL(c.EmbyPublicURL); err != nil {
			return c, fmt.Errorf("EMBY_PUBLIC_URL: %w", err)
		}
		u, _ := url.Parse(c.EmbyPublicURL)
		if u.Path != "" && u.Path != "/" {
			return c, errors.New("EMBY_PUBLIC_URL must be an origin")
		}
		if c.Environment == "production" && u.Scheme != "https" {
			return c, errors.New("production EMBY_PUBLIC_URL requires HTTPS")
		}
		c.EmbyPublicURL = strings.TrimRight(c.EmbyPublicURL, "/")
	}
	if c.EmbyUpstream != "" {
		if err := validURL(c.EmbyUpstream); err != nil {
			return c, fmt.Errorf("EMBY_UPSTREAM_URL: %w", err)
		}
	}
	var err error
	for _, flag := range []struct {
		name     string
		target   *bool
		fallback string
	}{
		{"STRM_LEGACY_KEY_ENABLED", &c.LegacyPlaybackEnabled, "true"},
		{"STRM_PROXY_FALLBACK", &c.STRMProxyFallback, "true"}, {"STRM_FORCE_PROXY", &c.STRMForceProxy, "false"},
		{"EMBY_DIRECT_PLAY_ONLY", &c.EmbyDirectPlayOnly, "false"},
		{"PLAYBACK_ALLOW_HTTP", &c.PlaybackAllowHTTP, "false"}, {"EMBY_302_ENABLED", &c.Emby302Enabled, "false"},
	} {
		*flag.target, err = strconv.ParseBool(get(flag.name, flag.fallback))
		if err != nil {
			return c, fmt.Errorf("%s must be true or false", flag.name)
		}
	}
	c.PlaybackResolveTimeout, err = time.ParseDuration(get("PLAYBACK_RESOLVE_TIMEOUT", requestbudget.DefaultResolve.String()))
	if err != nil || c.PlaybackResolveTimeout < requestbudget.MinConfiguredResolve || c.PlaybackResolveTimeout > requestbudget.MaxConfiguredResolve {
		return c, errors.New("PLAYBACK_RESOLVE_TIMEOUT must be 5s..1m")
	}
	c.CloudDriveRequestTimeout, err = time.ParseDuration(get("CLOUDDRIVE_REQUEST_TIMEOUT", "10s"))
	if err != nil || c.CloudDriveRequestTimeout < 500*time.Millisecond || c.CloudDriveRequestTimeout > time.Minute {
		return c, errors.New("CLOUDDRIVE_REQUEST_TIMEOUT must be 500ms..1m")
	}
	c.CloudDriveMaxListFiles, err = strconv.Atoi(get("CLOUDDRIVE_MAX_LIST_FILES", "1000"))
	if err != nil || c.CloudDriveMaxListFiles < 1 || c.CloudDriveMaxListFiles > 10000 {
		return c, errors.New("CLOUDDRIVE_MAX_LIST_FILES must be 1..10000")
	}
	for _, host := range strings.Split(get("PLAYBACK_ALLOWED_HOSTS", ""), ",") {
		host = strings.ToLower(strings.TrimSpace(host))
		if host == "" {
			continue
		}
		raw := strings.TrimPrefix(host, "*.")
		if strings.ContainsAny(raw, "/:@?\\ \t\r\n") || (!strings.Contains(raw, ".") || strings.Contains(raw, "*")) {
			return c, errors.New("PLAYBACK_ALLOWED_HOSTS must be DNS hostnames or *.domain names")
		}
		c.PlaybackAllowedHosts = append(c.PlaybackAllowedHosts, host)
	}
	for _, origin := range strings.Split(get("EMBY_STRM_RESOLVER_ORIGINS", ""), ",") {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		if validURL(origin) != nil {
			return c, errors.New("invalid EMBY_STRM_RESOLVER_ORIGINS")
		}
		parsed, _ := url.Parse(origin)
		if parsed.Path != "" && parsed.Path != "/" {
			return c, errors.New("resolver origins cannot contain paths")
		}
		c.EmbyResolverOrigins = append(c.EmbyResolverOrigins, strings.TrimRight(origin, "/"))
	}
	if c.EmbySTRMPathPrefix != "" && (!strings.HasPrefix(c.EmbySTRMPathPrefix, "/") || path.Clean(c.EmbySTRMPathPrefix) != c.EmbySTRMPathPrefix || strings.Contains(c.EmbySTRMPathPrefix, "\\")) {
		return c, errors.New("EMBY_STRM_PATH_PREFIX must be a canonical absolute Emby STRM path")
	}
	if c.CloudDriveEndpoint != "" {
		raw := c.CloudDriveEndpoint
		if !strings.Contains(raw, "://") {
			raw = "http://" + raw
		}
		if validURL(raw) != nil {
			return c, errors.New("CLOUDDRIVE_ENDPOINT must be an http(s) gRPC origin")
		}
		parsed, _ := url.Parse(raw)
		if parsed.Path != "" && parsed.Path != "/" {
			return c, errors.New("CLOUDDRIVE_ENDPOINT must not contain a path")
		}
		if parsed.Port() != "" {
			_, _, err = net.SplitHostPort(parsed.Host)
			if err != nil {
				return c, errors.New("invalid CloudDrive2 endpoint")
			}
		}
		if c.CloudDriveToken == "" && (c.CloudDriveUsername == "" || c.CloudDrivePassword == "") {
			return c, errors.New("set CLOUDDRIVE_TOKEN or both CLOUDDRIVE_USERNAME and CLOUDDRIVE_PASSWORD")
		}
	}

	c.EmbyProxyEnabled, err = strconv.ParseBool(get("EMBY_PROXY_ENABLED", "false"))
	if err != nil {
		return c, errors.New("EMBY_PROXY_ENABLED must be true or false")
	}
	if c.EmbyProxyEnabled && c.EmbyUpstream == "" {
		return c, errors.New("EMBY_UPSTREAM_URL is required when proxy is enabled")
	}
	if c.Emby302Enabled && !c.EmbyProxyEnabled {
		return c, errors.New("EMBY_302_ENABLED requires EMBY_PROXY_ENABLED")
	}
	c.WorkerPollInterval, err = time.ParseDuration(get("WORKER_POLL_INTERVAL", "2s"))
	if err != nil || c.WorkerPollInterval < 100*time.Millisecond {
		return c, errors.New("WORKER_POLL_INTERVAL must be >= 100ms")
	}
	c.WorkerLease, err = time.ParseDuration(get("WORKER_LEASE", "90s"))
	if err != nil || c.WorkerLease < 3*time.Second {
		return c, errors.New("WORKER_LEASE must be >= 3s")
	}
	return c, nil
}
func validURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("expected an absolute http(s) URL without credentials, query, or fragment")
	}
	return nil
}
