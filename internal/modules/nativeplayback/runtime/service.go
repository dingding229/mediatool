package runtime

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
	"media-tool/internal/modules/nativeplayback"
	"media-tool/internal/modules/playbackstats"
	"media-tool/internal/modules/playeraccess"
	"media-tool/internal/modules/subtitles"
	"strconv"
	"strings"
	"time"
)

type Links interface {
	Resolve(context.Context, string, string, string) (nativeplayback.RedirectTarget, error)
}
type Service struct {
	Subtitles subtitles.Repository
	Stats     playbackstats.Sink
	DB        *pgxpool.Pool
	Browse    nativelibrary.BrowseService
	Links     Links
}
type source struct {
	dto.MediaSourceInfo
	play, download bool
	item           dto.BaseItemDto
}

func (s *Service) sources(ctx context.Context, a nativeplayback.Actor, q nativeplayback.Request) ([]source, error) {
	if n, e := strconv.ParseInt(q.ItemID, 10, 64); e != nil || n < 1 {
		return nil, nativeplayback.ErrNotFound
	}
	if s.DB == nil || s.Browse == nil || a.Permissions == nil {
		return nil, nativeplayback.ErrUpstream
	}
	if !a.User.Enabled || a.User.MustChangePassword || a.User.ExpiresAt != nil && !a.User.ExpiresAt.After(time.Now()) {
		return nil, nativeplayback.ErrForbidden
	}
	items, _, err := s.Browse.Query(ctx, a.User, a.Permissions, nativelibrary.ItemQuery{IDs: []string{q.ItemID}, Limit: 1})
	if errors.Is(err, nativelibrary.ErrNotFound) {
		return nil, nativeplayback.ErrNotFound
	}
	if err != nil {
		if errors.Is(err, contracts.ErrForbidden) || errors.Is(err, contracts.ErrUnauthenticated) {
			return nil, nativeplayback.ErrForbidden
		}
		return nil, nativeplayback.ErrUpstream
	}
	if len(items) != 1 || items[0].Type != dto.Movie && items[0].Type != dto.Episode {
		return nil, nativeplayback.ErrNotFound
	}
	ids := []string{}
	for _, v := range items[0].MediaSources {
		ids = append(ids, v.Id)
	}
	visible, err := a.Permissions.VisibleLibraries(ctx, a.User)
	if err != nil {
		return nil, nativeplayback.ErrUpstream
	}
	rows, err := s.DB.Query(ctx, `WITH eligible AS (`+catalog.ExposableAssetsSQL+`) SELECT e.asset_id::text,l.id FROM eligible e JOIN native_asset_libraries a ON a.asset_id=e.asset_id JOIN native_libraries l ON l.id=a.library_id WHERE e.asset_id=ANY($1::uuid[]) AND l.id=ANY($2::text[]) AND NOT l.hidden LIMIT 20000`, ids, visible)
	if err != nil {
		return nil, nativeplayback.ErrUpstream
	}
	type membership struct{ id, lib string }
	members := []membership{}
	for rows.Next() {
		var v membership
		if err = rows.Scan(&v.id, &v.lib); err != nil {
			break
		}
		members = append(members, v)
	}
	err = errors.Join(err, rows.Err())
	rows.Close()
	if err != nil {
		return nil, nativeplayback.ErrUpstream
	}
	grants := map[string][2]bool{}
	for _, v := range members {
		id, lib := v.id, v.lib
		if e := a.Permissions.CanBrowse(ctx, a.User, lib); e != nil {
			if errors.Is(e, contracts.ErrUnavailable) {
				err = e
				break
			}
			continue
		}
		g := grants[id]
		if e := a.Permissions.CanPlay(ctx, a.User, lib); e == nil {
			g[0] = true
		} else if !errors.Is(e, contracts.ErrForbidden) {
			err = e
			break
		}
		if q.Download {
			if e := a.Permissions.CanDownload(ctx, a.User, lib); e == nil {
				g[1] = true
			} else if !errors.Is(e, contracts.ErrForbidden) {
				err = e
				break
			}
		}
		grants[id] = g
	}
	if err != nil {
		return nil, nativeplayback.ErrUpstream
	}
	out := []source{}
	anyAllowed := false
	for _, v := range items[0].MediaSources {
		g, visible := grants[v.Id]
		if !visible {
			continue
		}
		// Known visible versions are permission-checked before UA admission.
		// Unknown/foreign IDs are deliberately not rejected until AFTER UA.
		if q.MediaSourceID == v.Id && (!g[0] || q.Download && !g[1]) {
			return nil, nativeplayback.ErrForbidden
		}
		out = append(out, source{MediaSourceInfo: v, play: g[0], download: g[1], item: items[0]})
		anyAllowed = anyAllowed || g[0] && (!q.Download || g[1])
	}
	if len(out) == 0 {
		return nil, nativeplayback.ErrNotFound
	}
	if !anyAllowed {
		return nil, nativeplayback.ErrForbidden
	}
	return out, nil
}
func selectSource(sources []source, id string, download bool) (source, error) {
	for _, s := range sources {
		if (id == "" && s.play && (!download || s.download)) || s.Id == id {
			if !s.play || download && !s.download {
				return source{}, nativeplayback.ErrForbidden
			}
			return s, nil
		}
	}
	return source{}, nativeplayback.ErrNotFound
}
func (s *Service) Info(ctx context.Context, a nativeplayback.Actor, q nativeplayback.Request) (dto.PlaybackInfoResponse, error) {
	out := dto.PlaybackInfoResponse{}
	sources, err := s.sources(ctx, a, q)
	if err != nil {
		return out, err
	}
	if err = s.admit(ctx, a); err != nil {
		return out, err
	}
	chosen, err := selectSource(sources, q.MediaSourceID, false)
	if err != nil {
		return out, err
	}
	ids := []string{}
	for _, v := range sources {
		if v.play && (q.MediaSourceID == "" || v.Id == q.MediaSourceID) {
			ids = append(ids, v.Id)
		}
	}
	groups, err := s.subtitleGroups(ctx, ids)
	if err != nil {
		return out, err
	}

	for _, v := range sources {
		if !v.play || q.MediaSourceID != "" && v.Id != q.MediaSourceID {
			continue
		}
		v.Path, v.DirectStreamUrl, err = nativeplayback.StreamURLs(a.Origin, q.ItemID, v.Id, v.Container, a.Token)
		if err != nil {
			return out, nativeplayback.ErrUpstream
		}
		if err = addSubtitles(a, q.ItemID, &v, groups[v.Id]); err != nil {
			return out, err
		}
		out.MediaSources = append(out.MediaSources, v.MediaSourceInfo)
	}
	r, err := s.session(ctx, a, q.ItemID, chosen.Id, false)
	out.PlaySessionId = r.ID
	return out, err
}

// Shared by PlaybackInfo and the final 302 (including subtitle redirects).
// Permission checks still precede admission; a matching UA never grants identity.
func (s *Service) admit(ctx context.Context, a nativeplayback.Actor) error {
	gate := playeraccess.Repository{DB: s.DB}
	req := playeraccess.Request{UserAgent: a.UserAgent, Entry: "emby", Method: a.Method, ClientIP: a.Address.ClientIP, ConnectionIP: a.Address.ConnectionIP, IPSource: a.Address.Source, IPPublic: a.Address.Public}
	decision, err := gate.Check(ctx, req)
	if err != nil {
		return nativeplayback.ErrUpstream
	}
	if !decision.Allowed {
		return nativeplayback.ErrUADenied
	}
	return nil
}
func (s *Service) Stream(ctx context.Context, a nativeplayback.Actor, q nativeplayback.Request) (nativeplayback.RedirectTarget, error) {
	sources, err := s.sources(ctx, a, q)
	if err != nil {
		return nativeplayback.RedirectTarget{}, err
	}
	if err = s.admit(ctx, a); err != nil {
		return nativeplayback.RedirectTarget{}, err
	}
	chosen, err := selectSource(sources, q.MediaSourceID, q.Download)
	if err != nil {
		return nativeplayback.RedirectTarget{}, err
	}
	if s.Links == nil {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrUpstream
	}
	if q.SubtitleIndex != nil {
		return s.subtitleTarget(ctx, a, q, chosen)
	}
	slot, err := s.session(ctx, a, q.ItemID, chosen.Id, true)
	if err != nil {
		return nativeplayback.RedirectTarget{}, err
	}
	target, err := s.Links.Resolve(ctx, chosen.Id, a.UserAgent, a.Address.ClientIP)
	if err != nil {
		if release := s.release(ctx, slot); release != nil {
			return nativeplayback.RedirectTarget{}, nativeplayback.ErrUpstream
		}
		return nativeplayback.RedirectTarget{}, err
	}
	if a.Method != "HEAD" {
		s.record(a, chosen, slot.ID, "playing", 0, true)
	}
	return target, nil
}

var _ nativeplayback.Service = (*Service)(nil)

func (s *Service) record(a nativeplayback.Actor, v source, id, state string, position int64, redirect bool) {
	if s.Stats == nil {
		return
	}
	item := v.item
	kind, name, title := "movie", item.Name, ""
	if item.Type == dto.Episode {
		kind, name, title = "episode", item.SeriesName, item.Name
		if item.IndexNumber != nil && strings.ReplaceAll(title, " ", "") == "第"+strconv.Itoa(*item.IndexNumber)+"集" {
			title = ""
		}
	}
	s.Stats.Submit(playbackstats.Snapshot{ID: id, UserID: a.User.ID, DeviceID: a.Device.ID, ItemID: item.Id, SeriesID: item.SeriesId, SeasonID: item.SeasonId, Type: kind, Name: name, EpisodeTitle: title, Season: item.ParentIndexNumber, Episode: item.IndexNumber, UserAgent: a.UserAgent, IP: a.Address.ClientIP, State: state, At: time.Now(), Position: position, Redirect: redirect})
}
