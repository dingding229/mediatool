package messaging

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/adapters/telegram"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/maintenance"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/outbox"
	"media-tool/internal/platform/pluginstate"
	"media-tool/internal/platform/trace"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrIngress = errors.New("command ingress rejected")

type CommandIngress struct {
	DB          *pgxpool.Pool
	Connections *integrations.Manager
	Plugins     pluginv2.Store
	Owner       string
	Clock       func() time.Time
}

func (i CommandIngress) now() time.Time {
	if i.Clock != nil {
		return i.Clock().UTC()
	}
	return time.Now().UTC()
}

type actorRate struct {
	Minute int64 `json:"minute"`
	Used   int   `json:"used"`
}
type pollState struct {
	Offset       int64                `json:"offset"`
	LastReceived time.Time            `json:"lastReceived"`
	Owner        string               `json:"owner"`
	Until        time.Time            `json:"until"`
	Minute       int64                `json:"minute"`
	Used         int                  `json:"used"`
	Actors       map[string]actorRate `json:"actors"`
}
type actionState struct {
	Actor, Command, Arguments, InstanceID, SnapshotID string
	Revision                                          int64
	Expires                                           int64
	Used                                              bool
}

func allowedID(raw string, id int64) bool {
	if id == 0 {
		return false
	}
	for _, s := range strings.Split(raw, ",") {
		if strings.TrimSpace(s) == strconv.FormatInt(id, 10) {
			return true
		}
	}
	return false
}
func enabled(c integrations.Credentials) bool {
	return c.Record.Enabled && c.Record.Settings["inboundEnabled"] == true && c.Values["botToken"] != "" && strings.TrimSpace(c.String("allowedChatIds")) != "" && strings.TrimSpace(c.String("allowedUserIds")) != ""
}
func pollKey(c integrations.Credentials) string {
	sum := sha256.Sum256([]byte(c.Values["botToken"]))
	return "plugins.ingress.telegram." + c.Record.ID + "." + hex.EncodeToString(sum[:8])
}
func actorKey(c integrations.Credentials, chat, user int64) string {
	mac := hmac.New(sha256.New, []byte(c.Values["botToken"]))
	_, _ = mac.Write([]byte(c.Record.ID + ":" + strconv.FormatInt(chat, 10) + ":" + strconv.FormatInt(user, 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:16])
}

var commandName = regexp.MustCompile(`^[a-zA-Z0-9_]{1,64}$`)
var commandSecret = regexp.MustCompile(`(?i)(https?://\S+|magnet:\S+|\S*/\S+|bearer\s+\S+|(?:token|api[_-]?key|passkey|password|cookie|secret|pickcode)[=:]\s*\S+|[A-Za-z0-9_-]{32,})`)

func parseCommand(text string) (string, string, bool) {
	if !strings.HasPrefix(text, "/") || len([]rune(text)) > 200 {
		return "", "", false
	}
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "", "", false
	}
	name := strings.TrimPrefix(parts[0], "/")
	if !commandName.MatchString(name) {
		return "", "", false
	}
	args := strings.Join(parts[1:], " ")
	return name, commandSecret.ReplaceAllString(args, "[redacted]"), true
}
func (i CommandIngress) configuration(ctx context.Context) (integrations.Credentials, error) {
	if i.DB == nil || i.Connections == nil || i.Connections.Repository.DB == nil {
		return integrations.Credentials{}, ErrIngress
	}
	return i.Connections.Repository.Default(ctx, "telegram")
}
func (i CommandIngress) acquire(ctx context.Context, c integrations.Credentials) (int64, bool, error) {
	if !contracts.OpaqueID(i.Owner) {
		return 0, false, ErrIngress
	}
	tx, e := i.DB.Begin(ctx)
	if e != nil {
		return 0, false, e
	}
	defer deferutil.Rollback(ctx, tx)
	st := pollState{}
	if e = pluginstate.Lock(ctx, tx, pollKey(c), &st); e != nil {
		return 0, false, e
	}
	now := i.now()
	if st.Owner != "" && st.Owner != i.Owner && st.Until.After(now) {
		return 0, false, nil
	}
	if !st.LastReceived.IsZero() && now.Sub(st.LastReceived) > 6*24*time.Hour {
		st.Offset = 0
	}
	st.Owner = i.Owner
	st.Until = now.Add(40 * time.Second)
	if e = pluginstate.Put(ctx, tx, pollKey(c), st); e != nil {
		return 0, false, e
	}
	return st.Offset, true, dbutil.Commit(ctx, tx)
}
func (i CommandIngress) release(ctx context.Context, c integrations.Credentials) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_, e := i.DB.Exec(cleanup, `UPDATE app_settings SET value=value||'{"owner":""}'::jsonb WHERE key=$1 AND value->>'owner'=$2`, pollKey(c), i.Owner)
	if e != nil {
		deferutil.Report(e, "release bounded Telegram poll lease")
	}
}
func (i CommandIngress) PollOnce(ctx context.Context) (bool, error) {
	release, gateErr := maintenance.BeginWork(ctx, i.DB)
	if gateErr != nil {
		return false, nil
	}
	defer release()
	c, e := i.configuration(ctx)
	if e != nil || !enabled(c) {
		return false, nil
	}
	offset, owned, e := i.acquire(ctx, c)
	if e != nil || !owned {
		return true, e
	}
	defer i.release(ctx, c)
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	updates, e := (telegram.Client{Token: c.Values["botToken"], HTTP: i.Connections.OutboundHTTP}).Updates(bounded, offset)
	if e != nil {
		return true, e
	}
	sort.Slice(updates, func(a, b int) bool { return updates[a].ID < updates[b].ID })
	for _, u := range updates {
		if u.Callback != nil {
			e = i.HandleAction(bounded, c, u)
		} else {
			e = i.Receive(bounded, c, u)
		}
		if e != nil {
			return true, e
		}
		if u.Callback != nil {
			_ = (telegram.Client{Token: c.Values["botToken"], HTTP: i.Connections.OutboundHTTP}).AnswerCallback(bounded, u.Callback.ID)
		} // ACK failure never rolls back/replays a committed command.
	}
	return true, nil
}
func (i CommandIngress) Run(ctx context.Context) error {
	if i.Owner == "" {
		i.Owner = jobs.UUID()
	}
	nextWarning := time.Time{}
	for ctx.Err() == nil {
		active, e := i.PollOnce(ctx)
		wait := time.Second
		if !active {
			wait = 5 * time.Second
		}
		if e != nil {
			wait = 3 * time.Second
			if time.Now().After(nextWarning) {
				trace.Warn(ctx, "messaging", "Telegram入站暂不可用，将重试", "reason", "poll_unavailable")
				nextWarning = time.Now().Add(time.Minute)
			}
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
	return nil
}

// Receive authenticates before resolving a command. The one transaction consumes
// update/nonce/rate state, queues the current descriptor, and records a safe fact.
func (i CommandIngress) Receive(ctx context.Context, c integrations.Credentials, u telegram.Update) error {
	if !enabled(c) || u.ID < 0 || u.ID > 9007199254740991 {
		return ErrIngress
	}
	var chat, user int64
	var text, kind, actor string
	authorized := false
	var nonce string
	var action actionState
	now := i.now()
	if u.Message != nil {
		m := u.Message
		chat, user = m.Chat.ID, m.From.ID
		text = m.Text
		kind = "command"
		authorized = !m.From.IsBot && m.Date > now.Add(-5*time.Minute).Unix() && m.Date <= now.Add(time.Minute).Unix()
	}
	if u.Callback != nil {
		q := u.Callback
		kind = "action"
		if q.Message != nil {
			chat, user = q.Message.Chat.ID, q.From.ID
			authorized = !q.From.IsBot
			nonce = i.verifyAction(c, q.Data)
			if nonce == "" {
				authorized = false
			}
		}
	}
	authorized = authorized && allowedID(c.String("allowedChatIds"), chat) && allowedID(c.String("allowedUserIds"), user)
	if authorized {
		actor = actorKey(c, chat, user)
	}
	name, args, valid := parseCommand(text)
	args = strings.ReplaceAll(args, c.Values["botToken"], "[redacted]")
	if kind == "action" {
		valid = false
		if authorized {
			raw := []byte{}
			e := i.DB.QueryRow(ctx, `SELECT value FROM app_settings WHERE key=$1`, "plugins.ingress.callback."+nonce).Scan(&raw)
			if e == nil && json.Unmarshal(raw, &action) == nil && !action.Used && action.Actor == actor && action.Revision == c.Record.Revision && action.Expires >= now.Unix() {
				name, args, valid = action.Command, action.Arguments, true
			}
		}
	}
	var snap pluginv2.Snapshot
	var descriptor contracts.Descriptor
	if authorized && valid {
		var e error
		snap, descriptor, e = i.Plugins.CommandRoute(ctx, name)
		if e != nil {
			valid = false
		}
		if kind == "action" && (snap.InstanceID != action.InstanceID || snap.ID != action.SnapshotID) {
			valid = false
		}
	}
	tx, e := i.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer deferutil.Rollback(ctx, tx)
	var revision int64
	if e = tx.QueryRow(ctx, `SELECT revision FROM integrations WHERE id=$1 AND kind='telegram' AND enabled FOR SHARE`, c.Record.ID).Scan(&revision); e != nil || revision != c.Record.Revision {
		return ErrIngress
	}
	st := pollState{}
	if e = pluginstate.Lock(ctx, tx, pollKey(c), &st); e != nil {
		return e
	}
	if st.Owner != i.Owner || st.Until.Before(now) {
		return ErrIngress
	}
	if u.ID < st.Offset {
		return tx.Commit(ctx)
	}
	st.Offset = u.ID + 1
	st.LastReceived = now
	if st.Minute != now.Unix()/60 {
		st.Minute = now.Unix() / 60
		st.Used = 0
		st.Actors = map[string]actorRate{}
	}
	if st.Actors == nil {
		st.Actors = map[string]actorRate{}
	}
	rate := st.Actors[actor]
	if rate.Minute != st.Minute {
		rate = actorRate{Minute: st.Minute}
	}
	admitted := authorized && valid && st.Used < 60 && rate.Used < 5 && len(st.Actors) < 200
	if admitted && kind == "action" {
		current := actionState{}
		if e = pluginstate.Lock(ctx, tx, "plugins.ingress.callback."+nonce, &current); e != nil {
			return e
		}
		admitted = !current.Used && current.Actor == actor && current.Revision == revision && current.Expires >= now.Unix() && current.Command == name && current.Arguments == args
		if admitted {
			current.Used = true
			if e = pluginstate.Put(ctx, tx, "plugins.ingress.callback."+nonce, current); e != nil {
				return e
			}
		}
	}
	if admitted {
		key := "telegram:" + pollKey(c) + ":" + strconv.FormatInt(u.ID, 10)
		if e = i.Plugins.QueueCommandTx(ctx, tx, snap, descriptor, contracts.CommandInput{Channel: "telegram", Actor: actor, Arguments: args}, key); e != nil {
			return e
		}
		typ := "messaging.inbound"
		if kind == "action" {
			typ = "messaging.action.received"
		}
		if e = outbox.EmitVersionTx(ctx, tx, typ, c.Record.ID, strconv.FormatInt(u.ID, 10)+":"+pollKey(c), contracts.InboundMessage{InstanceID: snap.InstanceID, Command: name, Actor: actor, Kind: kind}); e != nil {
			return e
		}
		st.Used++
		rate.Used++
		st.Actors[actor] = rate
	}
	if e = pluginstate.Put(ctx, tx, pollKey(c), st); e != nil {
		return e
	}
	return dbutil.Commit(ctx, tx)
}
func (i CommandIngress) HandleAction(ctx context.Context, c integrations.Credentials, u telegram.Update) error {
	if u.Callback == nil {
		return ErrIngress
	}
	return i.Receive(ctx, c, u)
}
func actionMAC(c integrations.Credentials, body string) string {
	m := hmac.New(sha256.New, []byte(c.Values["botToken"]))
	_, _ = m.Write([]byte("mediatool.callback:" + c.Record.ID + ":" + body))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil)[:16])
}
func (i CommandIngress) verifyAction(c integrations.Credentials, data string) string {
	if len(data) > 64 {
		return ""
	}
	parts := strings.Split(data, ".")
	if len(parts) != 4 || parts[0] != "v1" || len(parts[1]) != 22 {
		return ""
	}
	expires, e := strconv.ParseInt(parts[2], 10, 64)
	if e != nil || expires < i.now().Unix() || expires > i.now().Add(10*time.Minute).Unix() {
		return ""
	}
	if !hmac.Equal([]byte(parts[3]), []byte(actionMAC(c, strings.Join(parts[:3], ".")))) {
		return ""
	}
	return parts[1]
}

// IssueAction is a host/admin facility, not a plugin Host API. Its signed opaque
// data can be put in a Telegram inline keyboard without disclosing credentials.
func (i CommandIngress) IssueAction(ctx context.Context, chat, user int64, command, arguments string) (string, error) {
	c, e := i.configuration(ctx)
	if e != nil || !enabled(c) || !allowedID(c.String("allowedChatIds"), chat) || !allowedID(c.String("allowedUserIds"), user) {
		return "", ErrIngress
	}
	name, args, ok := parseCommand("/" + command + " " + arguments)
	args = strings.ReplaceAll(args, c.Values["botToken"], "[redacted]")
	if !ok {
		return "", ErrIngress
	}
	snap, _, e := i.Plugins.CommandRoute(ctx, name)
	if e != nil {
		return "", e
	}
	bytes := make([]byte, 16)
	if _, e = rand.Read(bytes); e != nil {
		return "", e
	}
	nonce := base64.RawURLEncoding.EncodeToString(bytes)
	expires := i.now().Add(5 * time.Minute).Unix()
	tx, e := i.DB.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer deferutil.Rollback(ctx, tx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7064003)`); e != nil {
		return "", e
	}
	var n int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM app_settings WHERE key LIKE 'plugins.ingress.callback.%' AND (value->>'Expires')::bigint>$1`, i.now().Unix()).Scan(&n); e != nil || n >= 200 {
		return "", ErrIngress
	}
	record := actionState{InstanceID: snap.InstanceID, SnapshotID: snap.ID, Actor: actorKey(c, chat, user), Command: name, Arguments: args, Revision: c.Record.Revision, Expires: expires}
	if e = pluginstate.Lock(ctx, tx, "plugins.ingress.callback."+nonce, &record); e != nil {
		return "", e
	}
	if _, e = tx.Exec(ctx, `DELETE FROM app_settings WHERE key IN(SELECT key FROM app_settings WHERE key LIKE 'plugins.ingress.callback.%' AND (value->>'Expires')::bigint<$1 LIMIT 200)`, i.now().Unix()); e != nil {
		return "", e
	}
	if e = dbutil.Commit(ctx, tx); e != nil {
		return "", e
	}
	body := "v1." + nonce + "." + strconv.FormatInt(expires, 10)
	return body + "." + actionMAC(c, body), nil
}
