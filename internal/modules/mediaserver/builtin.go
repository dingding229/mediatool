package mediaserver

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"regexp"
)

type Builtin struct{ DB *pgxpool.Pool }

func (Builtin) Describe() Description {
	return Description{ID: "builtin", Name: "内置媒体库", Contract: ContractVersion, Runtime: "go-embedded", Stage: "catalog-preview", Capabilities: Capabilities{Catalog: true, MemberAuth: true, LibraryPermissions: true}, Note: "已确认资产的只读目录与独立媒体用户可用。播放票据、播放进度、设备管理和Emby协议替代尚未实现。"}
}
func (b Builtin) Libraries(ctx context.Context, a Access) ([]Library, error) {
	out := []Library{}
	if b.DB == nil {
		return out, ErrUnavailable
	}
	var rows pgx.Rows
	var err error
	if a.UserID == "" {
		rows, err = b.DB.Query(ctx, `SELECT id,name,media_type,false FROM native_media_libraries ORDER BY id`)
	} else {
		rows, err = b.DB.Query(ctx, `SELECT l.id,l.name,l.media_type,g.can_play FROM native_media_libraries l JOIN media_user_library_grants g ON g.library_id=l.id JOIN media_users u ON u.id=g.user_id WHERE u.id=$1 AND u.enabled AND NOT u.must_change_password AND u.credential_version=$2 ORDER BY l.id`, a.UserID, a.Revision)
	}
	if err != nil {
		return out, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var l Library
		if err = rows.Scan(&l.ID, &l.Name, &l.MediaType, &l.CanPlay); err != nil {
			return out, ErrUnavailable
		}
		out = append(out, l)
	}
	if rows.Err() != nil {
		return out, ErrUnavailable
	}
	return out, nil
}
func (b Builtin) authorize(ctx context.Context, a Access, library string, play bool) error {
	if b.DB == nil {
		return ErrUnavailable
	}
	if a.UserID == "" || a.Revision < 1 {
		return ErrDenied
	}
	var allowed bool
	err := b.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_user_library_grants g JOIN media_users u ON u.id=g.user_id WHERE g.user_id=$1 AND g.library_id=$2 AND u.enabled AND NOT u.must_change_password AND u.credential_version=$3 AND (NOT $4 OR g.can_play))`, a.UserID, library, a.Revision, play).Scan(&allowed)
	if err != nil {
		return ErrUnavailable
	}
	if !allowed {
		return ErrDenied
	}
	return nil
}

// Only confirmed, ready, present media are published. This is deliberately a
// projection without paths, pickCodes, integration IDs, or persistent grants.
const memberAssets = ` FROM media_assets a JOIN cloud_files f ON f.id=a.cloud_file_id JOIN media_asset_bindings b ON b.asset_id=a.id JOIN media_items m ON m.id=b.media_id JOIN native_media_libraries l ON l.media_type=m.media_type JOIN media_user_library_grants g ON g.library_id=l.id JOIN media_users u ON u.id=g.user_id WHERE u.id=$1 AND u.enabled AND NOT u.must_change_password AND u.credential_version=$2 AND l.id=$3 AND f.state='present' AND b.state='ready'`

var safePoster = regexp.MustCompile(`^/[A-Za-z0-9_-]+\.(?:jpg|png|webp)$`)
var safeUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (b Builtin) List(ctx context.Context, a Access, q Query) (Page, error) {
	out := Page{Items: []Item{}, Page: q.Page, PageSize: q.PageSize}
	if len(q.Text) > 200 || q.Page < 1 || q.Page > 10000 || q.PageSize < 1 || q.PageSize > 100 {
		return out, ErrInvalid
	}
	if err := b.authorize(ctx, a, q.LibraryID, false); err != nil {
		return out, err
	}
	where := memberAssets + ` AND ($4='' OR m.title ILIKE '%'||$4||'%')`
	rows, err := b.DB.Query(ctx, `SELECT a.id::text,m.title,m.release_year,m.media_type,coalesce(m.metadata->>'overview',''),coalesce(m.metadata->>'posterPath',''),b.season,b.episode`+where+` ORDER BY m.title,a.id LIMIT $5 OFFSET $6`, a.UserID, a.Revision, q.LibraryID, q.Text, q.PageSize, (q.Page-1)*q.PageSize)
	if err != nil {
		return out, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var i Item
		var poster string
		if err = rows.Scan(&i.ID, &i.Title, &i.Year, &i.MediaType, &i.Overview, &poster, &i.Season, &i.Episode); err != nil {
			return out, ErrUnavailable
		}
		if safePoster.MatchString(poster) {
			i.Poster = fmt.Sprintf("/api/v1/member/libraries/%s/items/%s/poster", q.LibraryID, i.ID)
		}
		out.Items = append(out.Items, i)
	}
	if rows.Err() != nil {
		return out, ErrUnavailable
	}
	rows.Close()
	if err = b.DB.QueryRow(ctx, `SELECT count(*)`+where, a.UserID, a.Revision, q.LibraryID, q.Text).Scan(&out.Total); err != nil {
		return out, ErrUnavailable
	}
	return out, nil
}
func (b Builtin) Poster(ctx context.Context, a Access, library, id string) (string, error) {
	if !safeUUID.MatchString(id) {
		return "", ErrNotFound
	}
	if err := b.authorize(ctx, a, library, false); err != nil {
		return "", err
	}
	var path string
	err := b.DB.QueryRow(ctx, `SELECT coalesce(m.metadata->>'posterPath','')`+memberAssets+` AND a.id=$4`, a.UserID, a.Revision, library, id).Scan(&path)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !safePoster.MatchString(path) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", ErrUnavailable
	}
	return path, nil
}
func (b Builtin) Playback(ctx context.Context, a Access, r PlaybackRequest) (PlaybackTicket, error) {
	if !safeUUID.MatchString(r.ItemID) {
		return PlaybackTicket{}, ErrNotFound
	}
	if err := b.authorize(ctx, a, r.LibraryID, true); err != nil {
		return PlaybackTicket{}, err
	}
	var exists bool
	err := b.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1`+memberAssets+` AND a.id=$4)`, a.UserID, a.Revision, r.LibraryID, r.ItemID).Scan(&exists)
	if err != nil {
		return PlaybackTicket{}, ErrUnavailable
	}
	if !exists {
		return PlaybackTicket{}, ErrNotFound
	}
	return PlaybackTicket{}, ErrUnsupported
}
