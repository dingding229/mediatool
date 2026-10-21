package v2

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/dingding229/mediatool-plugin/sdk/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"media-tool/internal/modules/jobs"
	"time"
)

func (s Store) Submit(ctx context.Context, snap Snapshot, d contracts.Descriptor, args json.RawMessage, key, service string, event any, deadline time.Time) (string, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer rollback(ctx, tx)
	id, e := s.submitTx(ctx, tx, snap, d, args, key, service, event, deadline)
	if e != nil {
		return "", e
	}
	e = tx.Commit(ctx)
	if e == nil {
		s.mailbox(snap.InstanceID).queued()
	}
	return id, e
}
func (s Store) submitTx(ctx context.Context, tx pgx.Tx, snap Snapshot, d contracts.Descriptor, args json.RawMessage, key, service string, event any, deadline time.Time) (string, error) {
	if len(key) > 500 || len(key) == 0 || len(args) > 128<<10 || contracts.ValidateJSON(args) != nil || deadline.After(time.Now().Add(15*time.Minute)) {
		return "", ErrInvalid
	}
	id := jobs.UUID()
	cipher, e := s.seal("call-args", id, args)
	if e != nil {
		return "", e
	}
	hash := digest(append([]byte(snap.InstanceID+":"+d.Kind+":"+d.Handler+":"), args...))
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,63))`, snap.InstanceID); e != nil {
		return "", e
	}
	var savedHash string
	e = tx.QueryRow(ctx, `SELECT id::text,request_hash FROM plugin_v2_runs WHERE dedupe_key=$1`, key).Scan(&id, &savedHash)
	if e == nil {
		if savedHash != hash {
			return "", ErrConflict
		}
		return id, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return "", e
	}
	var count int
	e = tx.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs WHERE instance_id=$1 AND state IN('pending','running')`, snap.InstanceID).Scan(&count)
	if e != nil {
		return "", e
	}
	if count >= 16 {
		return "", contracts.NewError(contracts.Busy)
	}
	// Short shared lock serializes the commit with revocation, never with an RPC.
	var live bool
	e = tx.QueryRow(ctx, `SELECT true FROM plugin_v2_instances WHERE id=$1 AND desired_state='enabled' AND generation=$2 AND grant_revision=$3 AND lease_until>clock_timestamp() FOR SHARE`, snap.InstanceID, snap.Generation, snap.GrantRevision).Scan(&live)
	if e != nil {
		return "", ErrConflict
	}
	var allowed int
	if e = tx.QueryRow(ctx, `SELECT count(DISTINCT scope) FROM plugin_v2_grants WHERE instance_id=$1 AND scope=ANY($2::text[]) AND fingerprint=(SELECT approved_fingerprint FROM plugin_v2_instances WHERE id=$1)`, snap.InstanceID, d.RequiredScopes).Scan(&allowed); e != nil {
		return "", e
	}
	unique := map[string]bool{}
	for _, scope := range d.RequiredScopes {
		unique[scope] = true
	}
	if allowed != len(unique) {
		return "", ErrConflict
	}
	var serviceID any
	if service != "" {
		serviceID = service
	}
	_, e = tx.Exec(ctx, `INSERT INTO plugin_v2_runs(id,instance_id,snapshot_id,generation,grant_revision,kind,handler,service_id,event_cursor,dedupe_key,request_hash,args_ciphertext,deadline_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, id, snap.InstanceID, snap.ID, snap.Generation, snap.GrantRevision, d.Kind, d.Handler, serviceID, event, key, hash, cipher, deadline)
	if e != nil {
		var pg *pgconn.PgError
		if errors.As(e, &pg) && pg.ConstraintName == "plugin_v2_one_service_inflight" {
			return "", contracts.NewError(contracts.Busy)
		}
		return "", e
	}
	return id, nil
}
func (s Store) Invoke(ctx context.Context, id, kind, key string, args json.RawMessage, operationKey string) (json.RawMessage, error) {
	snap, d, e := s.Descriptor(ctx, id, kind, key)
	if e != nil {
		return nil, e
	}
	return s.invokeSnapshot(ctx, snap, d, args, operationKey)
}
func (s Store) invokeSnapshot(ctx context.Context, snap Snapshot, d contracts.Descriptor, args json.RawMessage, operationKey string) (json.RawMessage, error) {
	id := snap.InstanceID
	if operationKey == "" {
		operationKey = jobs.UUID()
	}
	deadline := time.Now().Add(5 * time.Second)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	run, e := s.Submit(ctx, snap, d, args, "invoke:"+id+":"+d.Handler+":"+operationKey, "", nil, deadline)
	if e != nil {
		return nil, e
	}
	tick := time.NewTicker(30 * time.Millisecond)
	defer tick.Stop()
	for {
		// Observe before SQL so a commit between the query and select cannot be lost.
		changed := s.mailbox(id).observed()
		var state RunState
		var cipher []byte
		e = s.DB.QueryRow(ctx, `SELECT state,result_ciphertext FROM plugin_v2_runs WHERE id=$1`, run).Scan(&state, &cipher)
		if e != nil {
			return nil, e
		}
		switch state {
		case RunSucceeded:
			return s.open("call-result", run, cipher)
		case RunFailed:
			return nil, contracts.NewError(contracts.NeedsAction)
		case RunCanceled:
			return nil, ErrConflict
		case RunUnknown:
			return nil, contracts.NewError(contracts.ResultUnknown)
		}
		select {
		case <-ctx.Done():
			cleanupBudget := time.Second
			if deadline, ok := ctx.Value(policyDeadline{}).(time.Time); ok {
				remaining := time.Until(deadline)
				// Pure policy results are discarded. Existing deadline convergence owns
				// any expired mailbox row; never block beyond the total policy budget.
				if remaining <= 0 {
					return nil, contracts.NewError(contracts.ResultUnknown)
				}
				cleanupBudget = min(cleanupBudget, remaining)
			}
			cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), cleanupBudget)
			_, err := s.DB.Exec(cleanup, `UPDATE plugin_v2_runs SET state=CASE WHEN state='running' THEN 'unknown' ELSE 'canceled' END,reason='deadline',finished_at=now() WHERE id=$1 AND state IN('pending','running')`, run)
			stop()
			return nil, errors.Join(contracts.NewError(contracts.ResultUnknown), err)
		case <-changed:
		case <-tick.C:
		}
	}
}

// pump has one RPC at a time per instance. It claims only after the peer lane is
// available; host callbacks use independent SQL transactions and never this lease.
func (s Store) pump(ctx context.Context, l Lease, peer *transport.Peer, gate *hostGate) error {
	lane := &mailboxLane{wake: make(chan struct{}, 1), changed: make(chan struct{})}
	key := mailboxKey{s.DB, l.ID}
	if _, loaded := mailboxLanes.LoadOrStore(key, lane); loaded {
		return ErrConflict
	}
	defer mailboxLanes.CompareAndDelete(key, lane)
	defer lane.completed()
	lane.queued()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-lane.wake:
		case <-tick.C:
		}
		var id, handler, runKind string
		var raw []byte
		var deadline time.Time
		token := jobs.UUID()
		e := s.DB.QueryRow(ctx, `WITH next AS(SELECT r.id FROM plugin_v2_runs r JOIN plugin_v2_instances i ON i.id=r.instance_id WHERE r.instance_id=$1 AND r.generation=$2 AND r.grant_revision=$3 AND r.state='pending' AND r.deadline_at>clock_timestamp() AND (r.kind<>'service' OR EXISTS(SELECT 1 FROM jobs j WHERE r.dedupe_key='service-job:'||j.id::text AND j.state='succeeded')) AND i.desired_state='enabled' AND i.generation=$2 AND i.grant_revision=$3 AND i.lease_until>clock_timestamp() ORDER BY r.created_at,r.id FOR UPDATE OF r SKIP LOCKED LIMIT 1) UPDATE plugin_v2_runs r SET state='running',lease_token=$4,lease_until=now()+interval '35 seconds' FROM next WHERE r.id=next.id RETURNING r.id::text,r.handler,r.args_ciphertext,r.deadline_at,r.kind`, l.ID, l.Token, l.GrantRevision, token).Scan(&id, &handler, &raw, &deadline, &runKind)
		if errors.Is(e, pgx.ErrNoRows) {
			continue
		}
		if e != nil {
			return e
		}
		var authorized bool
		e = s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM plugin_v2_runs r JOIN plugin_v2_snapshots s ON s.id=r.snapshot_id JOIN plugin_v2_instances i ON i.id=r.instance_id,jsonb_array_elements(s.descriptors->'descriptors') d WHERE r.id=$1 AND r.lease_token=$2 AND r.state='running' AND i.desired_state='enabled' AND i.generation=r.generation AND i.grant_revision=r.grant_revision AND d->>'handler'=r.handler AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements_text(d->'requiredScopes') AS needed(scope) WHERE NOT EXISTS(SELECT 1 FROM plugin_v2_grants g WHERE g.instance_id=i.id AND g.scope=needed.scope AND g.fingerprint=i.approved_fingerprint)))`, id, token).Scan(&authorized)
		if e != nil {
			return e
		}
		if !authorized {
			if e = s.finishCall(ctx, id, token, RunCanceled, RunRevoked, nil); e != nil {
				return e
			}
			continue
		}
		if runKind == "event" {
			fresh, err := s.queuedEventFresh(ctx, id)
			if err != nil {
				return err
			}
			if !fresh {
				if err = s.finishCall(ctx, id, token, RunCanceled, RunDeadline, nil); err != nil {
					return err
				}
				continue
			}
		}
		args, e := s.open("call-args", id, raw)
		if e != nil {
			return e
		}
		if deadline.After(time.Now().Add(30 * time.Second)) {
			deadline = time.Now().Add(30 * time.Second)
		}
		bounded, cancel := context.WithDeadline(ctx, deadline)
		watchDone := make(chan struct{})
		go func() {
			defer close(watchDone)
			t := time.NewTicker(100 * time.Millisecond)
			defer t.Stop()
			for {
				select {
				case <-bounded.Done():
					return
				case <-t.C:
					var live bool
					err := s.DB.QueryRow(bounded, `SELECT EXISTS(SELECT 1 FROM plugin_v2_instances i JOIN plugin_v2_runs r ON r.instance_id=i.id WHERE r.id=$1 AND r.state='running' AND r.lease_token=$2 AND i.desired_state='enabled' AND i.generation=r.generation AND i.grant_revision=r.grant_revision)`, id, token).Scan(&live)
					if err != nil || !live {
						cancel()
						return
					}
				}
			}
		}()
		var scopeRaw []byte
		var kind, extensionID string
		e = s.DB.QueryRow(bounded, `SELECT d->>'kind',d->>'id',d->'requiredScopes' FROM plugin_v2_runs r JOIN plugin_v2_snapshots s ON s.id=r.snapshot_id,jsonb_array_elements(s.descriptors->'descriptors') d WHERE r.id=$1 AND d->>'handler'=r.handler`, id).Scan(&kind, &extensionID, &scopeRaw)
		if e != nil {
			cancel()
			<-watchDone
			return e
		}
		if kind == "api" || kind == "chain" || kind == "module" || kind == "action" || kind == "tool" {
			var scopes []string
			if json.Unmarshal(scopeRaw, &scopes) != nil {
				cancel()
				<-watchDone
				return ErrInvalid
			}
			gate.enterFor(scopes, kind == "action" || kind == "tool")
		}
		var result json.RawMessage
		e = peer.Call(bounded, "plugin.invoke", contracts.Invocation{Handler: handler, Args: args}, &result)
		canceled := bounded.Err() != nil
		cancel()
		<-watchDone
		state, reason := RunSucceeded, RunNoReason
		var cipher []byte
		if canceled {
			state, reason = RunUnknown, RunDeadline
		} else if e != nil {
			var rpc *contracts.Error
			if errors.As(e, &rpc) && (rpc.Code == contracts.NeedsAction || rpc.Code == contracts.InvalidParams || rpc.Code == contracts.PermissionDenied || rpc.Code == contracts.NotFound || rpc.Code == contracts.Busy) {
				state, reason = RunFailed, RunPluginFailed
			} else {
				state, reason = RunUnknown, RunWorkerLost
			}
		} else if kind == "tool" && len(result) > contracts.ToolOutputLimit {
			state, reason = RunUnknown, RunInvalidResult
		} else if len(result) > 128<<10 || contracts.ValidateJSON(result) != nil {
			state, reason = RunUnknown, RunInvalidResult
		} else if (kind == "chain" || kind == "module") && !validExtensionResult(kind, extensionID, args, result) {
			state, reason = RunFailed, RunInvalidResult
		} else {
			cipher, e = s.seal("call-result", id, result)
			if e != nil {
				return e
			}
		}
		if state != RunUnknown {
			gate.leave()
		} // Unknown/non-cooperating API keeps its ceiling until group termination.
		done, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		err := s.finishCall(done, id, token, state, reason, cipher)
		stop()
		if err != nil {
			return err
		}
		lane.completed()

		var needs *contracts.Error
		if (kind == "service" || kind == "event") && errors.As(e, &needs) && needs.Code == contracts.NeedsAction {
			bounded, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			pauseErr := s.pauseForAction(bounded, l)
			stop()
			if pauseErr != nil {
				return pauseErr
			}
			return contracts.NewError(contracts.NeedsAction)
		}

		lane.queued() // Drain committed backlog without spending another policy budget on polling.
		if state == RunUnknown {
			return contracts.NewError(contracts.ResultUnknown)
		} // non-cooperating call: supervisor kills the group
	}
}

func (s Store) finishCall(ctx context.Context, id, token string, state RunState, reason RunReason, cipher []byte) error {
	tag, e := s.DB.Exec(ctx, `UPDATE plugin_v2_runs r SET state=$3,reason=$4,result_ciphertext=$5,finished_at=now() FROM plugin_v2_instances i WHERE r.id=$1 AND r.lease_token=$2 AND r.state='running' AND r.lease_until>clock_timestamp() AND i.id=r.instance_id AND i.generation=r.generation AND i.grant_revision=r.grant_revision AND i.desired_state='enabled' AND i.lease_until>clock_timestamp()`, id, token, state, reason, cipher)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		// The caller/maintenance may have terminalized a deadline receipt just
		// before a cooperative RPC returned. Never overwrite that receipt, but
		// do not kill a healthy lane for losing this completion race. Unknown
		// RPC outcomes still terminate the worker; revoked generations still fail.
		var expired bool
		if state != RunUnknown {
			e = s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM plugin_v2_runs r JOIN plugin_v2_instances i ON i.id=r.instance_id WHERE r.id=$1 AND r.lease_token=$2 AND r.state IN('unknown','canceled') AND r.reason='deadline' AND i.generation=r.generation AND i.grant_revision=r.grant_revision AND i.desired_state='enabled' AND i.runtime_state='running' AND i.lease_until>clock_timestamp())`, id, token).Scan(&expired)
		}
		if e != nil {
			return e
		}
		if !expired {
			return ErrConflict
		}
	}

	return nil
}

func validHookResult(raw json.RawMessage) bool {
	var value contracts.NameRecognition
	return contracts.Decode(raw, &value) == nil && contracts.ValidateName(value) == nil
}

func validExtensionResult(kind, id string, args, result json.RawMessage) bool {
	if kind == "chain" && (id == "workflow.execution" || id == "auth.intercept") {
		var q struct {
			Allow *bool `json:"allow"`
		}
		return contracts.Decode(result, &q) == nil && q.Allow != nil
	}
	if kind == "chain" && id == "auth.verification" {
		var v contracts.AuthAssertion
		return contracts.Decode(result, &v) == nil && v.Subject != "" && len(v.Subject) <= 200 && len(v.Issuer) <= 200 && len(v.Audience) <= 200 && v.ExpiresAt > 0
	}

	if kind == "module" && (id == "notifications.channel" || id == "mediaserver.refresh") {
		return validEffectResult(id, args, result)
	}
	if kind == "chain" && (id == "media.recognize.convert" || id == "subscribe.episodes.refresh" || id == "subscribe.completion.check" || id == "discover.source" || id == "recommend.source") || kind == "module" && id == "metadata.identify" {
		return validDiscoveryResult(kind, id, args, result)
	}
	if kind == "chain" && (id == "resource.selection" || id == "resource.download") {
		return validAcquisitionResult(id, args, result)
	}
	if kind == "chain" && (id == "transfer.rename" || id == "transfer.rename.build" || id == "transfer.intercept" || id == "transfer.overwrite.check" || id == "storage.operation") {
		return validOrganizingResult(id, args, result)
	}
	return (kind == "chain" && id == "name.recognize" || kind == "module" && id == "metadata.name_recognize") && validHookResult(result)
}
