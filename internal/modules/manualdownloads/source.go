package manualdownloads

import (
	"context"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"io"
	"media-tool/internal/modules/downloads"
	"media-tool/internal/modules/playback"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const MaxTorrent = 10 << 20

var ErrSource = errors.New("种子无效：请选择不超过10MiB的torrent文件、HTTP(S)种子链接或带有效info hash的磁力链接")

type Source struct {
	Kind, Label, URL string
	Torrent          []byte
	Meta             downloads.TorrentMetadata
}

func Magnet(raw string) (downloads.TorrentMetadata, error) {
	if len(raw) > 8192 || strings.ContainsAny(raw, "\r\n\x00") {
		return downloads.TorrentMetadata{}, ErrSource
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "magnet" || u.Host != "" || u.Fragment != "" {
		return downloads.TorrentMetadata{}, ErrSource
	}
	aliases := []string{}
	for _, xt := range u.Query()["xt"] {
		xt = strings.ToLower(xt)
		hash := ""
		if strings.HasPrefix(xt, "urn:btih:") {
			hash = strings.TrimPrefix(xt, "urn:btih:")
			if len(hash) == 32 {
				b, e := base32.StdEncoding.DecodeString(strings.ToUpper(hash))
				if e != nil {
					return downloads.TorrentMetadata{}, ErrSource
				}
				hash = hex.EncodeToString(b)
			}
			if len(hash) != 40 {
				return downloads.TorrentMetadata{}, ErrSource
			}
		} else if strings.HasPrefix(xt, "urn:btmh:1220") {
			hash = strings.TrimPrefix(xt, "urn:btmh:1220")
			if len(hash) != 64 {
				return downloads.TorrentMetadata{}, ErrSource
			}
		} else {
			continue
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return downloads.TorrentMetadata{}, ErrSource
		}
		aliases = append(aliases, hash)
	}
	if len(aliases) == 0 {
		return downloads.TorrentMetadata{}, ErrSource
	}
	name := u.Query().Get("dn")
	if name == "" {
		name = "磁力任务 " + aliases[0][:12]
	}
	if len(name) > 1024 || strings.ContainsAny(name, "\r\n\x00") {
		return downloads.TorrentMetadata{}, ErrSource
	}
	return downloads.TorrentMetadata{InfoHash: aliases[0][:40], Aliases: aliases, Name: name}, nil
}
func File(raw []byte, label string) (Source, error) {
	meta, err := downloads.ParseTorrent(raw)
	if err != nil {
		return Source{}, ErrSource
	}
	if label == "" {
		label = meta.Name + ".torrent"
	}
	if len(label) > 8192 {
		return Source{}, ErrSource
	}
	return Source{Kind: "file", Label: label, Torrent: raw, Meta: meta}, nil
}
func Link(ctx context.Context, raw string) (Source, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "magnet:") {
		meta, err := Magnet(raw)
		return Source{Kind: "magnet", Label: raw, URL: raw, Meta: meta}, err
	}
	if len(raw) > 8192 {
		return Source{}, ErrSource
	}
	policy := playback.Policy{AllowHTTP: true}
	client := torrentClient(policy)
	defer client.CloseIdleConnections()
	return fetchTorrent(ctx, raw, policy, client)
}
func torrentClient(policy playback.Policy) *http.Client {
	return &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: nil, DialContext: policy.DialContext, TLSHandshakeTimeout: 5 * time.Second}, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 3 {
			return ErrSource
		}
		return policy.Check(r.Context(), r.URL.String())
	}}
}
func fetchTorrent(ctx context.Context, raw string, policy playback.Policy, client *http.Client) (Source, error) {
	if err := policy.Check(ctx, raw); err != nil {
		return Source{}, ErrSource
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return Source{}, ErrSource
	}
	req.Header.Set("User-Agent", "MediaTool/1.0 torrent-import")
	resp, err := client.Do(req)
	if err != nil {
		return Source{}, errors.New("种子链接获取失败，请检查公网链接、认证和代理限制；不接受私网目标")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.ContentLength > MaxTorrent {
		return Source{}, ErrSource
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, MaxTorrent+1))
	if err != nil || len(b) > MaxTorrent {
		return Source{}, ErrSource
	}
	source, err := File(b, raw)
	source.Kind = "url"
	return source, err
}
