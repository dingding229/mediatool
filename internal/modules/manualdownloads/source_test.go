package manualdownloads

import (
	"context"
	"io"
	"media-tool/internal/modules/playback"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestMagnetSourceValidatesHashes(t *testing.T) {
	for _, raw := range []string{"magnet:?xt=urn:btih:" + strings.Repeat("a", 40) + "&dn=Owned", "magnet:?xt=urn:btih:" + strings.Repeat("A", 32), "magnet:?xt=urn:btmh:1220" + strings.Repeat("b", 64)} {
		meta, err := Magnet(raw)
		if err != nil || len(meta.InfoHash) != 40 {
			t.Fatal(meta, err)
		}
	}
	for _, raw := range []string{"https://example.com", "magnet:?dn=invalid", "magnet:?xt=urn:btih:bad", "magnet:?xt=urn:btih:" + strings.Repeat("z", 40), "magnet://private?xt=urn:btih:" + strings.Repeat("a", 40)} {
		if _, err := Magnet(raw); err == nil {
			t.Fatal(raw)
		}
	}
}
func TestLinkRejectsSSRFAndFakeTorrent(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1:8080/torrent", "http://169.254.169.254/a", "http://192.168.1.2/a", "file:///etc/passwd", "https://user:pass@example.com/a"} {
		if _, err := Link(context.Background(), raw); err == nil {
			t.Fatal(raw)
		}
	}
	for _, raw := range [][]byte{nil, []byte("<html>not a torrent</html>"), []byte(strings.Repeat("x", MaxTorrent+1))} {
		if _, err := File(raw, "test.torrent"); err == nil {
			t.Fatal("accepted fake source")
		}
	}
}

type torrentTransport func(*http.Request) (*http.Response, error)

func (f torrentTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestHTTPSourceParsesTorrentAndRejectsUnsafeRedirect(t *testing.T) {
	ctx := context.Background()
	policy := playback.Policy{AllowHTTP: true, Lookup: func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("1.1.1.1")}}, nil
	}}
	raw := "d4:infod6:lengthi1e4:name9:owned.mkv12:piece lengthi16384e6:pieces20:" + strings.Repeat("a", 20) + "ee"
	for _, test := range []struct {
		body, location string
		large, valid   bool
	}{{body: raw, valid: true}, {body: "<html>login</html>"}, {body: raw, large: true}, {location: "http://127.0.0.1/private"}} {
		client := torrentClient(policy)
		client.Transport = torrentTransport(func(r *http.Request) (*http.Response, error) {
			response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(test.body)), Request: r, ContentLength: int64(len(test.body))}
			if test.large {
				response.ContentLength = MaxTorrent + 1
			}
			if test.location != "" {
				response.StatusCode = 302
				response.Header.Set("Location", test.location)
			}
			return response, nil
		})
		source, err := fetchTorrent(ctx, "https://tracker.example.com/download?passkey=fixture", policy, client)
		if test.valid {
			if err != nil || source.Kind != "url" || source.Meta.Name != "owned.mkv" || len(source.Torrent) == 0 {
				t.Fatal(source, err)
			}
		} else if err == nil {
			t.Fatal("invalid HTTP source accepted", test)
		}
	}
}
