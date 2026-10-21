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
	"media-tool/internal/platform/mediaext"
)

type subtitle struct {
	id, name, fullPath, pick string
	size                     int64
	updated                  time.Time
}

const subtitleSQL = `SELECT f.id::text,f.full_path,coalesce(f.pick_code,''),f.size_bytes,f.updated_at FROM media_assets a JOIN cloud_files v ON v.id=a.cloud_file_id JOIN cloud_files f ON f.storage_id=v.storage_id AND regexp_replace(f.full_path,'/[^/]+$','')=regexp_replace(v.full_path,'/[^/]+$','') WHERE a.id=$1::uuid AND f.state='present' AND f.id<>v.id AND ('.'||lower(regexp_replace(f.full_path,'^.*\.','')))=ANY($3::text[]) AND left(lower(f.full_path),length(regexp_replace(v.full_path,'\.[^/.]+$',''))+1)=lower(regexp_replace(v.full_path,'\.[^/.]+$',''))||'.' AND ($2::text='' OR f.id::text=$2) ORDER BY f.full_path,f.id LIMIT 201`

// A dot-delimited suffix belongs to this exact video's basename, never E01 -> E010.
func subtitleMatches(video, sub string) bool {
	if path.Dir(video) != path.Dir(sub) || !mediaext.IsSubtitle(sub) {
		return false
	}
	stem := strings.ToLower(strings.TrimSuffix(path.Base(video), path.Ext(video)))
	name := strings.ToLower(strings.TrimSuffix(path.Base(sub), path.Ext(sub)))
	return name == stem || strings.HasPrefix(name, stem+".")
}
func (s *Service) subtitleGroups(ctx context.Context, assets []string) (map[string][]subtitle, error) {
	out := map[string][]subtitle{}
	// One bounded query per PlaybackInfo, not two queries per physical version.
	sql := strings.Replace(subtitleSQL, "SELECT f.id::text,", "SELECT a.id::text,v.full_path,f.id::text,", 1)
	sql = strings.Replace(sql, "a.id=$1::uuid", "a.id=ANY($1::uuid[])", 1)
	sql = strings.Replace(sql, "ORDER BY f.full_path,f.id LIMIT 201", "ORDER BY a.id,f.full_path,f.id LIMIT 20001", 1)
	rows, e := s.DB.Query(ctx, sql, assets, "", mediaext.SubtitleExtensions())
	if e != nil {
		return nil, nativeplayback.ErrUpstream
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var asset, video string
		var v subtitle
		count++
		if e = rows.Scan(&asset, &video, &v.id, &v.fullPath, &v.pick, &v.size, &v.updated); e != nil {
			return nil, nativeplayback.ErrUpstream
		}
		if subtitleMatches(video, v.fullPath) {
			v.name = path.Base(v.fullPath)
			out[asset] = append(out[asset], v)
			if len(out[asset]) > 200 {
				return nil, nativeplayback.ErrUpstream
			}
		}
	}
	if count > 20000 || rows.Err() != nil {
		return nil, nativeplayback.ErrUpstream
	}
	return out, nil
}
func (s *Service) subtitles(ctx context.Context, asset string) ([]subtitle, error) {
	groups, e := s.subtitleGroups(ctx, []string{asset})
	return groups[asset], e
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
	target, e := nativeplayback.SubtitleURL(origin, item, source, token, v.id, index, ext)
	if e != nil {
		return dto.MediaStream{}, e
	}
	return dto.MediaStream{Type: "Subtitle", Codec: ext, Language: language, DisplayTitle: v.name, Index: &index, IsExternal: true, DeliveryMethod: "External", DeliveryUrl: target, IsTextSubtitleStream: ext != "sub" && ext != "idx" && ext != "sup", IsForced: strings.Contains(lower, ".forced.") || strings.Contains(lower, ".foreign."), IsDefault: strings.Contains(lower, ".default."), IsHearingImpaired: strings.Contains(lower, ".sdh.")}, nil
}
func addSubtitles(a nativeplayback.Actor, item string, v *source, subs []subtitle) error {
	index := 0
	for _, stream := range v.MediaStreams {
		if stream.Index != nil {
			index = max(index, *stream.Index+1)
		}
	}
	for _, sub := range subs {
		stream, e := subtitleStream(sub, index, a.Origin, item, v.Id, a.Token)
		if e != nil {
			return nativeplayback.ErrUpstream
		}
		v.MediaStreams = append(v.MediaStreams, stream)
		index++
	}
	return nil
}
func (s *Service) subtitleTarget(ctx context.Context, a nativeplayback.Actor, q nativeplayback.Request, v source) (nativeplayback.RedirectTarget, error) {
	// Rebuild the same deterministic list, including embedded stream offsets.
	subs, e := s.subtitles(ctx, v.Id)
	if e != nil {
		return nativeplayback.RedirectTarget{}, e
	}
	first := 0
	for _, stream := range v.MediaStreams {
		if stream.Index != nil {
			first = max(first, *stream.Index+1)
		}
	}
	n := *q.SubtitleIndex - first
	if n < 0 || n >= len(subs) {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrNotFound
	}
	sub := subs[n]
	if q.SubtitleID != "" && q.SubtitleID != sub.id {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrNotFound
	}
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(sub.name)), ".")
	if q.SubtitleFormat != "" && q.SubtitleFormat != ext {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrSubtitleConversion
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
	} // Recheck permission after provider I/O.
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
