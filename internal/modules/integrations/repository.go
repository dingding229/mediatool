package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/sites"
	"media-tool/internal/platform/secrets"
)

type Repository struct {
	DB    *pgxpool.Pool
	Vault *secrets.Vault
}

const columns = `id::text,kind,name,enabled,settings,credentials_ciphertext,revision,last_check_state,last_check_message,last_checked_at,updated_at`

func (r Repository) scan(row pgx.Row) (Credentials, error) {
	var c Credentials
	var raw json.RawMessage
	var encrypted []byte
	err := row.Scan(&c.Record.ID, &c.Record.Kind, &c.Record.Name, &c.Record.Enabled, &raw, &encrypted, &c.Record.Revision, &c.Record.LastCheckState, &c.Record.LastCheckMessage, &c.Record.LastCheckedAt, &c.Record.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, ErrUnavailable
	}
	if err = json.Unmarshal(raw, &c.Record.Settings); err != nil {
		return c, ErrUnavailable
	}
	c.Values, err = r.Vault.Open(c.Record.ID, encrypted)
	if err != nil {
		return c, err
	}
	c.Record.SecretFields = []string{}
	for key, value := range c.Values {
		if value != "" {
			c.Record.SecretFields = append(c.Record.SecretFields, key)
		}
	}
	sort.Strings(c.Record.SecretFields)
	return c, nil
}
func (r Repository) List(ctx context.Context) ([]Record, error) {
	rows, err := r.DB.Query(ctx, `SELECT `+columns+` FROM integrations ORDER BY kind,created_at`)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		c, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c.Record)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return out, nil
}
func (r Repository) Get(ctx context.Context, id string) (Credentials, error) {
	return r.scan(r.DB.QueryRow(ctx, `SELECT `+columns+` FROM integrations WHERE id=$1`, id))
}
func (r Repository) Default(ctx context.Context, kind string) (Credentials, error) {
	return r.scan(r.DB.QueryRow(ctx, `SELECT `+columns+` FROM integrations WHERE kind=$1 AND enabled ORDER BY created_at LIMIT 1`, kind))
}
func (r Repository) Save(ctx context.Context, id string, in Input) (Record, error) {
	if in.Kind == "open115" {
		if in.Settings == nil {
			in.Settings = map[string]any{}
		}
		if _, exists := in.Settings["cloudMountPrefix"]; !exists {
			in.Settings["cloudMountPrefix"] = "/115"
		}
	}
	if err := Validate(in); err != nil {
		return Record{}, err
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return Record{}, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	values := map[string]string{}
	if id != "" {
		old, err := r.scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM integrations WHERE id=$1 FOR UPDATE`, id))
		if err != nil {
			return Record{}, err
		}
		if (in.Kind == "local" || in.Kind == "clouddrive2") && (old.Record.Settings["endpoint"] != in.Settings["endpoint"] || old.Record.Settings["rootPath"] != in.Settings["rootPath"] || old.Record.Settings["cloudRoot"] != in.Settings["cloudRoot"]) {
			var fileBusy bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_operations WHERE integration_id=$1 AND state IN ('running','needs_reconcile'))`, id).Scan(&fileBusy); err != nil {
				return Record{}, ErrUnavailable
			}
			if fileBusy {
				return Record{}, errors.New("reconcile active file operations before changing their endpoint or root")
			}
		}
		if old.Record.Kind != in.Kind {
			return Record{}, errors.New("integration kind cannot change")
		}
		if in.Revision != old.Record.Revision {
			return Record{}, ErrRevision
		}
		if old.Record.Settings["endpoint"] != in.Settings["endpoint"] || (in.Kind == "open115" && old.Record.Settings["appId"] != in.Settings["appId"]) {
			for key, value := range old.Values {
				if in.Kind == "open115" && !contains(secretKeys[in.Kind], key) {
					continue
				}
				if value != "" {
					if _, provided := in.Secrets[key]; !provided {
						return Record{}, errors.New("changing endpoint requires replacing or clearing each existing credential")
					}
				}
			}
		}
		values = old.Values
		if in.Kind == "open115" && old.Record.Settings["appId"] != in.Settings["appId"] {
			values = map[string]string{}
		}
	} else {
		id = jobs.UUID()
	}
	for k, v := range in.Secrets {
		if in.Kind == "ptsite" && k == "requestHeaders" {
			headers, err := sites.ParseAccountHeaders(v)
			if err != nil {
				return Record{}, ErrInvalid
			}
			if len(headers) == 0 {
				v = ""
			} else {
				canonical, err := json.Marshal(headers)
				if err != nil {
					return Record{}, ErrInvalid
				}
				v = string(canonical)
			}
		}
		if v == "" {
			delete(values, k)
		} else {
			values[k] = v
		}
	}
	if in.Kind == "open115" {
		if _, replaced := in.Secrets["refreshToken"]; replaced {
			delete(values, "refreshBlocked")
			delete(values, "refreshAfter")
			delete(values, "accessExpiresAt")
		}
		if _, replaced := in.Secrets["accessToken"]; replaced {
			delete(values, "accessExpiresAt")
		}
	}
	// Authorization drafts are saveable without either token, even if the
	// client checked Enable. Never advertise an unauthenticated primary.
	if in.Kind == "open115" && values["accessToken"] == "" && values["refreshToken"] == "" {
		in.Enabled = false
	}
	if in.Enabled {
		switch in.Kind {
		case "open115":
			if values["accessToken"] == "" && values["refreshToken"] == "" {
				return Record{}, errors.New("official 115 requires your application authorization before enabling")
			}
		case "clouddrive2":
			if values["token"] == "" && (values["username"] == "" || values["password"] == "") {
				return Record{}, errors.New("enabled CloudDrive2 requires token or account credentials")
			}
		case "tmdb":
			if values["token"] == "" {
				return Record{}, errors.New("enabled TMDB requires a Read Access Token")
			}
		case "telegram":
			if values["botToken"] == "" || in.Settings["chatId"] == "" {
				return Record{}, errors.New("enabled Telegram requires Bot Token and Chat ID")
			}
		}
	}
	if in.Enabled && in.Kind != "ptsite" && in.Kind != "local" {
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,716))`, in.Kind); err != nil {
			return Record{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE integrations SET enabled=false,revision=revision+1,updated_at=now() WHERE kind=$1 AND id<>$2 AND enabled`, in.Kind, id); err != nil {
			return Record{}, err
		}
	}
	encrypted, err := r.Vault.Seal(id, values)
	if err != nil {
		return Record{}, err
	}
	raw, _ := json.Marshal(in.Settings)
	if in.Revision > 0 {
		_, err = tx.Exec(ctx, `UPDATE integrations SET name=$2,enabled=$3,settings=$4,credentials_ciphertext=$5,revision=revision+1,last_check_state='unchecked',last_check_message='',last_checked_at=NULL,updated_at=now() WHERE id=$1`, id, in.Name, in.Enabled, raw, encrypted)
	} else {
		_, err = tx.Exec(ctx, `INSERT INTO integrations(id,kind,name,enabled,settings,credentials_ciphertext)VALUES($1,$2,$3,$4,$5,$6)`, id, in.Kind, in.Name, in.Enabled, raw, encrypted)
	}
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return Record{}, ErrConflict
		}
		return Record{}, ErrUnavailable
	}
	audit, _ := json.Marshal(map[string]string{"integrationId": id, "kind": in.Kind})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','integration.save',$1)`, audit); err != nil {
		return Record{}, ErrUnavailable
	}
	saved, err := r.scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM integrations WHERE id=$1`, id))
	if err != nil {
		return Record{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Record{}, ErrUnavailable
	}
	return saved.Record, nil
}
func (r Repository) Delete(ctx context.Context, id string) error {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `DELETE FROM integrations WHERE id=$1`, id)
	if err != nil {
		return ErrUnavailable
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	audit, _ := json.Marshal(map[string]string{"integrationId": id})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','integration.delete',$1)`, audit); err != nil {
		return ErrUnavailable
	}
	return tx.Commit(ctx)
}
func (r Repository) RecordCheck(ctx context.Context, id string, revision int64, state, message string) error {
	_, err := r.DB.Exec(ctx, `UPDATE integrations SET last_check_state=$3,last_check_message=$4,last_checked_at=$5 WHERE id=$1 AND revision=$2`, id, revision, state, message, time.Now())
	return err
}
