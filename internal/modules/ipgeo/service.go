package ipgeo

import (
	"archive/zip"
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"log/slog"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/clientip"
	"media-tool/internal/platform/secrets"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const Attribution = "IP地址位置数据由纯真CZ88提供支持"
const downloadURL = "https://www.cz88.net/api/communityIpAuthorization/communityIpDbFile"

type Credentials struct {
	DownloadKey string `json:"downloadKey"`
	DecryptKey  string `json:"decryptKey"`
}
type Status struct {
	Configured  bool       `json:"configured"`
	IPv4        bool       `json:"ipv4"`
	IPv6        bool       `json:"ipv6"`
	UpdatedAt   *time.Time `json:"updatedAt"`
	SHA256      string     `json:"sha256"`
	Attribution string     `json:"attribution"`
}
type cached struct {
	ip       string
	location Location
}
type Service struct {
	DB        *pgxpool.Pool
	Vault     *secrets.Vault
	Directory string
	Client    *http.Client
	mu        sync.Mutex
	dbs       []*Database
	updated   *time.Time
	digest    string
	cache     map[string]*list.Element
	lru       *list.List
}

func New(db *pgxpool.Pool, v *secrets.Vault, dir string) *Service {
	return &Service{DB: db, Vault: v, Directory: dir, Client: &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("CZ88 不允许重定向") }}, cache: map[string]*list.Element{}, lru: list.New()}
}
func (s *Service) credentials(ctx context.Context) (Credentials, error) {
	out := Credentials{}
	if s.DB == nil || s.Vault == nil {
		return out, nil
	}
	var raw []byte
	e := s.DB.QueryRow(ctx, `SELECT encrypted_credentials FROM ip_geo_settings WHERE singleton`).Scan(&raw)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, nil
	}
	if e != nil {
		return out, e
	}
	v, e := s.Vault.Open("cz88-community", raw)
	if e != nil {
		return out, e
	}
	return Credentials{v["downloadKey"], v["decryptKey"]}, nil
}
func (s *Service) Save(ctx context.Context, v Credentials) error {
	if s.DB == nil || s.Vault == nil {
		return errors.New("敏感设置不可用")
	}
	old, e := s.credentials(ctx)
	if e != nil {
		return e
	}
	if v.DownloadKey == "" {
		v.DownloadKey = old.DownloadKey
	}
	if v.DecryptKey == "" {
		v.DecryptKey = old.DecryptKey
	}
	if len(v.DownloadKey) < 8 || len(v.DownloadKey) > 2048 || strings.ContainsAny(v.DownloadKey, " \r\n\x00") || len(v.DecryptKey) > 128 {
		return ErrDatabase
	}
	// Validate AES key without exposing it or requiring a database download.
	if _, e = decodeKey(v.DecryptKey); e != nil {
		return e
	}
	raw, e := s.Vault.Seal("cz88-community", map[string]string{"downloadKey": v.DownloadKey, "decryptKey": v.DecryptKey})
	if e != nil {
		return e
	}
	_, e = s.DB.Exec(ctx, `INSERT INTO ip_geo_settings(singleton,encrypted_credentials)VALUES(true,$1)ON CONFLICT(singleton)DO UPDATE SET encrypted_credentials=EXCLUDED.encrypted_credentials,updated_at=now()`, raw)
	return e
}
func (s *Service) Status(ctx context.Context) Status {
	c, _ := s.credentials(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Status{Configured: c.DownloadKey != "" && c.DecryptKey != "", UpdatedAt: s.updated, SHA256: s.digest, Attribution: Attribution}
	for _, d := range s.dbs {
		if d.size == 4 {
			out.IPv4 = true
		} else {
			out.IPv6 = true
		}
	}
	return out
}
func (s *Service) Lookup(raw string) Location {
	ip := clientip.Sanitize(raw)
	out := Location{IP: ip}
	if ip == "" {
		return out
	}
	a, _ := netip.ParseAddr(ip)
	s.mu.Lock()
	defer s.mu.Unlock()
	cacheKey := time.Now().Format("2006-01-02") + ":" + ip
	if x := s.cache[cacheKey]; x != nil {
		s.lru.MoveToFront(x)
		return x.Value.(cached).location
	}
	for _, d := range s.dbs {
		if a.Is4() == (d.size == 4) {
			out = d.Lookup(a)
			break
		}
	}
	x := s.lru.PushFront(cached{cacheKey, out})
	s.cache[cacheKey] = x
	if s.lru.Len() > 4096 {
		x = s.lru.Back()
		delete(s.cache, x.Value.(cached).ip)
		s.lru.Remove(x)
	}
	return out
}
func parseBundle(raw []byte, key string) ([]*Database, error) {
	if len(raw) > maxDatabase {
		return nil, ErrDatabase
	}
	if !bytes.HasPrefix(raw, []byte{'P', 'K'}) {
		d, e := Parse(raw, key, time.Now())
		if e != nil {
			return nil, e
		}
		return []*Database{d}, nil
	}
	z, e := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if e != nil {
		return nil, ErrDatabase
	}
	out := []*Database{}
	seen := map[int]bool{}
	total := uint64(0)
	for _, f := range z.File {
		if f.FileInfo().IsDir() || !strings.HasSuffix(strings.ToLower(f.Name), ".czdb") {
			continue
		}
		if len(out) >= 2 || f.UncompressedSize64 > maxDatabase {
			return nil, ErrDatabase
		}
		total += f.UncompressedSize64
		if total > maxDatabase {
			return nil, ErrDatabase
		}
		r, e := f.Open()
		if e != nil {
			return nil, ErrDatabase
		}
		b, e := io.ReadAll(io.LimitReader(r, maxDatabase+1))
		ce := r.Close()
		if e != nil || ce != nil {
			return nil, ErrDatabase
		}
		d, e := Parse(b, key, time.Now())
		if e != nil || seen[dSize(d)] {
			return nil, ErrDatabase
		}
		seen[d.size] = true
		out = append(out, d)
	}
	if len(out) == 0 {
		return nil, ErrDatabase
	}
	return out, nil
}
func dSize(d *Database) int {
	if d == nil {
		return 0
	}
	return d.size
}
func (s *Service) install(raw []byte, key string, persist bool) error {
	dbs, e := parseBundle(raw, key)
	if e != nil {
		return e
	}
	if persist {
		if e = os.MkdirAll(s.Directory, 0700); e != nil {
			return e
		}
		f, e := os.CreateTemp(s.Directory, ".czdb-*")
		if e != nil {
			return e
		}
		name := f.Name()
		defer func() {
			e := os.Remove(name)
			if e != nil && !os.IsNotExist(e) {
				deferutil.Report(e, "remove CZDB staging file")
			}
		}()
		if _, e = f.Write(raw); e != nil {
			deferutil.Close(f, "Write or Sync already failed; discard staging file")
			return e
		}
		if e = f.Sync(); e != nil {
			deferutil.Close(f, "Write or Sync already failed; discard staging file")
			return e
		}
		if e = f.Close(); e != nil {
			return e
		}
		if e = os.Rename(name, filepath.Join(s.Directory, "current.bundle")); e != nil {
			return e
		}
	}
	stat, e := os.Stat(filepath.Join(s.Directory, "current.bundle"))
	now := time.Now()
	if e == nil {
		now = stat.ModTime()
	}
	sum := sha256.Sum256(raw)
	s.mu.Lock()
	s.dbs = dbs
	s.updated = &now
	s.digest = hex.EncodeToString(sum[:])
	s.cache = map[string]*list.Element{}
	s.lru.Init()
	s.mu.Unlock()
	return nil
}
func (s *Service) Update(ctx context.Context, v Credentials) error {
	if v.DownloadKey == "" || v.DecryptKey == "" {
		return nil
	}
	u, _ := url.Parse(downloadURL)
	q := u.Query()
	q.Set("fn", "czdb")
	q.Set("key", v.DownloadKey)
	u.RawQuery = q.Encode()
	req, e := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if e != nil {
		return errors.New("CZ88 更新失败")
	}
	response, e := s.Client.Do(req)
	if e != nil {
		return errors.New("CZ88 更新失败")
	}
	defer deferutil.Close(response.Body, "CZ88 response processing already checked read/validation result")
	if response.StatusCode != 200 {
		return errors.New("CZ88 授权下载失败")
	}
	raw, e := io.ReadAll(io.LimitReader(response.Body, maxDatabase+1))
	if e != nil {
		return errors.New("CZ88 下载不完整")
	}
	return s.install(raw, v.DecryptKey, true)
}
func (s *Service) Run(ctx context.Context, logger *slog.Logger) {
	c, e := s.credentials(ctx)
	if e == nil && c.DecryptKey != "" {
		if raw, e := os.ReadFile(filepath.Join(s.Directory, "current.bundle")); e == nil {
			if e = s.install(raw, c.DecryptKey, false); e != nil {
				logger.Warn("CZDB 旧库不可用，降级为仅显示公网 IP", "module", "ipgeo")
			}
		}
	}
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	next := time.Time{}
	fingerprint := ""
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			c, e = s.credentials(ctx)
			if e != nil || c.DownloadKey == "" || c.DecryptKey == "" {
				continue
			}
			raw, _ := json.Marshal(c)
			hash := sha256.Sum256(raw)
			fp := hex.EncodeToString(hash[:])
			s.mu.Lock()
			last := s.updated
			s.mu.Unlock()
			if fp == fingerprint && now.Before(next) {
				continue
			}
			if fp == fingerprint && last != nil && now.Sub(*last) < 7*24*time.Hour {
				continue
			}
			fingerprint = fp
			next = now.Add(6 * time.Hour)
			check, cancel := context.WithTimeout(ctx, 2*time.Minute)
			e = s.Update(check, c)
			cancel()
			if e != nil {
				logger.Warn("CZDB 更新失败，保留旧库；无库时仅显示公网 IP", "module", "ipgeo")
			}
		}
	}
}
