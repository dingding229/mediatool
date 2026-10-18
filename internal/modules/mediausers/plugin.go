package mediausers

import (
	"context"
	"encoding/json"
	"media-tool/internal/platform/pluginactor"
)

// Page preserves one snapshot of user revision+grants, with a hard SQL bound.
// It is administration, not Login/Session or a media identity constructor.
func (r Repository) Page(ctx context.Context, start, limit int, query string) ([]User, error) {
	if _, err := pluginactor.Require(ctx, "mediaserver.users.read"); err != nil {
		return nil, err
	}
	if r.DB == nil || start < 0 || start > 200000 || limit < 1 || limit > 200 || len([]rune(query)) > 200 {
		return nil, ErrInvalid
	}
	rows, err := r.DB.Query(ctx, `SELECT `+userColumns+`,coalesce((SELECT jsonb_agg(jsonb_build_object('libraryId',g.library_id,'canPlay',g.can_play,'canDownload',g.can_download) ORDER BY g.library_id)FROM media_user_library_grants g WHERE g.user_id=u.id),'[]')FROM media_users u WHERE ($1=''OR position(lower($1)IN lower(username||' '||display_name))>0)ORDER BY created_at DESC,id LIMIT $2 OFFSET $3`, query, limit+1, start)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		var raw []byte
		if err = rows.Scan(append(userDest(&u), &raw)...); err != nil {
			return nil, ErrUnavailable
		}
		if err = json.Unmarshal(raw, &u.Grants); err != nil || len(u.Grants) > 200 {
			return nil, ErrUnavailable
		}
		out = append(out, u)
	}
	return out, dbError(rows.Err())
}
