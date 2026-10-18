package v2

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"testing"
	"time"
)

func authFixture(t *testing.T) (Store, Lease, Broker, string) {
	t.Helper()
	s, l := activeFixture(t)
	ctx := context.Background()
	if _, e := s.DB.Exec(ctx, `CREATE TABLE administrator_accounts (LIKE public.administrator_accounts INCLUDING ALL)`); e != nil {
		t.Fatal(e)
	}
	account := jobs.UUID()
	if _, e := s.DB.Exec(ctx, `INSERT INTO administrator_accounts(id,username,password_hash)VALUES($1,'bound','test-sentinel-not-a-login-hash')`, account); e != nil {
		t.Fatal(e)
	}
	d := contracts.Descriptor{Kind: "chain", ID: "auth.verification", ProviderID: "fake", Issuer: "hello-test", Audience: "mediatool-test", Origin: "https://hello.invalid", Handler: contracts.HandlerID("chain", "auth.verification"), RequiredScopes: []string{"auth.provider", "events.chain.auth.verification"}}
	if e := s.Publish(ctx, l, contracts.Snapshot{Version: 2, Descriptors: []contracts.Descriptor{d}}); e != nil {
		t.Fatal(e)
	}
	if e := s.Renew(ctx, l, true); e != nil {
		t.Fatal(e)
	}
	return s, l, Broker{s}, account
}
func authReply(s Store, a contracts.AuthAssertion) <-chan error {
	done := make(chan error, 1)
	go func() {
		ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		for ctx.Err() == nil {
			var id string
			e := s.DB.QueryRow(ctx, `SELECT id::text FROM plugin_v2_runs WHERE state='pending' AND handler=$1 ORDER BY created_at LIMIT 1`, contracts.HandlerID("chain", "auth.verification")).Scan(&id)
			if errors.Is(e, pgx.ErrNoRows) {
				time.Sleep(time.Millisecond)
				continue
			}
			if e != nil {
				done <- e
				return
			}
			raw, _ := json.Marshal(a)
			cipher, e := s.seal("call-result", id, raw)
			if e == nil {
				_, e = s.DB.Exec(ctx, `UPDATE plugin_v2_runs SET state='succeeded',result_ciphertext=$2 WHERE id=$1`, id, cipher)
			}
			done <- e
			return
		}
		done <- ctx.Err()
	}()
	return done
}
func TestBrokerBoundIdentityNonceAndImmediateRevocation(t *testing.T) {
	s, l, b, account := authFixture(t)
	ctx := context.Background()
	if _, e := b.Begin(ctx, l.ID, "fake", "http://wrong.invalid"); e == nil {
		t.Fatal("callback origin")
	}
	if e := b.Bind(ctx, "admin", l.ID, "fake", "bound", account, l.Fingerprint); e != nil {
		t.Fatal(e)
	}
	challenge, e := b.Begin(ctx, l.ID, "fake", "https://hello.invalid")
	if e != nil {
		t.Fatal(e)
	}
	done := authReply(s, contracts.AuthAssertion{Subject: "bound", Issuer: "hello-test", Audience: "mediatool-test", ExpiresAt: time.Now().Add(time.Minute).Unix(), IsAdmin: true})
	identity, e := b.Verify(ctx, l.ID, "fake", "https://hello.invalid", challenge.State, challenge.Nonce, "bound")
	if e != nil {
		t.Fatal(e)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	if identity.AccountID != account {
		t.Fatal("provider role injection")
	}
	if e = b.Validate(ctx, &identity); e != nil {
		t.Fatal(e)
	}
	if _, e = b.Verify(ctx, l.ID, "fake", "https://hello.invalid", challenge.State, challenge.Nonce, "bound"); e == nil {
		t.Fatal("nonce replay")
	}
	if _, e = b.PostVerifyPolicy(ctx, false, account); e == nil {
		t.Fatal("401 flipped")
	}
	if _, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope='auth.provider'`, l.ID); e != nil {
		t.Fatal(e)
	}
	if e = b.Validate(ctx, &identity); e == nil {
		t.Fatal("revoked identity survived")
	}
	if _, e = b.Begin(ctx, l.ID, "fake", "https://hello.invalid"); e == nil {
		t.Fatal("revoked provider")
	}
	var sessions int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM administrator_accounts WHERE id=$1 AND password_hash='test-sentinel-not-a-login-hash'`, account).Scan(&sessions); e != nil || sessions != 1 {
		t.Fatal("password altered", sessions, e)
	}
}
func TestBrokerUnknownSubjectIssuerAudienceExpiryFailClosed(t *testing.T) {
	s, l, b, account := authFixture(t)
	ctx := context.Background()
	if e := b.Bind(ctx, "admin", l.ID, "fake", "bound", account, l.Fingerprint); e != nil {
		t.Fatal(e)
	}
	good := contracts.AuthAssertion{Subject: "bound", Issuer: "hello-test", Audience: "mediatool-test", ExpiresAt: time.Now().Add(time.Minute).Unix()}
	for _, mutate := range []func(*contracts.AuthAssertion){func(a *contracts.AuthAssertion) { a.Subject = "unbound"; a.IsAdmin = true }, func(a *contracts.AuthAssertion) { a.Issuer = "evil" }, func(a *contracts.AuthAssertion) { a.Audience = "evil" }, func(a *contracts.AuthAssertion) { a.ExpiresAt = 1 }} {
		q, e := b.Begin(ctx, l.ID, "fake", "https://hello.invalid")
		if e != nil {
			t.Fatal(e)
		}
		a := good
		mutate(&a)
		done := authReply(s, a)
		if _, e = b.Verify(ctx, l.ID, "fake", "https://hello.invalid", q.State, q.Nonce, "bound"); e == nil {
			t.Fatal("invalid assertion")
		}
		if e = <-done; e != nil {
			t.Fatal(e)
		}
	}
	q, e := b.Begin(ctx, l.ID, "fake", "https://hello.invalid")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE app_settings SET value=jsonb_set(value,'{Expires}',to_jsonb('2000-01-01T00:00:00Z'::text)) WHERE key=$1`, "plugins.auth.nonce."+digest([]byte(q.State))); e != nil {
		t.Fatal(e)
	}
	if _, e = b.Verify(ctx, l.ID, "fake", "https://hello.invalid", q.State, q.Nonce, "bound"); e == nil {
		t.Fatal("expired nonce")
	}
}

func TestAuthPostVerifyOnlyRestrictsAndFailureDoesNotReplacePassword(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	d := contracts.Descriptor{Kind: "chain", ID: "auth.intercept", Handler: contracts.HandlerID("chain", "auth.intercept"), RequiredScopes: []string{"events.chain.auth.intercept"}}
	if e := s.Publish(ctx, l, contracts.Snapshot{Version: 2, Descriptors: []contracts.Descriptor{d}}); e != nil {
		t.Fatal(e)
	}
	if e := s.Renew(ctx, l, true); e != nil {
		t.Fatal(e)
	}
	b := Broker{s}
	account := jobs.UUID()
	if _, e := b.PostVerifyPolicy(ctx, false, account); e == nil {
		t.Fatal("builtin 401 converted")
	}
	var n int
	if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs`).Scan(&n); e != nil || n != 0 {
		t.Fatal("401 dispatched plugin", n, e)
	}
	for _, tc := range []struct {
		raw  string
		deny bool
	}{{`{"allow":false}`, true}, {`{"allow":true,"isAdmin":true}`, false}} {
		done := policyReply(s, json.RawMessage(tc.raw), false)
		_, e := b.PostVerifyPolicy(ctx, true, account)
		if tc.deny != (e != nil) {
			t.Fatal(tc, e)
		}
		if e = <-done; e != nil {
			t.Fatal(e)
		}
	}
	// An unavailable policy adds no privilege: it returns the already verified builtin identity only.
	start := time.Now()
	if _, e := b.PostVerifyPolicy(ctx, true, account); e != nil {
		t.Fatal(e)
	}
	if time.Since(start) > time.Second {
		t.Fatal("unbounded auth policy")
	}
}
