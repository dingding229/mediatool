package accounts

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/auth"
)

func sessionRepository(t *testing.T) (context.Context, Repository, Account, []byte) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("media_tool_test required")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil || cfg.ConnConfig.Database != "media_tool_test" {
		t.Fatal("isolated test DB required")
	}
	ctx := context.Background()
	base, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	schema := "t1_sessions_" + strings.ReplaceAll(jobs.UUID(), "-", "")
	q := pgx.Identifier{schema}.Sanitize()
	if _, e = base.Exec(ctx, `CREATE SCHEMA `+q); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if _, e := base.Exec(ctx, `DROP SCHEMA `+q+` CASCADE`); e != nil {
			t.Error(e)
		}
		base.Close()
	})
	for _, name := range []string{"administrator_accounts", "administrator_sessions", "administrator_recovery_codes", "audit_logs"} {
		if _, e = base.Exec(ctx, `CREATE TABLE `+pgx.Identifier{schema, name}.Sanitize()+` (LIKE public.`+pgx.Identifier{name}.Sanitize()+` INCLUDING ALL)`); e != nil {
			t.Fatal(e)
		}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(db.Close)
	repo := Repository{DB: db}
	token, e := (auth.Auth{Key: strings.Repeat("s", 64)}).Issue(time.Now())
	if e != nil {
		t.Fatal(e)
	}
	account, e := repo.Register(ctx, "t1_admin", "synthetic-password", token, SessionMetadata{IP: "8.8.8.8", UserAgent: "T1/1"})
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256([]byte(token))
	return ctx, repo, account, sum[:]
}
func TestSessionRenewalStrictWindowBoundariesAndValidity(t *testing.T) {
	ctx, r, account, digest := sessionRepository(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	cases := []struct {
		name                                                string
		age, remaining                                      time.Duration
		renew, invalid, temp, mustChange, revoked, mismatch bool
	}{
		{name: "inside 1h window", age: 2 * time.Hour, remaining: auth.Lifetime - time.Hour + time.Microsecond},
		{name: "exactly 1h no renewal", age: 2 * time.Hour, remaining: auth.Lifetime - time.Hour},
		{name: "1h plus microsecond", age: 2 * time.Hour, remaining: auth.Lifetime - time.Hour - time.Microsecond, renew: true},
		{name: "active recently but renewal due", age: 2 * time.Hour, remaining: auth.Lifetime - 2*time.Hour, renew: true},
		{name: "cap inside window", age: 29 * 24 * time.Hour, remaining: 23*time.Hour + time.Microsecond},
		{name: "cap exactly 1h", age: 29 * 24 * time.Hour, remaining: 23 * time.Hour},
		{name: "cap 1h plus microsecond", age: 29 * 24 * time.Hour, remaining: 23*time.Hour - time.Microsecond, renew: true},
		{name: "already capped", age: 29 * 24 * time.Hour, remaining: 24 * time.Hour},
		{name: "seven days idle expired", age: 8 * 24 * time.Hour, remaining: -time.Microsecond, invalid: true},
		{name: "expires exactly now", age: 8 * 24 * time.Hour, invalid: true},
		{name: "absolute 30 days even future expiry", age: 30 * 24 * time.Hour, remaining: auth.Lifetime, invalid: true},
		{name: "revoked", age: 2 * time.Hour, remaining: auth.Lifetime - 2*time.Hour, invalid: true, revoked: true},
		{name: "credential mismatch", age: 2 * time.Hour, remaining: auth.Lifetime - 2*time.Hour, invalid: true, mismatch: true},
		{name: "expired temporary password", age: 2 * time.Hour, remaining: auth.Lifetime - 2*time.Hour, invalid: true, temp: true},
		{name: "valid temporary password not extended", age: 2 * time.Hour, remaining: auth.Lifetime - 2*time.Hour, temp: true, mustChange: true},
		{name: "must change without deadline not extended", age: 2 * time.Hour, remaining: auth.Lifetime - 2*time.Hour, mustChange: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var temp, revoked any
			if c.temp {
				until := now.Add(-time.Hour)
				if c.mustChange {
					until = now.Add(30 * time.Minute)
				}
				temp = until
			}
			if c.revoked {
				revoked = now.Add(-time.Minute)
			}
			version := int64(1)
			if c.mismatch {
				version = 2
			}
			if _, e := r.DB.Exec(ctx, `UPDATE administrator_accounts SET credential_version=$2,must_change_password=$3,temporary_password_expires_at=$4 WHERE id=$1`, account.ID, version, c.mustChange, temp); e != nil {
				t.Fatal(e)
			}
			created, old := now.Add(-c.age), now.Add(c.remaining)
			if _, e := r.DB.Exec(ctx, `UPDATE administrator_sessions SET created_at=$2,last_active_at=$3,expires_at=$4,revoked_at=$5,credential_version=1,ip='8.8.8.8',user_agent='T1/1' WHERE token_digest=$1`, digest, created, now, old, revoked); e != nil {
				t.Fatal(e)
			}
			// Freeze only the one SQL clock expression; all locking/predicates/UPDATE
			// clauses are exactly the production statement, not a second implementation.
			query := strings.Replace(touchSessionSQL, "statement_timestamp()", "$4::timestamptz", 1)
			var result TouchResult
			e := r.DB.QueryRow(ctx, query, digest, "8.8.8.8", "T1/1", now).Scan(&result.ExpiresAt, &result.AsOf, &result.Renewed)
			if c.renew {
				if e != nil || !result.Renewed {
					t.Fatal("expected renewal", e)
				}
			} else if !errors.Is(e, pgx.ErrNoRows) {
				t.Fatal("unexpected write", e)
			}
			var expiry, creation time.Time
			if e = r.DB.QueryRow(ctx, `SELECT created_at,expires_at FROM administrator_sessions WHERE token_digest=$1`, digest).Scan(&creation, &expiry); e != nil {
				t.Fatal(e)
			}
			expected := old
			if c.renew {
				expected = now.Add(auth.Lifetime)
				if cap := created.Add(auth.AbsoluteLifetime); expected.After(cap) {
					expected = cap
				}
			}
			if !creation.Equal(created) || !expiry.Equal(expected) {
				t.Fatal("expiry/creation changed incorrectly", c.name, creation, expiry, expected)
			}
			if c.invalid {
				if _, e = r.Session(ctx, digest); !errors.Is(e, ErrCredentials) {
					t.Fatal("invalid session authorized", e)
				}
			}
		})
	}
}
func TestSessionRenewalConcurrencyAndActivityThrottle(t *testing.T) {
	ctx, r, a, digest := sessionRepository(t)
	if _, e := r.DB.Exec(ctx, `UPDATE administrator_sessions SET created_at=now()-interval '2 hours',expires_at=now()+interval '6 days 22 hours',last_active_at=now() WHERE token_digest=$1`, digest); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	results := make(chan TouchResult, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := r.TouchSession(ctx, digest, SessionMetadata{IP: "8.8.8.8", UserAgent: "T1/1"})
			if e != nil {
				t.Error(e)
			}
			results <- out
		}()
	}
	wg.Wait()
	close(results)
	renewals := 0
	for out := range results {
		if out.Renewed {
			renewals++
		}
	}
	if renewals != 1 {
		t.Fatal("concurrent renewal count", renewals)
	}
	var before, after string
	if e := r.DB.QueryRow(ctx, `SELECT xmin::text FROM administrator_sessions WHERE token_digest=$1`, digest).Scan(&before); e != nil {
		t.Fatal(e)
	}
	for range 5 {
		out, e := r.TouchSession(ctx, digest, SessionMetadata{IP: "8.8.8.8", UserAgent: "T1/1"})
		if e != nil || out.Renewed {
			t.Fatal(out, e)
		}
	}
	if e := r.DB.QueryRow(ctx, `SELECT xmin::text FROM administrator_sessions WHERE token_digest=$1`, digest).Scan(&after); e != nil || before != after {
		t.Fatal("activity updated inside minute", e)
	}
	out, e := r.TouchSession(ctx, digest, SessionMetadata{IP: "1.1.1.1", UserAgent: "T1/2"})
	if e != nil || out.Renewed || out.ExpiresAt.IsZero() {
		t.Fatal("metadata touch must not renew", out, e)
	}
	changed, e := r.Authenticate(ctx, a.Username, "synthetic-password")
	if e != nil {
		t.Fatal(e)
	}
	if e = r.ChangePassword(ctx, changed, "synthetic-password", "changed-password"); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Session(ctx, digest); !errors.Is(e, ErrCredentials) {
		t.Fatal("password change did not invalidate", e)
	}
	if out, e = r.TouchSession(ctx, digest, SessionMetadata{}); e != nil || out.Renewed {
		t.Fatal("revoked credential renewed", out, e)
	}
}
func TestInitialSessionExpirySevenDaysAndTemporaryClamp(t *testing.T) {
	ctx, r, a, digest := sessionRepository(t)
	var created, expires time.Time
	if e := r.DB.QueryRow(ctx, `SELECT created_at,expires_at FROM administrator_sessions WHERE token_digest=$1`, digest).Scan(&created, &expires); e != nil {
		t.Fatal(e)
	}
	if expires.Sub(created) != auth.Lifetime || !a.SessionExpiresAt.Equal(expires) || expires.After(created.Add(auth.AbsoluteLifetime)) {
		t.Fatal("initial session lifetime")
	}
	until := time.Now().UTC().Add(25 * time.Minute).Truncate(time.Microsecond)
	if _, e := r.DB.Exec(ctx, `UPDATE administrator_accounts SET must_change_password=true,temporary_password_expires_at=$2 WHERE id=$1`, a.ID, until); e != nil {
		t.Fatal(e)
	}
	result, e := r.CreateSession(ctx, a, "synthetic-temporary-token", SessionMetadata{})
	if e != nil || !result.ExpiresAt.Equal(until) {
		t.Fatal("temporary password deadline not respected", result, e)
	}
}
