package networkproxy

import (
	"bufio"
	"context"
	"io"
	"media-tool/internal/modules/playback"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func publicTestDNS(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
}
func TestPinnedHTTPProxyPreservesHostWithoutRemoteDNSOrCredentialLeak(t *testing.T) {
	var connects, gets atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connects.Add(1)
		if r.Method != "CONNECT" || r.Host != "93.184.216.34:80" || r.Header.Get("Proxy-Authorization") == "" {
			t.Error("un-pinned CONNECT", r.Method, r.Host)
		}
		c, rw, e := w.(http.Hijacker).Hijack()
		if e != nil {
			t.Error(e)
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(time.Second))
		_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = rw.Flush()
		inner, e := http.ReadRequest(rw.Reader)
		if e != nil {
			t.Error(e)
			return
		}
		gets.Add(1)
		if inner.Host != "public.fixture" || inner.URL.IsAbs() || inner.Header.Get("Proxy-Authorization") != "" || inner.Header.Get("Cookie") != "" || inner.Header.Get("Authorization") != "" {
			t.Error("authority or credential leak", inner.Host, inner.Header)
		}
		_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 2\r\n\r\nok")
	}))
	defer proxy.Close()
	client, e := Client(Record{URL: strings.Replace(proxy.URL, "://", "://test:private@", 1)}, "plugin")
	if e != nil {
		t.Fatal(e)
	}
	transport := client.Transport.(*http.Transport)
	defer transport.CloseIdleConnections()
	if e = pinHostTransport(transport, playback.Policy{Lookup: publicTestDNS, AllowHTTP: true}, strings.Replace(proxy.URL, "://", "://test:private@", 1)); e != nil {
		t.Fatal(e)
	}
	response, e := client.Get("http://public.fixture/safe")
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	raw, e := io.ReadAll(response.Body)
	if e != nil || string(raw) != "ok" || connects.Load() != 1 || gets.Load() != 1 {
		t.Fatal(string(raw), connects.Load(), gets.Load(), e)
	}
}
func TestProxyRebindingMixedDNSAndConnectRefusalNeverFallBack(t *testing.T) {
	var hits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.WriteHeader(403) }))
	defer proxy.Close()
	for _, answer := range [][]net.IPAddr{{{IP: net.ParseIP("127.0.0.1")}}, {{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("169.254.169.254")}}, {{IP: net.ParseIP("::ffff:127.0.0.1")}}, {{IP: net.ParseIP("64:ff9b::7f00:1")}}} {
		t.Run(answer[0].IP.String(), func(t *testing.T) {
			calls := 0
			p := playback.Policy{AllowHTTP: true, Lookup: func(context.Context, string) ([]net.IPAddr, error) {
				calls++
				if calls == 1 {
					return publicTestDNS(context.Background(), "")
				}
				return answer, nil
			}}
			if e := p.Check(context.Background(), "http://public.fixture"); e != nil {
				t.Fatal(e)
			}
			client, _ := Client(Record{URL: proxy.URL}, "plugin")
			tr := client.Transport.(*http.Transport)
			defer tr.CloseIdleConnections()
			if e := pinHostTransport(tr, p, proxy.URL); e != nil {
				t.Fatal(e)
			}
			if _, e := client.Get("http://public.fixture/"); e == nil {
				t.Fatal("split-view DNS accepted")
			}
		})
	}
	if hits.Load() != 0 {
		t.Fatal("proxy used before DNS fence", hits.Load())
	}
	client, _ := Client(Record{URL: proxy.URL}, "plugin")
	tr := client.Transport.(*http.Transport)
	defer tr.CloseIdleConnections()
	_ = pinHostTransport(tr, playback.Policy{Lookup: publicTestDNS, AllowHTTP: true}, proxy.URL)
	if _, e := client.Get("http://public.fixture/"); e == nil || hits.Load() != 1 {
		t.Fatal("CONNECT refusal bypassed", e, hits.Load())
	}
}
func TestSOCKS5AndSOCKS5HReceiveOnlyPinnedIPAddress(t *testing.T) {
	for _, scheme := range []string{"socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
			listener, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			defer listener.Close()
			done := make(chan error, 1)
			go func() {
				c, e := listener.Accept()
				if e != nil {
					done <- e
					return
				}
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(2 * time.Second))
				r := bufio.NewReader(c)
				head := make([]byte, 2)
				if _, e = io.ReadFull(r, head); e != nil {
					done <- e
					return
				}
				methods := make([]byte, int(head[1]))
				_, e = io.ReadFull(r, methods)
				if e != nil {
					done <- e
					return
				}
				_, _ = c.Write([]byte{5, 0})
				request := make([]byte, 10)
				_, e = io.ReadFull(r, request)
				if e != nil {
					done <- e
					return
				}
				if request[3] != 1 || net.IP(request[4:8]).String() != "93.184.216.34" {
					t.Error("SOCKS remote DNS used", request)
				}
				_, _ = c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
				inner, e := http.ReadRequest(r)
				if e == nil && inner.Host != "public.fixture" {
					t.Error(inner.Host)
				}
				if e == nil {
					_, e = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
				}
				done <- e
			}()
			raw := scheme + "://" + listener.Addr().String()
			client, _ := Client(Record{URL: raw}, "plugin")
			tr := client.Transport.(*http.Transport)
			defer tr.CloseIdleConnections()
			_ = pinHostTransport(tr, playback.Policy{Lookup: publicTestDNS, AllowHTTP: true}, raw)
			response, e := client.Get("http://public.fixture/safe")
			if e != nil {
				t.Fatal(e)
			}
			_ = response.Body.Close()
			if e = <-done; e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestProxyHandshakeIsBoundedAndHonorsCancellation(t *testing.T) {
	for _, oversized := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "large_header"}[oversized], func(t *testing.T) {
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c, rw, e := w.(http.Hijacker).Hijack()
				if e != nil {
					return
				}
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(time.Second))
				if oversized {
					_, _ = rw.WriteString("HTTP/1.1 200 OK\r\nX-Oversized: " + strings.Repeat("a", 40<<10) + "\r\n\r\n")
					_ = rw.Flush()
				}
				buf := make([]byte, 1)
				_, _ = c.Read(buf)
			}))
			defer proxy.Close()
			client, _ := Client(Record{URL: proxy.URL}, "plugin")
			tr := client.Transport.(*http.Transport)
			defer tr.CloseIdleConnections()
			_ = pinHostTransport(tr, playback.Policy{Lookup: publicTestDNS, AllowHTTP: true}, proxy.URL)
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			request, _ := http.NewRequestWithContext(ctx, "GET", "http://public.fixture/", nil)
			start := time.Now()
			if response, e := client.Do(request); e == nil {
				_ = response.Body.Close()
				t.Fatal("invalid handshake accepted")
			}
			if time.Since(start) > time.Second {
				t.Fatal("cancellation ignored")
			}
		})
	}
}

func TestFaviconPublicClientPinsDirectAndProxyTargetDNS(t *testing.T) {
	for _, config := range []Record{{}, {URL: "http://127.0.0.1:9", Sites: true}} {
		p := playback.Policy{AllowHTTP: true, Lookup: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		}}
		client, e := PublicClient(config, "ptsite", p)
		if e != nil {
			t.Fatal(e)
		}
		defer client.CloseIdleConnections()
		if client.Transport.(*http.Transport).Proxy != nil {
			t.Fatal("unpinned proxy lookup")
		}
		if response, e := client.Get("http://tracker.example/favicon.ico"); e == nil {
			response.Body.Close()
			t.Fatal("private target permitted")
		}
	}
}
