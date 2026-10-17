package strm

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/auth"
)

// ParseOptions retains the gateway's managed-origin/key policy. The verifier
// must validate the per-file grant; it must not call the resolver via HTTP.
type ParseOptions struct {
	Builder       Builder
	Origins       []string
	LegacyEnabled bool
	ManagedURL    func(context.Context, string) (storage.FileRef, error)
}

func ParseStable(ctx context.Context, raw string, o ParseOptions) (storage.FileRef, error) {
	u, err := url.Parse(raw)
	if len(raw) > 8192 || err != nil || u.User != nil || u.Fragment != "" || u.Path != Endpoint || u.RawPath != "" || strings.ContainsAny(raw, "\r\n\x00") {
		return storage.FileRef{}, errors.New("not a managed STRM URL")
	}
	origin := u.Scheme + "://" + u.Host
	allowed := origin == strings.TrimRight(o.Builder.BaseURL, "/")
	for _, extra := range o.Origins {
		if origin == strings.TrimRight(extra, "/") {
			allowed = true
		}
	}
	if !allowed {
		return storage.FileRef{}, errors.New("unknown STRM origin")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return storage.FileRef{}, errors.New("invalid STRM query")
	}
	for k, v := range query {
		if (k != "apiKey" && k != "path" && k != "pickCode" && k != "assetId") || len(v) != 1 {
			return storage.FileRef{}, errors.New("ambiguous STRM query")
		}
	}
	if query.Get("assetId") != "" {
		if o.ManagedURL == nil {
			return storage.FileRef{}, errors.New("file-grant lookup unavailable")
		}
		return o.ManagedURL(ctx, raw)
	}
	if !o.LegacyEnabled || !auth.Match(o.Builder.APIKey, query.Get("apiKey")) {
		return storage.FileRef{}, errors.New("unmanaged STRM key")
	}
	ref := storage.FileRef{Path: query.Get("path"), PickCode: query.Get("pickCode")}
	return ref, o.Builder.Validate(ref)
}
