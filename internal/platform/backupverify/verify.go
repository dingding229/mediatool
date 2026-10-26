package backupverify

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/schema"
	"media-tool/internal/platform/secrets"
	"regexp"
	"sort"
	"strings"
)

type Table struct {
	Rows    int64    `json:"rows"`
	Digest  string   `json:"digest"`
	Columns []string `json:"columns"`
}
type Report struct {
	AuthorizedExternalJWTMigrationVerified   bool             `json:"authorizedExternalJWTMigrationVerified"`
	ExternalCredentialsInvalidated           int64            `json:"externalCredentialsInvalidated"`
	AuthorizedPermanentKeysMigrationVerified bool             `json:"authorizedPermanentKeysMigrationVerified"`
	InitialTables                            map[string]Table `json:"initialTables"`

	BaselineVersion                         int64          `json:"baselineVersion"`
	FinalVersion                            int64          `json:"finalVersion"`
	Tables                                  int            `json:"tables"`
	LegacyBusinessColumnsPreserved          bool           `json:"legacyBusinessColumnsPreserved"`
	DryRunCiphertextsUnchanged              bool           `json:"dryRunCiphertextsUnchanged"`
	SecondMigrationIdempotent               bool           `json:"secondMigrationIdempotent"`
	CipherRows                              map[string]int `json:"cipherRows"`
	AuthorizedSubscriptionMigrationVerified bool           `json:"authorizedSubscriptionMigrationVerified"`
	NoWorkersStarted                        bool           `json:"noWorkersStarted"`
}

type Querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func Inventory(ctx context.Context, db Querier, baseline map[string]Table) (map[string]Table, error) {
	rows, e := db.Query(ctx, `SELECT table_name,column_name FROM information_schema.columns WHERE table_schema='public' ORDER BY table_name,ordinal_position`)
	if e != nil {
		return nil, e
	}
	columns := map[string][]string{}
	for rows.Next() {
		var t, c string
		if rows.Scan(&t, &c) != nil {
			rows.Close()
			return nil, errors.New("inventory")
		}
		columns[t] = append(columns[t], c)
	}
	if e = rows.Err(); e != nil {
		rows.Close()
		return nil, e
	}
	rows.Close()
	out := map[string]Table{}
	names := []string{}
	for name := range columns {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		selected := columns[name]
		if baseline != nil {
			old, ok := baseline[name]
			if !ok {
				continue
			}
			selected = old.Columns
		}
		parts := []string{}
		for _, c := range selected {
			parts = append(parts, pgx.Identifier{c}.Sanitize())
		}
		query := "SELECT count(*),coalesce(md5(string_agg(to_jsonb(t)::text,E'\\n' ORDER BY to_jsonb(t)::text)),md5('')) FROM (SELECT " + join(parts) + " FROM " + pgx.Identifier{"public", name}.Sanitize() + ") t"
		value := Table{Columns: selected}
		if e = db.QueryRow(ctx, query).Scan(&value.Rows, &value.Digest); e != nil {
			return nil, errors.New("table checksum unavailable")
		}
		out[name] = value
	}
	return out, nil
}
func join(parts []string) string { return strings.Join(parts, ",") }
func equal(a, b map[string]Table, skipGoose bool) bool {
	for name, x := range a {
		if skipGoose && name == "goose_db_version" {
			continue
		}
		y, ok := b[name]
		if !ok || x.Rows != y.Rows || x.Digest != y.Digest {
			return false
		}
	}
	return true
}
func Verify(ctx context.Context, db *pgxpool.Pool, dsn, expected, key string, testOnly bool) (Report, error) {
	r := Report{NoWorkersStarted: true}
	allowed := regexp.MustCompile(`^media_tool_restore_[a-f0-9]{12}$`).MatchString(expected) || testOnly && expected == "media_tool_test"
	var actual string
	if !allowed || db.QueryRow(ctx, `SELECT current_database()`).Scan(&actual) != nil || actual != expected {
		return r, errors.New("restore-only database guard")
	}
	if e := db.QueryRow(ctx, `SELECT max(version_id)FROM goose_db_version WHERE is_applied`).Scan(&r.BaselineVersion); e != nil {
		return r, e
	}
	before, e := Inventory(ctx, db, nil)
	r.InitialTables = before
	if e != nil {
		return r, e
	}
	// Preserve the full inventory for the backup receipt. Compare only the exact
	// approved empty removals; the migration separately locks and asserts columns.
	projection := before
	if r.BaselineVersion < 68 {
		projection, e = retirementProjection(before)
		if e != nil {
			return r, e
		}
		projection, e = Inventory(ctx, db, projection)
		if e != nil {
			return r, e
		}
	}
	var protected37 map[string]Table
	if r.BaselineVersion < 37 {
		protected37, e = subscriptionMigrationProjection(ctx, db, before)
		if e != nil {
			return r, e
		}
	}
	if r.BaselineVersion < 89 {
		projection = externalJWTProjection(projection)
	}
	if e = schema.Up(ctx, dsn); e != nil {
		return r, e
	}
	after, e := Inventory(ctx, db, projection)
	if e != nil {
		return r, e
	}
	if r.BaselineVersion < 89 {
		r.ExternalCredentialsInvalidated, e = verifyExternalJWTMigration(ctx, db, before)
		if e != nil {
			return r, e
		}
		r.AuthorizedExternalJWTMigrationVerified = true
	}
	compareBefore, compareAfter := map[string]Table{}, map[string]Table{}
	for n, v := range projection {
		compareBefore[n] = v
	}
	for n, v := range after {
		compareAfter[n] = v
	}
	if protected37 != nil {
		actual37, err := subscriptionMigrationProjection(ctx, db, before)
		if err != nil {
			return r, err
		}
		if !equal(protected37, actual37, false) {
			return r, errors.New("unapproved subscription/settings data changed")
		}
		if err = validateSubscriptionMigration(ctx, db); err != nil {
			return r, err
		}
		for name := range protected37 {
			delete(compareBefore, name)
			delete(compareAfter, name)
		}
		r.AuthorizedSubscriptionMigrationVerified = true
	}

	r.LegacyBusinessColumnsPreserved = equal(compareBefore, compareAfter, true)
	if !r.LegacyBusinessColumnsPreserved {
		return r, errors.New("legacy data changed during migration")
	}
	all, e := Inventory(ctx, db, nil)
	if e != nil {
		return r, e
	}
	old, e := secrets.New(key)
	if e != nil {
		return r, e
	}
	nextKey := make([]byte, 32)
	if _, e = rand.Read(nextKey); e != nil {
		return r, e
	}
	next, _ := secrets.New(hex.EncodeToString(nextKey))
	r.CipherRows, e = secrets.Rekey(ctx, db, old, next, true)
	if e != nil {
		return r, errors.New("restored ciphertext readability failed")
	}
	checked, e := Inventory(ctx, db, nil)
	if e != nil {
		return r, e
	}
	r.DryRunCiphertextsUnchanged = equal(all, checked, false)
	if e = schema.Up(ctx, dsn); e != nil {
		return r, e
	}
	twice, e := Inventory(ctx, db, nil)
	if e != nil {
		return r, e
	}
	r.SecondMigrationIdempotent = equal(checked, twice, false)
	r.Tables = len(all)
	if checkedErr := db.QueryRow(ctx, `SELECT max(version_id)FROM goose_db_version WHERE is_applied`).Scan(&r.FinalVersion); checkedErr != nil {
		return Report{}, checkedErr
	}
	if !r.DryRunCiphertextsUnchanged || !r.SecondMigrationIdempotent {
		return r, fmt.Errorf("clone immutability failed")
	}
	return r, nil
}

type Export struct {
	Snapshot string           `json:"snapshot"`
	Tables   map[string]Table `json:"tables"`
	Version  int64            `json:"version"`
}

func ExportSnapshot(ctx context.Context, db *pgxpool.Pool) (Export, pgx.Tx, error) {
	tx, e := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return Export{}, nil, e
	}
	out := Export{}
	out.Tables, e = Inventory(ctx, tx, nil)
	if e != nil {
		deferutil.Rollback(ctx, tx)
		return out, nil, e
	}
	if e = tx.QueryRow(ctx, `SELECT pg_export_snapshot()`).Scan(&out.Snapshot); e != nil {
		deferutil.Rollback(ctx, tx)
		return out, nil, e
	}
	if checkedErr := tx.QueryRow(ctx, `SELECT max(version_id)FROM goose_db_version WHERE is_applied`).Scan(&out.Version); checkedErr != nil {
		deferutil.Rollback(ctx, tx)
		return Export{}, nil, checkedErr
	}
	return out, tx, nil
}
