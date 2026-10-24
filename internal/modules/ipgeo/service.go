// Package ipgeo provides offline, public-IP-only attribution from pinned xdb files.
package ipgeo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lionsoul2014/ip2region/binding/golang/service"
	"media-tool/internal/platform/clientip"
)

const DefaultDirectory = "/app/ip2region"
const DatabaseCommit = "f8b0792b0cd729f5b2593f690aeed225e49c1967"
const IPv4SHA256 = "27ecba8c9a6d5e3b280aedbe7ee15c65372b179fbd0dad3845d8c608f2fcace1"
const IPv6SHA256 = "939f6b46bd2b8bec3cf7c5ceb8ba782266ae9b1f35b5ba7916700dec0b7506ed"

type Location struct {
	IP       string `json:"ip"`
	Country  string `json:"country,omitempty"`
	Province string `json:"province,omitempty"`
	City     string `json:"city,omitempty"`
	ISP      string `json:"isp,omitempty"`
	Label    string `json:"label,omitempty"`
}
type Status struct {
	Source        string     `json:"source"`
	Version       string     `json:"version"`
	IPv4          bool       `json:"ipv4"`
	IPv6          bool       `json:"ipv6"`
	IPv4Date      *time.Time `json:"ipv4Date"`
	IPv6Date      *time.Time `json:"ipv6Date"`
	DataDate      *time.Time `json:"dataDate"`
	CacheStrategy string     `json:"cacheStrategy"`
	MemoryBytes   int        `json:"memoryBytes"`
	Reason        string     `json:"reason,omitempty"`
}
type Service struct {
	// The pinned upstream searcher resets its mutable ioCount even in content mode.
	// An exclusive lock also serializes Close with searches, not just state reads.
	mu     sync.Mutex
	engine *service.Ip2Region
	status Status
}

func New(dir string) *Service {
	s := &Service{status: Status{Source: "ip2region", Version: DatabaseCommit[:12], CacheStrategy: "content"}}
	load := func(name, digest string, create func(int, string, int) (*service.Config, error)) *service.Config {
		path := filepath.Join(dir, name)
		f, e := os.Open(path)
		if e != nil {
			return nil
		}
		hash := sha256.New()
		_, e = io.Copy(hash, f)
		ce := f.Close()
		if e != nil || ce != nil || hex.EncodeToString(hash.Sum(nil)) != digest {
			return nil
		}
		config, e := create(service.BufferCache, path, 1)
		if e != nil {
			return nil
		}
		s.status.MemoryBytes += len(config.CBuffer())
		return config
	}
	v4 := load("ip2region_v4.xdb", IPv4SHA256, service.NewV4Config)
	v6 := load("ip2region_v6.xdb", IPv6SHA256, service.NewV6Config)
	if v4 != nil {
		at := time.Unix(int64(v4.Header().CreatedAt), 0).UTC()
		s.status.IPv4 = true
		s.status.IPv4Date = &at
		s.status.DataDate = &at
	}
	if v6 != nil {
		at := time.Unix(int64(v6.Header().CreatedAt), 0).UTC()
		s.status.IPv6 = true
		s.status.IPv6Date = &at
		if s.status.DataDate == nil || at.After(*s.status.DataDate) {
			s.status.DataDate = &at
		}
	}
	if v4 != nil || v6 != nil {
		var err error
		s.engine, err = service.NewIp2Region(v4, v6)
		if err != nil {
			s.status.IPv4, s.status.IPv6 = false, false
			s.status.MemoryBytes = 0
		}
	}
	if !s.status.IPv4 || !s.status.IPv6 {
		s.status.Reason = "镜像内 IP 数据缺失或校验失败，请重新拉取已发布镜像；不会联网下载。"
	}
	return s
}
func (s *Service) Status(context.Context) Status {
	if s == nil {
		return Status{Source: "ip2region", Version: DatabaseCommit[:12], CacheStrategy: "content", Reason: "IP 数据暂不可用"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}
func (s *Service) Lookup(raw string) Location {
	addr, e := netip.ParseAddr(strings.TrimSpace(raw))
	if e != nil {
		return Location{Label: "无效 IP"}
	}
	if !clientip.PublicIP(addr) {
		return Location{Label: "局域网/保留地址"}
	}
	out := Location{IP: addr.Unmap().String(), Label: "归属地暂不可用"}
	if s == nil {
		return out
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine == nil {
		return out
	}
	region, e := s.engine.Search(out.IP)
	if e != nil {
		return out
	}
	parts := strings.Split(region, "|")
	if len(parts) < 4 {
		return out
	}
	clean := func(v string) string {
		v = strings.TrimSpace(v)
		if v == "0" {
			return ""
		}
		return v
	}
	out.Country, out.Province, out.City, out.ISP = clean(parts[0]), clean(parts[1]), clean(parts[2]), clean(parts[3])
	labels := []string{}
	for _, v := range []string{out.Country, out.Province, out.City, out.ISP} {
		if v != "" && (len(labels) == 0 || labels[len(labels)-1] != v) {
			labels = append(labels, v)
		}
	}
	out.Label = strings.Join(labels, " ")
	if out.Label == "" {
		out.Label = "未知归属地"
	}
	return out
}
func (s *Service) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine != nil {
		s.engine.Close()
		s.engine = nil
	}
	s.status.IPv4, s.status.IPv6 = false, false
	s.status.MemoryBytes = 0
}
