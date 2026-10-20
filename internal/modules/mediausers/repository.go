package mediausers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/auth"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	uuidcheck "media-tool/internal/platform/identity"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ DB *pgxpool.Pool }

func dbError(err error) error {
	if err == nil {
		return nil
	}
	var p *pgconn.PgError
	if errors.As(err, &p) && p.Code == "23505" {
		return ErrConflict
	}
	return ErrUnavailable
}
func validID(s string) bool { return uuidcheck.UUID(s) }

const userColumns = `id::text,username,display_name,enabled,must_change_password,credential_version,created_at,updated_at,max_streams,max_parental_rating,expires_at`

func userDest(u *User) []any {
	return []any{&u.ID, &u.Username, &u.DisplayName, &u.Enabled, &u.MustChangePassword, &u.Revision, &u.CreatedAt, &u.UpdatedAt, &u.MaxStreams, &u.MaxParentalRating, &u.ExpiresAt}
}
func scanUser(row pgx.Row) (u User, err error) {
	u.Grants = []Grant{}
	err = row.Scan(userDest(&u)...)
	return
}
func audit(ctx context.Context, tx pgx.Tx, actor, action, id string) error {
	raw, _ := json.Marshal(map[string]string{"userId": id})
	_, err := tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES($1,$2,$3)`, actor, action, raw)
	return err
}
func writeGrants(ctx context.Context, tx pgx.Tx, id string, grants []Grant) error {
	if _, err := tx.Exec(ctx, `DELETE FROM media_user_library_grants WHERE user_id=$1`, id); err != nil {
		return err
	}
	for _, g := range grants {
		if _, err := tx.Exec(ctx, `INSERT INTO media_user_library_grants(user_id,library_id,can_play,can_download)VALUES($1,$2,$3,$4)`, id, g.LibraryID, g.CanPlay, g.CanDownload); err != nil {
			return err
		}
	}
	return nil
}
func revoke(ctx context.Context, tx pgx.Tx, id string) error {
	if _, err := tx.Exec(ctx, `UPDATE media_user_sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE media_user_tokens SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, id)
	return err
}
func (r Repository) Save(ctx context.Context, id string, in Input) (User, error) {
	if err := in.Validate(id == ""); err != nil {
		return User{}, err
	}
	if r.DB == nil {
		return User{}, ErrUnavailable
	}
	if id != "" && !validID(id) {
		return User{}, ErrInvalid
	}
	hash := ""
	var err error
	if id == "" {
		hash, err = auth.HashPassword(in.Password)
		if err != nil {
			return User{}, err
		}
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return User{}, ErrUnavailable
	}
	defer deferutil.Rollback(ctx, tx)
	var u User
	action := "media_user.updated"
	if id == "" {
		id = jobs.UUID()
		action = "media_user.created"
		u, err = scanUser(tx.QueryRow(ctx, `INSERT INTO media_users(id,username,display_name,password_hash,enabled,must_change_password,max_streams,max_parental_rating,expires_at)VALUES($1,$2,$3,$4,$5,coalesce($6,false),coalesce($7,2),$8,$9)RETURNING `+userColumns, id, in.Username, in.DisplayName, hash, in.Enabled, in.MustChangePassword, in.MaxStreams, in.MaxParentalRating, in.ExpiresAt))
	} else {
		u, err = scanUser(tx.QueryRow(ctx, `UPDATE media_users SET username=$2,display_name=$3,enabled=$4,must_change_password=coalesce($6,must_change_password),max_streams=coalesce($7,max_streams),max_parental_rating=CASE WHEN $10 THEN $8 ELSE max_parental_rating END,expires_at=CASE WHEN $11 THEN $9 ELSE expires_at END,credential_version=credential_version+1,updated_at=now() WHERE id=$1 AND credential_version=$5 RETURNING `+userColumns, id, in.Username, in.DisplayName, in.Enabled, in.Revision, in.MustChangePassword, in.MaxStreams, in.MaxParentalRating, in.ExpiresAt, in.parentalSet || in.MaxParentalRating != nil, in.expirySet || in.ExpiresAt != nil))
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrConflict
		}
	}
	if err != nil {
		return User{}, dbError(err)
	}
	if err = writeGrants(ctx, tx, id, in.Grants); err != nil {
		return User{}, dbError(err)
	}
	if err = revoke(ctx, tx, id); err != nil {
		return User{}, ErrUnavailable
	}
	if err = audit(ctx, tx, "administrator", action, id); err != nil {
		return User{}, ErrUnavailable
	}
	if err = dbutil.Commit(ctx, tx); err != nil {
		return User{}, ErrUnavailable
	}
	u.Grants = append([]Grant{}, in.Grants...)
	return u, nil
}
func (r Repository) List(ctx context.Context) ([]User, error) {
	out := []User{}
	if r.DB == nil {
		return out, ErrUnavailable
	}
	// One snapshot avoids pairing an old user revision with new permissions.
	rows, err := r.DB.Query(ctx, `SELECT `+userColumns+`,(SELECT count(*) FROM media_user_tokens s WHERE s.user_id=u.id AND s.revoked_at IS NULL AND s.expires_at>now() AND s.credential_version=u.credential_version),coalesce((SELECT jsonb_agg(jsonb_build_object('libraryId',g.library_id,'canPlay',g.can_play,'canDownload',g.can_download) ORDER BY g.library_id) FROM media_user_library_grants g WHERE g.user_id=u.id),'[]'::jsonb) FROM media_users u ORDER BY created_at DESC LIMIT 500`)
	if err != nil {
		return out, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var u User
		var raw []byte
		if err = rows.Scan(append(userDest(&u), &u.ActiveSessions, &raw)...); err != nil {
			return out, ErrUnavailable
		}
		if err = json.Unmarshal(raw, &u.Grants); err != nil {
			return out, ErrUnavailable
		}
		out = append(out, u)
	}
	return out, dbError(rows.Err())
}

// Mutations fence the credential/permission revision and revoke every session.
func (r Repository) Mutate(ctx context.Context, id string, revision int64, action, password string, requireChange ...bool) error {
	if !validID(id) || revision < 1 {
		return ErrInvalid
	}
	if r.DB == nil {
		return ErrUnavailable
	}
	hash := ""
	var err error
	if action == "password" {
		hash, err = auth.HashPassword(password)
		if err != nil {
			return err
		}
	} else if action != "delete" && action != "revoke" {
		return ErrInvalid
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer deferutil.Rollback(ctx, tx)
	sql := `UPDATE media_users SET credential_version=credential_version+1,updated_at=now() WHERE id=$1 AND credential_version=$2`
	args := []any{id, revision}
	if action == "delete" {
		sql = `DELETE FROM media_users WHERE id=$1 AND credential_version=$2`
	} else if action == "password" {
		sql = `UPDATE media_users SET password_hash=$3,must_change_password=$4,credential_version=credential_version+1,updated_at=now() WHERE id=$1 AND credential_version=$2`
		force := len(requireChange) > 0 && requireChange[0]
		args = append(args, hash, force)
	}
	tag, err := tx.Exec(ctx, sql, args...)
	if err != nil {
		return dbError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	if action != "delete" {
		if err = revoke(ctx, tx, id); err != nil {
			return ErrUnavailable
		}
	}
	if err = audit(ctx, tx, "administrator", "media_user."+action, id); err != nil {
		return ErrUnavailable
	}
	return dbError(dbutil.Commit(ctx, tx))
}
func (r Repository) TakeAttempt(ctx context.Context, secret, scope string, limit int) (bool, error) {
	if r.DB == nil {
		return false, ErrUnavailable
	}
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte("media-tool:member-rate:v1:" + scope))
	if _, err := r.DB.Exec(ctx, `DELETE FROM media_user_auth_limits WHERE window_start<now()-interval '1 day'`); err != nil {
		return false, ErrUnavailable
	}
	var n int
	err := r.DB.QueryRow(ctx, `INSERT INTO media_user_auth_limits(scope_digest,window_start,attempts)VALUES($1,now(),1)ON CONFLICT(scope_digest)DO UPDATE SET window_start=CASE WHEN media_user_auth_limits.window_start<now()-interval '1 minute' THEN now() ELSE media_user_auth_limits.window_start END,attempts=CASE WHEN media_user_auth_limits.window_start<now()-interval '1 minute' THEN 1 ELSE media_user_auth_limits.attempts+1 END RETURNING attempts`, h.Sum(nil)).Scan(&n)
	return n <= limit, dbError(err)
}

// Get is management-only and deliberately includes hidden-library grants.
func (r Repository) Get(ctx context.Context, id string) (User, error) {
	if !validID(id) {
		return User{}, ErrInvalid
	}
	if r.DB == nil {
		return User{}, ErrUnavailable
	}
	var u User
	var raw []byte
	err := r.DB.QueryRow(ctx, `SELECT `+userColumns+`,(SELECT count(*) FROM media_user_tokens t WHERE t.user_id=u.id AND t.revoked_at IS NULL AND t.expires_at>now() AND t.credential_version=u.credential_version),coalesce((SELECT jsonb_agg(jsonb_build_object('libraryId',g.library_id,'canPlay',g.can_play,'canDownload',g.can_download) ORDER BY g.library_id) FROM media_user_library_grants g WHERE g.user_id=u.id),'[]'::jsonb) FROM media_users u WHERE id=$1`, id).Scan(append(userDest(&u), &u.ActiveSessions, &raw)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	if err != nil {
		return u, ErrUnavailable
	}
	if err = json.Unmarshal(raw, &u.Grants); err != nil {
		return u, ErrUnavailable
	}
	return u, nil
}
