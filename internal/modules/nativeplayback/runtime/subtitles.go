package runtime

import (
	"bytes"
	"context"
	"errors"
	"media-tool/internal/adapters/open115"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/nativeplayback"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/subtitles"
	"media-tool/internal/platform/mediaext"
)

type subtitle struct {
	entry                    *subtitles.Entry
	id, name, fullPath, pick string
	size                     int64
	updated                  time.Time
}

const subtitleSQL = subtitles.AutoSQL

func subtitleMatches(video, sub string) bool { return subtitles.Matches(video, sub) }
func (s *Service) subtitleGroups(ctx context.Context, assets []string) (map[string][]subtitle, error) {
	repo := s.Subtitles
	if repo.DB == nil {
		repo.DB = s.DB
	}
	entries, e := repo.Groups(ctx, assets, mediaext.SubtitleExtensions())
	if e != nil {
		return nil, nativeplayback.ErrUpstream
	}
	out := map[string][]subtitle{}
	for asset, list := range entries {
		for _, v := range list {
			if v.Hidden || v.Deleting {
				continue
			}
			id := v.ID
			if v.Source == "automatic" {
				id = v.SourceKey
			}
			entry := v
			out[asset] = append(out[asset], subtitle{id: id, name: v.Filename, entry: &entry})
		}
	}
	return out, nil
}
func (s *Service) subtitles(ctx context.Context, asset string) ([]subtitle, error) {
	v, e := s.subtitleGroups(ctx, []string{asset})
	return v[asset], e
}

func subtitleStream(v subtitle, index int, origin, item, source, token string) (dto.MediaStream, error) {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(v.name), "."))
	lower := strings.ToLower(v.name)
	language := ""
	for _, lang := range []string{"zh", "chs", "cht", "zh-cn", "zh-tw", "eng", "en", "jpn", "ja"} {
		if strings.Contains(lower, "."+lang+".") {
			language = lang
			break
		}
	}
	title := v.name
	forced := strings.Contains(lower, ".forced.") || strings.Contains(lower, ".foreign.")
	def := strings.Contains(lower, ".default.")
	if v.entry != nil {
		language = v.entry.Language
		title = v.entry.Title
		forced = v.entry.Forced
		def = v.entry.Default
		ext = v.entry.Extension
	}
	target, e := nativeplayback.SubtitleURL(origin, item, source, token, v.id, index, ext)
	if e != nil {
		return dto.MediaStream{}, e
	}
	return dto.MediaStream{Type: "Subtitle", Codec: ext, Language: language, DisplayTitle: title, Index: &index, IsExternal: true, DeliveryMethod: "External", DeliveryUrl: target, IsTextSubtitleStream: ext != "sub" && ext != "idx" && ext != "sup", IsForced: forced, IsDefault: def, IsHearingImpaired: strings.Contains(lower, ".sdh.")}, nil
}
func addSubtitles(a nativeplayback.Actor, item string, v *source, subs []subtitle) error {
	for _, sub := range subs {
		if sub.entry != nil && sub.entry.Source == "embedded" {
			if sub.entry.Overridden {
				for i := range v.MediaStreams {
					m := &v.MediaStreams[i]
					if m.Type == "Subtitle" && m.Index != nil && *m.Index == sub.entry.Index {
						m.Language = sub.entry.Language
						m.DisplayTitle = sub.entry.Title
						m.Title = sub.entry.Title
						m.IsDefault = sub.entry.Default
						m.IsForced = sub.entry.Forced
					}
				}
			}
			continue
		}
		index := 65536
		if sub.entry != nil {
			index = sub.entry.Index
		}
		stream, e := subtitleStream(sub, index, a.Origin, item, v.Id, a.Token)
		if e != nil {
			return nativeplayback.ErrUpstream
		}
		v.MediaStreams = append(v.MediaStreams, stream)
	}
	v.DefaultSubtitleStreamIndex = nil
	for _, stream := range v.MediaStreams {
		if stream.Type == "Subtitle" && stream.IsDefault && stream.Index != nil {
			index := *stream.Index
			v.DefaultSubtitleStreamIndex = &index
			break
		}
	}
	return nil
}
func (s *Service) subtitleTarget(ctx context.Context, a nativeplayback.Actor, q nativeplayback.Request, v source) (nativeplayback.RedirectTarget, error) {
	// Revalidate the per-asset registry identity; hiding/deletion never remaps an index.
	subs, e := s.subtitles(ctx, v.Id)
	if e != nil {
		return nativeplayback.RedirectTarget{}, e
	}
	var sub subtitle
	found := false
	for _, candidate := range subs {
		if candidate.entry != nil && candidate.entry.Source != "embedded" && candidate.entry.Index == *q.SubtitleIndex {
			sub = candidate
			found = true
			break
		}
	}
	if !found || q.SubtitleID != "" && q.SubtitleID != sub.id {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrNotFound
	}
	ext := sub.entry.Extension
	if q.SubtitleFormat != "" && q.SubtitleFormat != ext {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrSubtitleConversion
	}
	if sub.entry.Source == "uploaded" {
		repo := s.Subtitles
		if repo.DB == nil {
			repo.DB = s.DB
		}
		entry, body, err := repo.Read(ctx, v.Id, sub.entry.ID)
		if err != nil {
			return nativeplayback.RedirectTarget{}, nativeplayback.ErrNotFound
		}
		latest, err := s.sources(ctx, a, q)
		if err == nil {
			_, err = selectSource(latest, v.Id, false)
		}
		if err != nil {
			return nativeplayback.RedirectTarget{}, err
		}
		if err = s.subtitleVisible(ctx, v.Id, sub.entry.ID); err != nil {
			return nativeplayback.RedirectTarget{}, err
		}
		text, err := nativeplayback.NewTextSubtitle(entry.Extension, body)
		if err != nil {
			return nativeplayback.RedirectTarget{}, nativeplayback.ErrUpstream
		}
		return nativeplayback.RedirectTarget{Subtitle: text}, nil
	}
	links, ok := s.Links.(interface {
		ResolveSubtitle(context.Context, string, string, string, string) (nativeplayback.RedirectTarget, error)
	})
	if !ok {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrUpstream
	}
	target, e := links.ResolveSubtitle(ctx, v.Id, sub.id, a.UserAgent, a.Address.ClientIP)
	if e == nil {
		latest, err := s.sources(ctx, a, q)
		e = err
		if e == nil {
			_, e = selectSource(latest, v.Id, false)
		}
	} // Recheck permission and visibility after provider I/O.
	if e == nil {
		e = s.subtitleVisible(ctx, v.Id, sub.entry.ID)
	}
	return target, e
}
func (r *Resolver) ResolveSubtitle(parent context.Context, asset, id, ua, ip string) (nativeplayback.RedirectTarget, error) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	evidence, e := r.load(ctx, asset)
	if e != nil {
		return nativeplayback.RedirectTarget{}, e
	}
	load := func() (subtitle, error) {
		var v subtitle
		rows, e := r.DB.Query(ctx, subtitleSQL, asset, id, mediaext.SubtitleExtensions())
		if e != nil {
			return v, e
		}
		defer rows.Close()
		if !rows.Next() {
			return v, pgx.ErrNoRows
		}
		e = rows.Scan(&v.id, &v.fullPath, &v.pick, &v.size, &v.updated)
		return v, e
	}
	sub, e := load()
	if e != nil || !subtitleMatches(evidence.Path, sub.fullPath) {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrNotFound
	}
	if r.Source == nil {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrUpstream
	}
	link, e := r.Source.Resolve(ctx, storage.FileRef{CloudFileID: sub.id, Path: sub.fullPath, PickCode: sub.pick, ExpectedSize: sub.size, UserAgent: ua, ClientIP: ip})
	if e != nil {
		var retry *nativeplayback.RetryError
		if errors.As(e, &retry) {
			return nativeplayback.RedirectTarget{}, retry
		}
		var api *open115.APIError
		if errors.Is(e, open115.ErrRateLimited) || errors.As(e, &api) && api.RateLimited() {
			return nativeplayback.RedirectTarget{}, &nativeplayback.RetryError{After: time.Minute}
		}
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrUpstream
	}
	target, e := url.Parse(link.URL)
	if e != nil || len(link.URL) > 8192 || len(link.Headers) > 0 || link.UserAgent != "" && link.UserAgent != ua || !link.ExpiresAt.IsZero() && time.Until(link.ExpiresAt) < 5*time.Second || !link.CacheUntil.IsZero() && time.Until(link.CacheUntil) < 5*time.Second || target.Scheme != "https" && target.Scheme != "http" {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrUpstream
	}
	if e = r.Policy.Check(ctx, link.URL); e != nil {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrUpstream
	}
	latest, e := load()
	if e != nil || latest != sub {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrNotFound
	}
	latestAsset, e := r.load(ctx, asset)
	if e != nil {
		return nativeplayback.RedirectTarget{}, e
	}
	if latestAsset.Path != evidence.Path || latestAsset.Pick != evidence.Pick || latestAsset.Revision != evidence.Revision || latestAsset.Output != evidence.Output || !bytes.Equal(latestAsset.Digest, evidence.Digest) {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrNotFound
	}
	return nativeplayback.RedirectTarget{URL: link.URL, ExpiresAt: link.ExpiresAt}, nil
}

func (s *Service) subtitleVisible(ctx context.Context, asset, id string) error {
	repo := s.Subtitles
	if repo.DB == nil {
		repo.DB = s.DB
	}
	v, e := repo.Get(ctx, asset, id)
	if e != nil || v.Hidden || v.Deleting {
		return nativeplayback.ErrNotFound
	}
	return nil
}
