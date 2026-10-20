package networkproxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"golang.org/x/net/proxy"
	"io"
	"media-tool/internal/modules/playback"
	"net"
	"net/http"
	"net/url"
	"time"
)

// The host profile is public-origin-only even with a configured proxy. Do not
// let a proxy perform a second, split-view DNS lookup of the target hostname.
func pinHostTransport(t *http.Transport, p playback.Policy, raw string) error {
	t.Proxy = nil
	if raw == "" {
		t.DialContext = p.DialContext
		return nil
	}
	u, e := url.Parse(raw)
	if e != nil {
		return ErrInvalid
	}
	t.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, playback.ErrUnsafeTarget
		}
		ips, e := p.Addresses(ctx, host)
		if e != nil {
			return nil, e
		}
		var last error
		for _, ip := range ips {
			c, e := proxyTunnel(ctx, u, network, net.JoinHostPort(ip.IP.String(), port))
			if e == nil {
				return c, nil
			}
			last = e
		}
		return nil, last
	}
	return nil
}

type tunnelConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *tunnelConn) Read(b []byte) (int, error) { return c.reader.Read(b) }
func proxyTunnel(ctx context.Context, u *url.URL, network, target string) (net.Conn, error) {
	port := u.Port()
	if port == "" {
		port = "80"
		switch u.Scheme {
		case "https":
			port = "443"
		case "socks5", "socks5h":
			port = "1080"
		}
	}
	endpoint := net.JoinHostPort(u.Hostname(), port)
	if u.Scheme == "socks5" || u.Scheme == "socks5h" {
		var auth *proxy.Auth
		if u.User != nil {
			pass, _ := u.User.Password()
			auth = &proxy.Auth{User: u.User.Username(), Password: pass}
		}
		d, e := proxy.SOCKS5("tcp", endpoint, auth, &net.Dialer{Timeout: 5 * time.Second})
		if e != nil {
			return nil, e
		}
		cd, ok := d.(proxy.ContextDialer)
		if !ok {
			return nil, ErrHostUnsupported
		}
		// target contains a validated literal IP, including for socks5h.
		return cd.DialContext(ctx, network, target)
	}
	c, e := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", endpoint)
	if e != nil {
		return nil, e
	}
	ok := false
	defer func() {
		if !ok {
			_ = c.Close()
		}
	}()
	rawConn := c
	stop := context.AfterFunc(ctx, func() { _ = rawConn.Close() })
	defer stop()
	if e = c.SetDeadline(time.Now().Add(5 * time.Second)); e != nil {
		return nil, e
	}
	if u.Scheme == "https" {
		tc := tls.Client(c, &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12})
		if e = tc.HandshakeContext(ctx); e != nil {
			return nil, e
		}
		c = tc
	}
	request := &http.Request{Method: "CONNECT", URL: &url.URL{Opaque: target}, Host: target, Header: make(http.Header)}
	if u.User != nil {
		pass, _ := u.User.Password()
		request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(u.User.Username()+":"+pass)))
	}
	if e = request.Write(c); e != nil {
		return nil, e
	}
	limited := &io.LimitedReader{R: c, N: 32 << 10}
	reader := bufio.NewReader(limited)
	response, e := http.ReadResponse(reader, request)
	if e != nil {
		return nil, errors.New("invalid proxy CONNECT response")
	}
	if response.StatusCode != 200 {
		return nil, errors.New("proxy CONNECT refused")
	}
	// Keep any buffered tunnel bytes; only the handshake headers have a 32KiB cap.
	limited.N = 1 << 62
	if e = c.SetDeadline(time.Time{}); e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	ok = true
	return &tunnelConn{Conn: c, reader: reader}, nil
}
