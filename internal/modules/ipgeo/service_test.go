package ipgeo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func testService(t *testing.T) *Service {
	t.Helper()
	s := New(filepath.Join("..", "..", "..", "data", "build", "ip2region"))
	status := s.Status(t.Context())
	if !status.IPv4 || !status.IPv6 {
		t.Fatal("run sh scripts/prepare-ip2region.sh data/build/ip2region before real offline query tests", status)
	}
	t.Cleanup(s.Close)
	return s
}
func TestPinnedOfflineIPv4IPv6AndPublicBoundary(t *testing.T) {
	s := testService(t)
	for _, tc := range []struct{ ip, country, province, city, isp string }{
		{"223.5.5.5", "中国", "浙江省", "杭州市", "阿里"},
		{"114.114.114.114", "中国", "江苏省", "南京市", ""},
		{"8.8.8.8", "United States", "California", "", "Google LLC"},
		{"240e:3b7:3272::1", "中国", "广东省", "深圳市", "电信"},
		{"2400:3200::1", "中国", "浙江省", "杭州市", "阿里"},
	} {
		got := s.Lookup(tc.ip)
		if got.IP != tc.ip || got.Country != tc.country || got.Province != tc.province || got.City != tc.city || got.ISP != tc.isp {
			t.Fatalf("%s: %+v", tc.ip, got)
		}
	}
	for _, ip := range []string{"10.0.0.1", "127.0.0.1", "100.64.0.1", "198.51.100.1", "203.0.113.1", "::1", "fc00::1", "2001:db8::1", "fe80::1%en0", "::ffff:192.168.1.1"} {
		got := s.Lookup(ip)
		if got.IP != "" || got.Label != "局域网/保留地址" {
			t.Fatal(ip, got)
		}
	}
	for _, ip := range []string{"bad", "", "1.2.3.999", "8.8.8.8:53"} {
		if got := s.Lookup(ip); got.IP != "" || got.Label != "无效 IP" {
			t.Fatal(ip, got)
		}
	}
	if got := s.Lookup("::ffff:223.5.5.5"); got.IP != "223.5.5.5" {
		t.Fatal(got)
	}
	status := s.Status(context.Background())
	if status.Source != "ip2region" || status.Version != DatabaseCommit[:12] || status.CacheStrategy != "content" || status.MemoryBytes != 48400455 || status.IPv4Date == nil || status.IPv6Date == nil {
		t.Fatal(status)
	}
}
func TestContentQueriesAndCloseAreRaceSafe(t *testing.T) {
	s := testService(t)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Go(func() {
			for j := 0; j < 100; j++ {
				if s.Lookup("240e:3b7:3272::1").City != "深圳市" {
					t.Error("concurrent lookup")
				}
			}
		})
	}
	wg.Wait()
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			for j := 0; j < 50; j++ {
				s.Lookup("223.5.5.5")
				s.Status(context.Background())
			}
		})
	}
	s.Close()
	wg.Wait()
	if s.Status(context.Background()).IPv4 {
		t.Fatal("closed database still marked ready")
	}
}
func TestMissingOrCorruptDataNeverProvidesFalseGeography(t *testing.T) {
	dir := t.TempDir()
	if e := os.WriteFile(filepath.Join(dir, "ip2region_v4.xdb"), []byte("not an xdb"), 0444); e != nil {
		t.Fatal(e)
	}
	s := New(dir)
	defer s.Close()
	status := s.Status(t.Context())
	if status.IPv4 || status.IPv6 || status.Reason == "" {
		t.Fatal(status)
	}
	got := s.Lookup("223.5.5.5")
	if got.IP != "223.5.5.5" || got.Country != "" || got.Label != "归属地暂不可用" {
		t.Fatal(got)
	}
	var missing *Service
	if missing.Lookup("10.0.0.1").IP != "" || missing.Status(t.Context()).Source != "ip2region" {
		t.Fatal("nil fallback")
	}
	script, e := os.ReadFile(filepath.Join("..", "..", "..", "scripts", "prepare-ip2region.sh"))
	if e != nil {
		t.Fatal(e)
	}
	for _, pin := range []string{DatabaseCommit, IPv4SHA256, IPv6SHA256} {
		if !strings.Contains(string(script), pin) {
			t.Fatal("build/runtime data pins differ")
		}
	}
}
