package settings

import (
	"context"
	"encoding/json"
	"errors"
	dbutil "media-tool/internal/platform/database"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNativeInput = errors.New("invalid native settings")
var ErrNativeConflict = errors.New("native settings revision conflict")
var ErrNativeUnavailable = errors.New("native settings unavailable")

type NativeManagement struct {
	ServerName string `json:"serverName"`
	Enabled    bool   `json:"enabled"`
	Revision   int64  `json:"revision"`
}

func (in NativeManagement) Valid() bool {
	return in.Revision > 0 && strings.TrimSpace(in.ServerName) == in.ServerName && utf8.ValidString(in.ServerName) && utf8.RuneCountInString(in.ServerName) >= 1 && utf8.RuneCountInString(in.ServerName) <= 100 && !strings.ContainsFunc(in.ServerName, unicode.IsControl)
}
func SaveNativeManagement(parent context.Context, db *pgxpool.Pool, in NativeManagement) (out NativeManagement, err error) {
	if !in.Valid() {
		return out, ErrNativeInput
	}
	if db == nil {
		return out, ErrNativeUnavailable
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	tx, err := db.Begin(ctx)
	if err != nil {
		return out, ErrNativeUnavailable
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer done()
		if e := tx.Rollback(cleanup); e != nil && !errors.Is(e, pgx.ErrTxClosed) {
			err = ErrNativeUnavailable
		}
	}()
	if err = tx.QueryRow(ctx, `UPDATE native_server_settings SET server_name=$1,enabled=$2,revision=revision+1,updated_at=now() WHERE singleton AND revision=$3 RETURNING revision`, in.ServerName, in.Enabled, in.Revision).Scan(&in.Revision); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, ErrNativeConflict
		}
		return out, ErrNativeUnavailable
	}
	raw, e := json.Marshal(in)
	if e != nil {
		return out, ErrNativeInput
	}
	tag, e := tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','native.server.settings',$1)`, raw)
	if e != nil || tag.RowsAffected() != 1 {
		return out, ErrNativeUnavailable
	}
	if err = dbutil.Commit(ctx, tx); err != nil {
		return out, ErrNativeUnavailable
	}
	return in, nil
}
