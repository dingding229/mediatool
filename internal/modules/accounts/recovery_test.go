package accounts

import (
	"context"
	"crypto/sha256"
	"errors"
	"media-tool/internal/platform/database"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRecoveryCodeFormatAndDigest(t *testing.T) {
	code, err := newRecoveryCode()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := recoveryDigest(code)
	upper, otherErr := recoveryDigest(strings.ToUpper(code))
	if err != nil || otherErr != nil || len(digest) != sha256.Size || string(digest) != string(upper) {
		t.Fatal("invalid recovery format")
	}
	for _, s := range []string{"", "username", strings.Repeat("x", 101), "mt-" + strings.Repeat("g", 64)} {
		if _, err := recoveryDigest(s); err == nil {
			t.Fatal("invalid code accepted")
		}
	}
}
func TestPostgresRecoveryExpiryRotationRevocationAndSingleUse(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated DB required")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err = db.QueryRow(ctx, "SELECT current_database()").Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("isolated test database required")
	}
	if _, err = db.Exec(ctx, "TRUNCATE administrator_accounts,administrator_auth_limits CASCADE"); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, "TRUNCATE administrator_accounts,administrator_auth_limits CASCADE")
	r := Repository{DB: db}
	a, err := r.Register(ctx, "recovery_admin", "Recovery-old-2026", "owned-session")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = r.IssueRecoveryCode(ctx, a, "wrong-password"); !errors.Is(err, ErrCredentials) {
		t.Fatal("wrong password allowed")
	}
	old, expiry, err := r.IssueRecoveryCode(ctx, a, "Recovery-old-2026")
	if err != nil || expiry.Sub(time.Now()) < 29*24*time.Hour || expiry.Sub(time.Now()) > RecoveryLifetime+time.Minute {
		t.Fatal("expiry", err)
	}
	fresh, _, err := r.IssueRecoveryCode(ctx, a, "Recovery-old-2026")
	if err != nil {
		t.Fatal(err)
	}
	if err = r.RecoverPassword(ctx, a.Username, old, "Recovery-new-2026"); !errors.Is(err, ErrCredentials) {
		t.Fatal("rotated code accepted")
	}
	for _, who := range []string{a.Username, "nonexistent"} {
		if err = r.RecoverPassword(ctx, who, "mt-"+strings.Repeat("a", 64), "Recovery-new-2026"); !errors.Is(err, ErrCredentials) {
			t.Fatal("invalid code accepted")
		}
	}
	var stored []byte
	if err = db.QueryRow(ctx, "SELECT code_digest FROM administrator_recovery_codes").Scan(&stored); err != nil || strings.Contains(string(stored), fresh) || len(stored) != 32 {
		t.Fatal("plaintext stored")
	}
	if err = r.RecoverPassword(ctx, a.Username, fresh, strings.Repeat("x", 21)); err == nil {
		t.Fatal("length bypass")
	}
	if _, err = db.Exec(ctx, "UPDATE administrator_recovery_codes SET expires_at=now()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	if err = r.RecoverPassword(ctx, a.Username, fresh, "Recovery-new-2026"); !errors.Is(err, ErrCredentials) {
		t.Fatal("expired code accepted")
	}
	fresh, _, err = r.IssueRecoveryCode(ctx, a, "Recovery-old-2026")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() { results <- r.RecoverPassword(ctx, a.Username, fresh, "Recovery-new-2026") })
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if !errors.Is(err, ErrCredentials) {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatal("recovery concurrency", wins)
	}
	digest := sha256.Sum256([]byte("owned-session"))
	if _, err = r.Session(ctx, digest[:]); err == nil {
		t.Fatal("session survived")
	}
	if _, err = r.Authenticate(ctx, a.Username, "Recovery-old-2026"); !errors.Is(err, ErrCredentials) {
		t.Fatal("old password survived")
	}
	a, err = r.Authenticate(ctx, a.Username, "Recovery-new-2026")
	if err != nil {
		t.Fatal(err)
	}
	if err = r.RecoverPassword(ctx, a.Username, fresh, "Recovery-next-2026"); !errors.Is(err, ErrCredentials) {
		t.Fatal("code replay accepted")
	}
	_, _, err = r.IssueRecoveryCode(ctx, a, "Recovery-new-2026")
	if err != nil {
		t.Fatal(err)
	}
	if err = r.ChangePassword(ctx, a, "Recovery-new-2026", "Changed-next-2026"); err != nil {
		t.Fatal(err)
	}
	var count int
	db.QueryRow(ctx, "SELECT count(*) FROM administrator_recovery_codes").Scan(&count)
	if count != 0 {
		t.Fatal("password change retained code")
	}
	var audit string
	db.QueryRow(ctx, "SELECT string_agg(details::text,' ') FROM audit_logs WHERE action LIKE 'account.%'").Scan(&audit)
	if strings.Contains(audit, fresh) || strings.Contains(audit, "Recovery-new-2026") {
		t.Fatal("audit secrets")
	}
}
