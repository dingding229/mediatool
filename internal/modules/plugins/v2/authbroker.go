package v2

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/dingding229/mediatool-plugin/sdk/events"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/pluginstate"
	"net/url"
	"time"
)

// Broker is deliberately not a session issuer. The UI/callback login adapter is
// unsupported until separately reviewed. Password/Argon2/CSRF/media auth stay
// entirely in the built-in authentication path, with no plugin hook attached.
type Broker struct{ Store Store }
type AuthChallenge struct {
	State string `json:"state"`
	Nonce string `json:"nonce"`
}
type authNonce struct {
	Instance, Provider, Origin, Nonce, Generation string
	Revision                                      int64
	Expires                                       time.Time
	Used                                          bool
}
type authBinding struct {
	Account, Provider, Fingerprint string
	InstanceID                     string `json:"instanceId,omitempty"`
}
type BrokerIdentity struct {
	AccountID  string               `json:"accountId"`
	Version    int64                `json:"-"`
	Snapshot   Snapshot             `json:"-"`
	Descriptor contracts.Descriptor `json:"-"`
	Expires    time.Time            `json:"-"`
	bindingKey string
}

func authDenied() error { return contracts.NewError(contracts.PermissionDenied) }
func authRandom() (string, error) {
	b := make([]byte, 32)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func (b Broker) provider(ctx context.Context, instance, provider string) (Snapshot, contracts.Descriptor, error) {
	snap, d, e := b.Store.Descriptor(ctx, instance, "chain", "auth.verification")
	if e != nil {
		return snap, d, e
	}
	if d.ProviderID != provider || provider == "" || len(provider) > 64 {
		return snap, d, authDenied()
	}
	return snap, d, nil
}
func (b Broker) Bind(ctx context.Context, actor, instance, provider, subject, account, fingerprint string) error {
	snap, d, e := b.provider(ctx, instance, provider)
	if e != nil {
		return e
	}
	if subject == "" || len(subject) > 200 || !validUUID(account) {
		return ErrInvalid
	}
	tx, e := b.Store.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer rollback(ctx, tx)
	var fp string
	if e = tx.QueryRow(ctx, `SELECT approved_fingerprint FROM plugin_v2_instances WHERE id=$1`, instance).Scan(&fp); e != nil {
		return e
	}
	if fingerprint != fp {
		return ErrConflict
	}
	var version int64
	if e = tx.QueryRow(ctx, `SELECT credential_version FROM administrator_accounts WHERE id=$1 FOR SHARE`, account).Scan(&version); e != nil {
		return authDenied()
	}
	key := "plugins.auth.binding." + workflowHash([]string{instance, provider, subject})
	v := authBinding{}
	if e = pluginstate.Lock(ctx, tx, key, &v); e != nil {
		return e
	}
	v = authBinding{Account: account, Provider: provider, Fingerprint: fp, InstanceID: instance}
	if e = pluginstate.Put(ctx, tx, key, v); e != nil {
		return e
	}
	if e = b.Store.fencePolicy(ctx, tx, snap, d); e != nil {
		return e
	}
	if e = workflowAudit(ctx, tx, actor, "plugins.auth.bind", instance); e != nil {
		return e
	}
	return dbutil.Commit(ctx, tx)
}
func (b Broker) Begin(ctx context.Context, instance, provider, origin string) (AuthChallenge, error) {
	snap, d, e := b.provider(ctx, instance, provider)
	if e != nil {
		return AuthChallenge{}, e
	}
	u, e := url.Parse(origin)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || origin != d.Origin {
		return AuthChallenge{}, authDenied()
	}
	state, e := authRandom()
	if e != nil {
		return AuthChallenge{}, e
	}
	nonce, e := authRandom()
	if e != nil {
		return AuthChallenge{}, e
	}
	tx, e := b.Store.DB.Begin(ctx)
	if e != nil {
		return AuthChallenge{}, e
	}
	defer rollback(ctx, tx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7062010)`); e != nil {
		return AuthChallenge{}, e
	}
	// A private, bounded ledger; no raw challenge/subject or session token retained.
	_, e = tx.Exec(ctx, `DELETE FROM app_settings WHERE key LIKE 'plugins.auth.nonce.%' AND (value->>'Expires')::timestamptz<clock_timestamp()`)
	if e != nil {
		return AuthChallenge{}, e
	}
	var count int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM app_settings WHERE key LIKE 'plugins.auth.nonce.%'`).Scan(&count); e != nil {
		return AuthChallenge{}, e
	}
	if count >= 200 {
		return AuthChallenge{}, contracts.NewError(contracts.Busy)
	}
	n := authNonce{Instance: instance, Provider: provider, Origin: origin, Nonce: digest([]byte(nonce)), Generation: snap.Generation, Revision: snap.GrantRevision, Expires: time.Now().Add(5 * time.Minute)}
	if e = pluginstate.Lock(ctx, tx, "plugins.auth.nonce."+digest([]byte(state)), &authNonce{}); e != nil {
		return AuthChallenge{}, e
	}
	if e = pluginstate.Put(ctx, tx, "plugins.auth.nonce."+digest([]byte(state)), n); e != nil {
		return AuthChallenge{}, e
	}
	if e = b.Store.fencePolicy(ctx, tx, snap, d); e != nil {
		return AuthChallenge{}, e
	}
	if e = dbutil.Commit(ctx, tx); e != nil {
		return AuthChallenge{}, e
	}
	return AuthChallenge{state, nonce}, nil
}
func (b Broker) Verify(ctx context.Context, instance, provider, origin, state, nonce, challenge string) (BrokerIdentity, error) {
	denied := BrokerIdentity{}
	if len(state) != 64 || len(nonce) != 64 || len(challenge) > 200 {
		return denied, authDenied()
	}
	snap, d, e := b.provider(ctx, instance, provider)
	if e != nil {
		return denied, e
	}
	tx, e := b.Store.DB.Begin(ctx)
	if e != nil {
		return denied, e
	}
	defer rollback(ctx, tx)
	var n authNonce
	key := "plugins.auth.nonce." + digest([]byte(state))
	// Unknown states must not create durable entries.
	var raw []byte
	if e = tx.QueryRow(ctx, `SELECT value FROM app_settings WHERE key=$1 FOR UPDATE`, key).Scan(&raw); e != nil {
		return denied, authDenied()
	}
	if json.Unmarshal(raw, &n) != nil {
		return denied, authDenied()
	}
	if n.Used || n.Instance != instance || n.Provider != provider || n.Origin != origin || origin != d.Origin || n.Generation != snap.Generation || n.Revision != snap.GrantRevision || time.Now().After(n.Expires) || n.Nonce != digest([]byte(nonce)) {
		return denied, authDenied()
	}
	n.Used = true
	if e = pluginstate.Put(ctx, tx, key, n); e != nil {
		return denied, e
	}
	if e = b.Store.fencePolicy(ctx, tx, snap, d); e != nil {
		return denied, e
	}
	if e = dbutil.Commit(ctx, tx); e != nil {
		return denied, e
	}
	// No locks held across the 200ms provider RPC; errors fail closed, never success.
	request, _ := json.Marshal(contracts.AuthRequest{ProviderID: provider, Challenge: challenge})
	args, _ := json.Marshal(events.ChainEnvelope{Type: contracts.ChainAuthVerification, SchemaVersion: 1, Payload: request})
	bounded, stop := context.WithTimeout(ctx, 200*time.Millisecond)
	result, e := b.Store.invokeSnapshot(bounded, snap, d, args, "")
	stop()
	if e != nil {
		return denied, authDenied()
	}
	var a contracts.AuthAssertion
	if contracts.Decode(result, &a) != nil || a.Subject == "" || len(a.Subject) > 200 || a.Issuer != d.Issuer || a.Audience != d.Audience || a.ExpiresAt <= time.Now().Unix() || a.ExpiresAt > time.Now().Add(5*time.Minute).Unix() {
		return denied, authDenied()
	}
	bindingKey := "plugins.auth.binding." + workflowHash([]string{instance, provider, a.Subject})
	var binding authBinding
	if e = (WorkflowRunner{b.Store}).load(ctx, bindingKey, &binding); e != nil {
		return denied, authDenied()
	}
	identity := BrokerIdentity{AccountID: binding.AccountID(), Snapshot: snap, Descriptor: d, Expires: time.Unix(a.ExpiresAt, 0), bindingKey: bindingKey}
	if e = b.Validate(ctx, &identity); e != nil {
		return denied, e
	}
	// Provider IsAdmin is intentionally ignored; local account is the only authority.
	prepared, e := b.PostVerifyPolicy(ctx, true, identity.AccountID)
	if e != nil {
		return denied, e
	}
	if e = dbutil.CheckPrepared(prepared, b.Store.DB); e != nil {
		return denied, e
	}
	return identity, nil
}
func (a authBinding) AccountID() string { return a.Account }

// Validate is mandatory on EVERY use of a broker identity: disable, revocation,
// changed bindings or a new generation invalidate it immediately. No auth cache.
func (b Broker) Validate(ctx context.Context, i *BrokerIdentity) error {
	if i == nil || i.AccountID == "" || time.Now().After(i.Expires) {
		return authDenied()
	}
	tx, e := b.Store.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer rollback(ctx, tx)
	var binding authBinding
	var raw []byte
	if e = tx.QueryRow(ctx, `SELECT value FROM app_settings WHERE key=$1 FOR SHARE`, i.bindingKey).Scan(&raw); e != nil || json.Unmarshal(raw, &binding) != nil || binding.Account != i.AccountID {
		return authDenied()
	}
	var version int64
	var fp string
	e = tx.QueryRow(ctx, `SELECT a.credential_version,p.approved_fingerprint FROM administrator_accounts a,plugin_v2_instances p WHERE a.id=$1 AND p.id=$2 AND coalesce(a.temporary_password_expires_at>now(),true)`, i.AccountID, i.Snapshot.InstanceID).Scan(&version, &fp)
	if e != nil || fp != binding.Fingerprint || (i.Version != 0 && i.Version != version) {
		return authDenied()
	}
	i.Version = version
	if e = b.Store.fencePolicy(ctx, tx, i.Snapshot, i.Descriptor); e != nil {
		return authDenied()
	}
	return dbutil.Commit(ctx, tx)
}
func (b Broker) PostVerifyPolicy(ctx context.Context, builtinOK bool, account string) (context.Context, error) {
	if !builtinOK || !validUUID(account) {
		return ctx, authDenied()
	}
	allow := true
	ctx, e := b.Store.acquisitionChain(ctx, contracts.ChainAuthIntercept, &contracts.AuthPolicy{AccountID: account, Allow: true}, func(_ context.Context, raw json.RawMessage) bool {
		var v struct {
			Allow *bool `json:"allow"`
		}
		if contracts.Decode(raw, &v) != nil || v.Allow == nil {
			return false
		}
		allow = allow && *v.Allow
		return true
	})
	if e == nil && !allow {
		e = authDenied()
	}
	return ctx, e
}
