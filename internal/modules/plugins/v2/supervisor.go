package v2

import (
	"bytes"
	"context"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/dingding229/mediatool-plugin/sdk/schema"
	"github.com/dingding229/mediatool-plugin/sdk/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"io"
	"log/slog"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/plugins/v2/sandbox"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"
)

type Supervisor struct {
	Store                  Store
	Executable             string
	Logger                 *slog.Logger
	MaxWorkers             int
	PerWorkerRSS, TotalRSS uint64
}

func (s Supervisor) Run(ctx context.Context) error {
	if s.Store.DB == nil || s.Store.Vault == nil || !filepath.IsAbs(s.Executable) {
		return ErrInvalid
	}
	if s.Logger == nil {
		s.Logger = slog.Default()
	}
	if s.MaxWorkers == 0 {
		s.MaxWorkers = 8
	}
	if s.PerWorkerRSS == 0 {
		s.PerWorkerRSS = 256 << 20
	}
	if s.TotalRSS == 0 {
		s.TotalRSS = 1 << 30
	}
	capacity := min(s.MaxWorkers, int(s.TotalRSS/s.PerWorkerRSS))
	if capacity < 1 || capacity > 8 {
		return ErrInvalid
	}
	owner := jobs.UUID()
	active := map[string]bool{}
	nextMaintenance := time.Time{}
	finished := make(chan string, capacity)
	var wg sync.WaitGroup
	defer wg.Wait()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		if time.Now().After(nextMaintenance) {
			bounded, stop := context.WithTimeout(ctx, 5*time.Second)
			if e := s.Store.MaintainExtensions(bounded); e != nil && ctx.Err() == nil {
				s.Logger.Warn("Plugin extension reconciliation deferred", "module", "plugins")
			}
			stop()
			nextMaintenance = time.Now().Add(time.Second)
		}
		activeIDs := []string{}
		for id := range active {
			activeIDs = append(activeIDs, id)
		}
		bounded, stop := context.WithTimeout(ctx, 5*time.Second)
		e := s.Store.recoverProcesses(bounded, activeIDs...)
		stop()
		if e != nil && ctx.Err() == nil {
			s.Logger.Warn("Plugin process recovery deferred", "module", "plugins")
		}
		bounded, stop = context.WithTimeout(ctx, 5*time.Second)
		e = s.Store.Reclaim(bounded)
		stop()
		if e != nil && ctx.Err() == nil {
			s.Logger.Warn("Plugin lease reconciliation unavailable", "module", "plugins")
		}
		for len(active) < capacity && ctx.Err() == nil {
			excluded := []string{}
			for id := range active {
				excluded = append(excluded, id)
			}
			bounded, stop := context.WithTimeout(ctx, 5*time.Second)
			l, e := s.Store.claim(bounded, owner, excluded)
			stop()
			if errors.Is(e, pgx.ErrNoRows) {
				break
			}
			if e != nil {
				s.Logger.Warn("Plugin claim unavailable", "module", "plugins")
				break
			}
			active[l.ID] = true
			wg.Add(1)
			go func() {
				defer wg.Done()
				why := s.execute(ctx, l)
				bounded, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if e := s.Store.Finish(bounded, l, why); e != nil && !errors.Is(e, ErrConflict) {
					s.Logger.Warn("Plugin fenced finish deferred", "module", "plugins", "instance", l.ID)
				}
				finished <- l.ID
			}()
		}
		select {
		case <-ctx.Done():
			return nil
		case id := <-finished:
			delete(active, id)
		case <-tick.C:
		}
	}
}
func (s Supervisor) execute(parent context.Context, l Lease) Reason {
	// Keep the parent OS thread alive for the child's entire Pdeathsig lifetime.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if e := s.Store.EnsurePaths(l.ID); e != nil {
		return DataUnavailable
	}
	dir := filepath.Join(s.Store.Root, l.ID)
	cmd := exec.Command(s.Executable, "plugin-worker", "--plugin", l.PluginID, "--instance", l.ID, "--fingerprint", l.Fingerprint)
	cmd.Dir = dir
	cmd.WaitDelay = 5 * time.Second
	cmd.Env = workerEnvironment(dir, l.ID)
	cmd.ExtraFiles = nil
	sandbox.Configure(cmd)
	input, e := cmd.StdinPipe()
	if e != nil {
		return WorkerFailed
	}
	output, e := cmd.StdoutPipe()
	if e != nil {
		_ = input.Close()
		return WorkerFailed
	}
	tail := &stderrLimit{exceeded: make(chan struct{}), onLandlock: func() {
		s.Logger.Warn("Plugin Landlock unavailable; trusted-code mode", "module", "plugins", "instance", l.ID, "reason", "landlock_unavailable")
	}}
	cmd.Stderr = tail
	if e = sandbox.PrepareFDs(); e != nil {
		_ = input.Close()
		_ = output.Close()
		return WorkerFailed
	}
	if e = cmd.Start(); e != nil {
		_ = input.Close()
		_ = output.Close()
		return WorkerFailed
	}
	all, scanErr := sandbox.Snapshot()
	var stamp string
	for _, p := range all {
		if p.PID == cmd.Process.Pid {
			stamp = p.Start
		}
	}
	if scanErr != nil || stamp == "" {
		_ = sandbox.KillGroup(cmd.Process.Pid)
		_ = cmd.Wait()
		return ResourceLimit
	}
	registerCtx, registerCancel := context.WithTimeout(parent, 3*time.Second)
	registered, regErr := s.Store.DB.Exec(registerCtx, `UPDATE plugin_v2_instances SET worker_pid=$4,worker_start=$5 WHERE id=$1 AND generation=$2 AND owner_id=$3 AND lease_until>now() AND desired_state='enabled' AND runtime_state='starting'`, l.ID, l.Token, l.Owner, cmd.Process.Pid, stamp)
	registerCancel()
	if regErr != nil || registered.RowsAffected() != 1 {
		_ = sandbox.KillGroup(cmd.Process.Pid)
		_ = cmd.Wait()
		return WorkerFailed
	}
	// Wait runs separately so pipe IO, renewals and heartbeat can make progress.
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	peer, e := transport.New(&transport.Stream{Reader: output, Writer: input}, transport.Options{Prefix: "h", MaxPending: 8})
	if e != nil {
		_ = sandbox.KillGroup(cmd.Process.Pid)
		<-exited
		return WorkerFailed
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	pumpDone := make(chan error, 1)
	pumpStarted := false
	defer func() {
		cancel()
		if pumpStarted {
			<-pumpDone
		}
	}()
	defer func() { _ = peer.Close() }() // Close retains the primary protocol error; no payload write is acknowledged here.
	descendants := map[int]sandbox.Process{}
	cpuPrevious := map[string]time.Duration{}
	var cpuWindow time.Duration
	cpuSince := time.Now()
	gate := &hostGate{}
	if e = peer.Handle("host.call", s.Store.scopedHost(l, gate)); e != nil {
		_ = sandbox.KillGroup(cmd.Process.Pid)
		<-exited
		return WorkerFailed
	}
	ioCtx, stopIO := context.WithCancel(context.Background())
	defer stopIO()
	if e = peer.Start(ioCtx); e != nil {
		_ = sandbox.KillGroup(cmd.Process.Pid)
		<-exited
		return WorkerFailed
	}
	protocol := make(chan error, 1)
	go func() {
		phase := "initialize"
		initCtx, stop := context.WithTimeout(ctx, 5*time.Second)
		var out contracts.Initialized
		e := peer.Call(initCtx, "plugin.initialize", contracts.Initialize{ContractVersion: 2, InstanceID: l.ID, Generation: l.Token, ConfigRevision: l.Config.Revision, DefinitionFingerprint: l.Fingerprint, CatalogBuild: BuildID(), Capabilities: l.Scopes}, &out)
		stop()
		if e == nil && (out.ContractVersion != 2 || out.SchemaFingerprint != schema.Fingerprint || out.CatalogBuild != BuildID()) {
			e = ErrUnavailable
		}
		if e == nil {
			phase = "init"
			initCtx, stop = context.WithTimeout(ctx, 15*time.Second)
			e = peer.Call(initCtx, "plugin.init", l.Config, nil)
			stop()
		}
		if e == nil {
			phase = "state"
			initCtx, stop = context.WithTimeout(ctx, 2*time.Second)
			var ready bool
			e = peer.Call(initCtx, "plugin.get_state", nil, &ready)
			stop()
			if e == nil && !ready {
				e = ErrUnavailable
			}
		}
		if e == nil {
			phase = "describe"
			describeCtx, stop := context.WithTimeout(ctx, 5*time.Second)
			var snapshot contracts.Snapshot
			e = peer.Call(describeCtx, "plugin.describe", nil, &snapshot)
			if e == nil {
				phase = "publish"
				e = s.Store.Publish(describeCtx, l, snapshot)
			}
			stop()
		}
		if e != nil {
			code := "internal"
			var rpc *contracts.Error
			var pg *pgconn.PgError
			switch {
			case errors.As(e, &rpc):
				code = "rpc_" + strconv.Itoa(int(rpc.Code))
			case errors.As(e, &pg):
				code = "db_" + pg.Code
			case errors.Is(e, ErrInvalid):
				code = "invalid"
			case errors.Is(e, ErrConflict):
				code = "conflict"
			case errors.Is(e, context.DeadlineExceeded):
				code = "deadline"
			case errors.Is(e, context.Canceled):
				code = "canceled"
			}
			s.Logger.Warn("Plugin startup failed", "module", "plugins", "instance", l.ID, "stage", phase, "error_class", code)
		}
		protocol <- e
	}()
	var result Reason = WorkerFailed
	running := false
	waited := false
	renew := time.NewTicker(5 * time.Second)
	defer renew.Stop()
	watch := time.NewTicker(200 * time.Millisecond)
	defer watch.Stop()
loop:
	for {
		select {
		case <-ctx.Done():
			if parent.Err() != nil {
				result = NoReason
			}
			break loop
		case <-pumpDone:
			pumpStarted = false
			break loop
		case <-peer.Done():
			break loop
		case <-exited:
			waited = true
			break loop
		case e := <-protocol:
			if e != nil {
				break loop
			}
			running = true
			if e = s.Store.Renew(ctx, l, true); e != nil {
				break loop
			}
			pumpStarted = true
			go func() { pumpDone <- s.Store.pump(ctx, l, peer, gate) }()
		case <-renew.C:
			bounded, stop := context.WithTimeout(ctx, 3*time.Second)
			e := s.Store.Renew(bounded, l, running)
			if e == nil {
				var pong struct {
					Nonce string `json:"nonce"`
				}
				e = peer.Call(bounded, "rpc.ping", struct {
					Nonce string `json:"nonce"`
				}{l.Token}, &pong)
				if e == nil && pong.Nonce != l.Token {
					e = ErrUnavailable
				}
			}
			stop()
			if e != nil {
				break loop
			}
		case <-tail.exceeded:
			result = ResourceLimit
			break loop
		case <-watch.C:
			if runtime.GOOS != "linux" {
				continue
			}
			all, e := sandbox.Snapshot()
			if e != nil {
				result = ResourceLimit
				break loop
			}
			var rss uint64
			for id, old := range descendants {
				alive := false
				for _, now := range all {
					if now.PID == id && now.Start == old.Start {
						alive = true
						descendants[id] = now
						break
					}
				}
				if !alive {
					delete(descendants, id)
				}
			}
			selected := append(sandbox.Tree(cmd.Process.Pid, all), sandbox.Orphans(l.ID, all)...)
			for _, p := range selected {
				descendants[p.PID] = p
			}
			for _, p := range descendants {
				rss += p.RSS
			}
			nextCPU := map[string]time.Duration{}
			for _, p := range descendants {
				key := p.Start + ":" + strconv.Itoa(p.PID)
				if previous, ok := cpuPrevious[key]; ok && p.CPU >= previous {
					cpuWindow += p.CPU - previous
				}
				nextCPU[key] = p.CPU
			}
			cpuPrevious = nextCPU
			if elapsed := time.Since(cpuSince); elapsed >= 5*time.Second {
				if cpuWindow > elapsed+elapsed/4 {
					result = ResourceLimit
					break loop
				}
				cpuSince = time.Now()
				cpuWindow = 0
			}
			if rss > s.PerWorkerRSS {
				s.Logger.Warn("Plugin RSS limit reached", "module", "plugins", "instance", l.ID, "rss_bytes", rss, "limit_bytes", s.PerWorkerRSS)
				result = ResourceLimit
				break loop
			}
		}
	}
	stopCtx, stopGrace := context.WithTimeout(context.Background(), 10*time.Second)
	if !waited {
		if e := peer.Call(stopCtx, "plugin.stop_service", struct{}{}, nil); e != nil {
			s.Logger.Debug("Plugin graceful stop unavailable; group termination follows", "module", "plugins", "instance", l.ID)
		}
	}
	stopGrace()
	cancel()
	_ = peer.Close()
	if all, e := sandbox.Snapshot(); e == nil {
		for _, p := range append(sandbox.Tree(cmd.Process.Pid, all), sandbox.Tagged(l.ID, all)...) {
			descendants[p.PID] = p
		}
	}
	cleanupOK := true
	if e := sandbox.KillGroup(cmd.Process.Pid); e != nil {
		cleanupOK = false
		s.Logger.Warn("Plugin group signal failed", "module", "plugins", "instance", l.ID)
	}
	for _, p := range descendants {
		if e := sandbox.KillKnown(p); e != nil {
			cleanupOK = false
			s.Logger.Warn("Plugin descendant signal failed", "module", "plugins", "instance", l.ID)
		}
	}
	if !waited {
		<-exited
	}
	if all, e := sandbox.Snapshot(); e == nil {
		for _, p := range sandbox.Tagged(l.ID, all) {
			descendants[p.PID] = p
			if e = sandbox.KillKnown(p); e != nil {
				cleanupOK = false
			}
		}
	}
	owned := make([]sandbox.Process, 0, len(descendants))
	for _, p := range descendants {
		owned = append(owned, p)
	}
	if e := sandbox.ReapOwned(owned); e != nil {
		cleanupOK = false
		s.Logger.Warn("Plugin adopted child reap deferred", "module", "plugins", "instance", l.ID)
	}

	if e := sandbox.ReapGroup(cmd.Process.Pid); e != nil {
		s.Logger.Warn("Plugin descendant reap deferred", "module", "plugins", "instance", l.ID)
	} else if cleanupOK {
		bounded, stop := context.WithTimeout(context.Background(), 3*time.Second)
		_, e = s.Store.DB.Exec(bounded, `UPDATE plugin_v2_instances SET worker_pid=0,worker_start='' WHERE id=$1 AND generation=$2 AND owner_id=$3 AND worker_pid=$4 AND worker_start=$5`, l.ID, l.Token, l.Owner, cmd.Process.Pid, stamp)
		stop()
		if e != nil {
			s.Logger.Warn("Plugin process reference release deferred", "module", "plugins", "instance", l.ID)
		}
	}
	if tail.warned() {
		s.Logger.Warn("Plugin reported best-effort isolation or stderr output", "module", "plugins", "instance", l.ID, "reason", tail.category())
	}
	return result
}

// Consume all stderr without retaining/forwarding raw secret-bearing errors or unbounded output.
type stderrLimit struct {
	window     time.Time
	mu         sync.Mutex
	bytes      int
	tail       []byte
	landlock   bool
	exceeded   chan struct{}
	over       bool
	onLandlock func()
}

func (s *stderrLimit) Write(b []byte) (int, error) {
	s.mu.Lock()
	if s.window.IsZero() || time.Since(s.window) >= time.Minute {
		s.window = time.Now()
		s.bytes = 0
	}
	if len(b) > 65536-s.bytes {
		if !s.over {
			s.over = true
			if s.exceeded != nil {
				close(s.exceeded)
			}
		}
		s.bytes = 65536
	} else {
		s.bytes += len(b)
	}
	chunk := b
	if len(chunk) > 4096 {
		chunk = chunk[len(chunk)-4096:]
	}
	s.tail = append(s.tail, chunk...)
	if len(s.tail) > 4096 {
		s.tail = append([]byte{}, s.tail[len(s.tail)-4096:]...)
	}
	warn := !s.landlock && bytes.Contains(s.tail, []byte("reason=landlock_unavailable"))
	if warn {
		s.landlock = true
	}
	callback := s.onLandlock
	s.mu.Unlock()
	if warn && callback != nil {
		callback()
	}
	return len(b), nil
}
func (s *stderrLimit) warned() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bytes > 0 && !s.landlock
}

var _ io.Writer = (*stderrLimit)(nil)

func workerEnvironment(dir, instance string) []string {
	return []string{"PATH=/usr/bin:/bin", "HOME=" + filepath.Join(dir, "data"), "TMPDIR=" + filepath.Join(dir, "tmp"), "LANG=C.UTF-8", "TZ=UTC", "GOMEMLIMIT=192MiB", "GOMAXPROCS=2", "MEDIATOOL_PLUGIN_INSTANCE=" + instance}
}

// category reports only a bounded constant, never raw stderr (which may contain
// business payloads or secrets). This is diagnostic only, not a security decision.
func (s *stderrLimit) category() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range []string{"newosproc", "pthread_create", "Resource temporarily unavailable"} {
		if bytes.Contains(s.tail, []byte(v)) {
			return "worker_process_limit"
		}
	}
	for _, v := range []string{"out of memory", "cannot allocate memory", "ThreadSanitizer"} {
		if bytes.Contains(s.tail, []byte(v)) {
			return "worker_memory_limit"
		}
	}
	return "worker_stderr"
}
