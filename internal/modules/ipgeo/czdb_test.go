package ipgeo

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/aes"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fixtureKey = "MDEyMzQ1Njc4OWFiY2RlZg=="

func mpText(s string) []byte {
	b := []byte(s)
	if len(b) < 32 {
		return append([]byte{byte(0xa0 + len(b))}, b...)
	}
	return append([]byte{0xd9, byte(len(b))}, b...)
}
func fixture(size int) []byte {
	key, _ := base64.StdEncoding.DecodeString(fixtureKey)
	geo := []byte{0x93}
	for _, v := range []string{"中国", "广东", "深圳"} {
		geo = append(geo, mpText(v)...)
	}
	record := []byte{0xce, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(record[1:], uint32(len(geo))<<24)
	record = append(record, mpText("电信")...)
	first := 17 + len(record)
	stride := size*2 + 5
	last := first
	data := make([]byte, first+stride+8+len(geo))
	data[0] = 0
	if size == 16 {
		data[0] = 1
	}
	le.PutUint32(data[1:], uint32(len(data)))
	le.PutUint32(data[5:], uint32(first))
	le.PutUint32(data[13:], uint32(last))
	copy(data[17:], record)
	start, end := netip.MustParseAddr("8.8.8.0"), netip.MustParseAddr("8.8.8.255")
	if size == 16 {
		start, end = netip.MustParseAddr("2606:4700::"), netip.MustParseAddr("2606:4700:ffff:ffff:ffff:ffff:ffff:ffff")
	}
	if size == 4 {
		a, b := start.As4(), end.As4()
		copy(data[first:], a[:])
		copy(data[first+size:], b[:])
	} else {
		a, b := start.As16(), end.As16()
		copy(data[first:], a[:])
		copy(data[first+size:], b[:])
	}
	le.PutUint32(data[first+size*2:], 17)
	data[first+size*2+4] = byte(len(record))
	le.PutUint32(data[first+stride:], 14)
	le.PutUint32(data[first+stride+4:], uint32(len(geo)))
	for i, v := range geo {
		data[first+stride+8+i] = v ^ key[i%len(key)]
	}
	raw := make([]byte, 28+len(data))
	le.PutUint32(raw, 1)
	le.PutUint32(raw[4:], 123)
	le.PutUint32(raw[8:], 16)
	plain := make([]byte, 16)
	le.PutUint32(plain, 123<<20|991231)
	block, _ := aes.NewCipher(key)
	block.Encrypt(raw[12:28], plain)
	copy(raw[28:], data)
	return raw
}
func TestCZDBSyntheticIPv4IPv6BoundsAndAuthorization(t *testing.T) {
	for _, size := range []int{4, 16} {
		raw := fixture(size)
		d, e := Parse(raw, fixtureKey, time.Now())
		if e != nil {
			t.Fatal(e)
		}
		ip := "8.8.8.8"
		if size == 16 {
			ip = "2606:4700::1111"
		}
		v := d.Lookup(netip.MustParseAddr(ip))
		if v.Label != "中国 广东 深圳 电信" || v.Country != "中国" || v.Province != "广东" {
			t.Fatal(v)
		}
		if d.Lookup(netip.MustParseAddr("1.1.1.1")).Label != "" {
			t.Fatal("out of range hit")
		}
		for _, n := range []int{0, 5, 27, 44, len(raw) - 1} {
			if _, e := Parse(raw[:n], fixtureKey, time.Now()); e == nil {
				t.Fatal("truncation accepted", n)
			}
		}
		for _, i := range []int{8, 12, 28 + 1, 28 + 5, 28 + 13, len(raw) - 4} {
			b := bytes.Clone(raw)
			b[i] ^= 255
			if _, e := Parse(b, fixtureKey, time.Now()); e == nil {
				t.Fatal("corruption accepted", i)
			}
		}
		if _, e = Parse(raw, base64.StdEncoding.EncodeToString([]byte("fedcba9876543210")), time.Now()); e == nil {
			t.Fatal("wrong key")
		}
		expired := bytes.Clone(raw)
		k, _ := base64.StdEncoding.DecodeString(fixtureKey)
		block, _ := aes.NewCipher(k)
		plain := make([]byte, 16)
		le.PutUint32(plain, 123<<20|200101)
		block.Encrypt(expired[12:28], plain)
		if _, e = Parse(expired, fixtureKey, time.Now()); e == nil {
			t.Fatal("expired key")
		}
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestAtomicUpdateFallbackLRUAndNoDatabase(t *testing.T) {
	s := New(nil, nil, t.TempDir())
	if v := s.Lookup("8.8.8.8"); v.IP != "8.8.8.8" || v.Label != "" {
		t.Fatal(v)
	}
	if v := s.Lookup("192.168.1.1"); v.IP != "" {
		t.Fatal(v)
	}
	var bundle bytes.Buffer
	z := zip.NewWriter(&bundle)
	for _, size := range []int{4, 16} {
		f, e := z.Create(string(rune('a'+size)) + ".czdb")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write(fixture(size)); e != nil {
			t.Fatal(e)
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	raw := bundle.Bytes()
	s.Client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != "www.cz88.net" || r.URL.Path != "/api/communityIpAuthorization/communityIpDbFile" || r.URL.Query().Get("key") != "PRIVATE_TEST_KEY" {
			t.Fatal("download target")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(raw))}, nil
	})
	c := Credentials{"PRIVATE_TEST_KEY", fixtureKey}
	if e := s.Update(t.Context(), c); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(s.Directory, "current.bundle")
	before, _ := os.ReadFile(path)
	if s.Lookup("8.8.8.8").Label != "中国 广东 深圳 电信" {
		t.Fatal("install/cache invalidation")
	}
	raw = []byte("incomplete")
	if e := s.Update(t.Context(), c); e == nil {
		t.Fatal("invalid update accepted")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) || s.Lookup("8.8.8.8").Label == "" {
		t.Fatal("old library lost")
	}
	s.Client.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://evil.invalid/?key=PRIVATE_TEST_KEY"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	if e := s.Update(context.Background(), c); e == nil || strings.Contains(e.Error(), c.DownloadKey) {
		t.Fatal("redirect/secret leak", e)
	}
}
func FuzzParseCZDB(f *testing.F) {
	f.Add(fixture(4))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = Parse(b, fixtureKey, time.Now()) })
}
