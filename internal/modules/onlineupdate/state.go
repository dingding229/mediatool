// Package onlineupdate executes only verified immutable releases for this app.
package onlineupdate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"media-tool/internal/modules/updatecheck"
	deferutil "media-tool/internal/platform/cleanup"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const Root = "/data/online-update"
const label = "io.mediatool.online-update"

var ErrUnsafe = errors.New("在线更新安全校验失败，未执行操作")
var errNotApplied = errors.New("replacement not started")
var ErrBusy = errors.New("已有更新正在执行，请等待结果")
var idPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type Event struct {
	At    time.Time `json:"at"`
	Phase string    `json:"phase"`
}
type Job struct {
	ID          string              `json:"id"`
	Actor       string              `json:"actor"`
	FromVersion string              `json:"fromVersion"`
	FromSchema  int64               `json:"fromSchema"`
	FromImage   string              `json:"fromImage"`
	Target      updatecheck.Release `json:"target"`
	Phase       string              `json:"phase"`
	CreatedAt   time.Time           `json:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt"`
	Backup      string              `json:"backup"`
	DumpSHA256  string              `json:"dumpSHA256"`
	Events      []Event             `json:"events"`
	Failure     string              `json:"failure,omitempty"`
}

func (j Job) Done() bool {
	return j.Phase == "succeeded" || j.Phase == "rolled_back" || j.Phase == "failed" || j.Phase == "manual_recovery"
}
func newID() (string, error) {
	b := make([]byte, 16)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func atomicFile(p string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(p), ".update-")
	if e != nil {
		return e
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	c := f.Close()
	if e != nil {
		return e
	}
	if c != nil {
		return c
	}
	if os.Geteuid() == 0 && strings.HasPrefix(filepath.Clean(p), Root+"/") {
		if e = os.Chown(f.Name(), 65532, 65532); e != nil {
			return e
		}
	}
	if e = os.Rename(f.Name(), p); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(p))
	if e != nil {
		return e
	}
	defer deferutil.Close(d, "Descriptor teardown only; primary operation and persistence results are checked")
	return d.Sync()
}
func prepare(root string) error {
	if e := os.MkdirAll(root, 0700); e != nil {
		return e
	}
	if os.Geteuid() == 0 {
		return os.Chown(root, 65532, 65532)
	}
	return nil
}
func save(root string, j Job) error {
	if !idPattern.MatchString(j.ID) {
		return ErrUnsafe
	}
	b, e := json.Marshal(j)
	if e != nil {
		return e
	}
	return atomicFile(filepath.Join(root, j.ID+".json"), b)
}
func load(root, id string) (Job, error) {
	var j Job
	if !idPattern.MatchString(id) {
		return j, ErrUnsafe
	}
	f, e := os.Open(filepath.Join(root, id+".json"))
	if e != nil {
		return j, e
	}
	defer deferutil.Close(f, "Descriptor teardown only; primary operation and persistence results are checked")
	e = json.NewDecoder(io.LimitReader(f, 2<<20)).Decode(&j)
	if e == nil && j.ID != id {
		e = ErrUnsafe
	}
	return j, e
}
func latest(root string) (*Job, error) {
	files, e := filepath.Glob(filepath.Join(root, "*.json"))
	if e != nil {
		return nil, e
	}
	var out *Job
	for _, f := range files {
		id := filepath.Base(f)
		id = id[:len(id)-5]
		if !idPattern.MatchString(id) {
			continue
		}
		j, e := load(root, id)
		if e != nil {
			return nil, e
		}
		if out == nil || j.CreatedAt.After(out.CreatedAt) {
			v := j
			out = &v
		}
	}
	return out, nil
}
func advance(root string, j *Job, phase string) error {
	j.Phase = phase
	j.UpdatedAt = time.Now().UTC()
	j.Events = append(j.Events, Event{j.UpdatedAt, phase})
	return save(root, *j)
}
func lock(root, name string) (func(), error) {
	if e := prepare(root); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(filepath.Join(root, name+".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if os.Geteuid() == 0 {
		if e = f.Chown(65532, 65532); e != nil {
			deferutil.Close(f, "lock ownership failed")
			return nil, e
		}
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		deferutil.Close(f, "Lock acquisition failed; release descriptor")
		return nil, ErrBusy
	}
	return func() { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN); _ = f.Close() }, nil
}

// Steps isolates failure injection without changing budgets/assertions or Docker permissions.
type Steps interface {
	Verify(context.Context, Job) error
	Backup(context.Context, *Job) error
	Apply(context.Context, Job) error
	Health(context.Context, Job) error
	Rollback(context.Context, Job) error
}

func execute(ctx context.Context, root string, j *Job, s Steps) error {
	if j.Done() {
		return nil
	}
	// Any interruption after backup may have changed Compose/schema. Recover before
	// executing another release; never replay an uncertain replacement.
	if j.Phase == "applying" || j.Phase == "checking" || j.Phase == "rolling_back" {
		return rollback(ctx, root, j, s)
	}
	if e := advance(root, j, "verifying"); e != nil {
		return e
	}
	if e := s.Verify(ctx, *j); e != nil {
		j.Failure = "release_hub_mismatch_or_unavailable"
		_ = advance(root, j, "failed")
		return e
	}
	if pre, ok := s.(interface {
		Preflight(context.Context, *Job) error
	}); ok {
		if e := advance(root, j, "preflighting"); e != nil {
			return e
		}
		if e := pre.Preflight(ctx, j); e != nil {
			j.Failure = "preflight_rejected"
			return errors.Join(e, advance(root, j, "failed"))
		}
	}
	if e := advance(root, j, "backing_up"); e != nil {
		return e
	}
	if e := s.Backup(ctx, j); e != nil {
		j.Failure = "backup_failed"
		_ = advance(root, j, "failed")
		return e
	}
	if e := advance(root, j, "applying"); e != nil {
		return e
	}
	if e := s.Apply(ctx, *j); e != nil {
		j.Failure = "apply_failed"
		if errors.Is(e, updatecheck.ErrUntrusted) {
			j.Failure = "release_hub_mismatch_or_unavailable"
		}
		if errors.Is(e, errNotApplied) {
			return errors.Join(e, advance(root, j, "failed"))
		}
		return rollback(ctx, root, j, s)
	}
	if e := advance(root, j, "checking"); e != nil {
		return e
	}
	if e := s.Health(ctx, *j); e != nil {
		j.Failure = "health_or_schema_failed"
		return rollback(ctx, root, j, s)
	}
	if final, ok := s.(interface {
		Finalize(context.Context, Job) error
	}); ok {
		if e := final.Finalize(ctx, *j); e != nil {
			j.Failure = "old_container_cleanup_failed"
			return errors.Join(e, advance(root, j, "manual_recovery"))
		}
	}
	return advance(root, j, "succeeded")
}
func rollback(ctx context.Context, root string, j *Job, s Steps) error {
	recovery, done := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Minute)
	defer done()
	ctx = recovery
	if e := advance(root, j, "rolling_back"); e != nil {
		return e
	}
	if e := s.Rollback(ctx, *j); e != nil {
		_ = advance(root, j, "manual_recovery")
		return e
	}
	return advance(root, j, "rolled_back")
}
