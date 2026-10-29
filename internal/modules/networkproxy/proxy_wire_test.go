package networkproxy

import (
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func socksFixture(t *testing.T, user, password string, hits *atomic.Int32) string {
	t.Helper()
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			c, e := ln.Accept()
			if e != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer c.Close()
				c.SetDeadline(time.Now().Add(3 * time.Second))
				head := make([]byte, 2)
				if _, e := io.ReadFull(c, head); e != nil {
					return
				}
				methods := make([]byte, int(head[1]))
				if _, e := io.ReadFull(c, methods); e != nil {
					return
				}
				if head[0] != 5 {
					return
				}
				c.Write([]byte{5, 2})
				if _, e := io.ReadFull(c, head); e != nil {
					return
				}
				u := make([]byte, int(head[1]))
				if _, e := io.ReadFull(c, u); e != nil {
					return
				}
				size := make([]byte, 1)
				if _, e := io.ReadFull(c, size); e != nil {
					return
				}
				p := make([]byte, int(size[0]))
				if _, e := io.ReadFull(c, p); e != nil {
					return
				}
				if string(u) != user || string(p) != password {
					c.Write([]byte{1, 1})
					return
				}
				c.Write([]byte{1, 0})
				request := make([]byte, 4)
				if _, e := io.ReadFull(c, request); e != nil {
					return
				}
				var ip []byte
				switch request[3] {
				case 1:
					ip = make([]byte, 4)
				case 4:
					ip = make([]byte, 16)
				default:
					return
				}
				if _, e := io.ReadFull(c, ip); e != nil {
					return
				}
				port := make([]byte, 2)
				if _, e := io.ReadFull(c, port); e != nil {
					return
				}
				if !net.IP(ip).IsLoopback() {
					return
				}
				target, e := net.DialTimeout("tcp", net.JoinHostPort(net.IP(ip).String(), strconv.Itoa(int(binary.BigEndian.Uint16(port)))), time.Second)
				if e != nil {
					return
				}
				defer target.Close()
				target.SetDeadline(time.Now().Add(3 * time.Second))
				hits.Add(1)
				c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
				done := make(chan struct{})
				go func() { io.Copy(target, c); close(done) }()
				io.Copy(c, target)
				c.Close()
				<-done
			}()
		}
	}()
	t.Cleanup(func() { ln.Close(); <-stopped; wg.Wait() })
	return "socks5://" + ln.Addr().String()
}
func TestAuthenticatedHTTPAndSOCKS5WireRoutingNoFallback(t *testing.T) {
	const user = "user@ /%"
	const pass = "p@:/?#% owned"
	for _, scheme := range []string{"http", "socks5"} {
		t.Run(scheme, func(t *testing.T) {
			var originHits, proxyHits atomic.Int32
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				originHits.Add(1)
				if r.Header.Get("Proxy-Authorization") != "" {
					t.Error("proxy credential leaked to origin")
				}
				io.WriteString(w, "via-proxy")
			}))
			defer origin.Close()
			var endpoint string
			if scheme == "http" {
				forward := &http.Transport{Proxy: nil}
				defer forward.CloseIdleConnections()
				proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					want := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
					if r.Header.Get("Proxy-Authorization") != want {
						w.WriteHeader(407)
						return
					}
					proxyHits.Add(1)
					req := r.Clone(r.Context())
					req.RequestURI = ""
					req.Header = r.Header.Clone()
					req.Header.Del("Proxy-Authorization")
					response, e := forward.RoundTrip(req)
					if e != nil {
						w.WriteHeader(502)
						return
					}
					defer response.Body.Close()
					w.WriteHeader(response.StatusCode)
					io.Copy(w, response.Body)
				}))
				defer proxy.Close()
				endpoint = proxy.URL
			} else {
				endpoint = socksFixture(t, user, pass, &proxyHits)
			}
			config := recordAt(endpoint)
			config.Username = user
			config.Password = pass
			config.TMDB = true
			client, e := Client(config, "tmdb")
			if e != nil {
				t.Fatal(e)
			}
			defer client.CloseIdleConnections()
			response, e := client.Get(origin.URL + "/proof")
			if e != nil {
				t.Fatal(e)
			}
			body, e := io.ReadAll(response.Body)
			response.Body.Close()
			if e != nil || response.StatusCode != 200 || string(body) != "via-proxy" || originHits.Load() != 1 || proxyHits.Load() != 1 {
				t.Fatal("route proof failed")
			}
			client.CloseIdleConnections()
			config.Password = "wrong"
			rejected, e := Client(config, "tmdb")
			if e != nil {
				t.Fatal(e)
			}
			response, e = rejected.Get(origin.URL + "/bad-auth")
			if response != nil {
				response.Body.Close()
				if response.StatusCode == 200 {
					t.Fatal("bad auth accepted")
				}
			} else if e == nil {
				t.Fatal("missing failure")
			}
			rejected.CloseIdleConnections()
			if originHits.Load() != 1 {
				t.Fatal("authentication failure fell back direct")
			}
			closed, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			dead := recordAt(scheme + "://" + closed.Addr().String())
			closed.Close()
			dead.TMDB = true
			failed, e := Client(dead, "tmdb")
			if e != nil {
				t.Fatal(e)
			}
			if response, e = failed.Get(origin.URL + "/closed-proxy"); e == nil {
				response.Body.Close()
				t.Fatal("closed proxy bypassed")
			}
			failed.CloseIdleConnections()
			if originHits.Load() != 1 {
				t.Fatal("connection failure fell back direct")
			}
			t.Logf("%s authenticated proxy_hits=%d origin_hits=%d; bad-auth and closed-proxy added_origin_hits=0", scheme, proxyHits.Load(), originHits.Load())
		})
	}
}
