package onlineupdate

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type fakeSteps struct {
	fail  string
	calls []string
}

func (f *fakeSteps) step(s string) error {
	f.calls = append(f.calls, s)
	if f.fail == s {
		return errors.New("injected " + s)
	}
	return nil
}
func (f *fakeSteps) Verify(context.Context, Job) error { return f.step("verify") }
func (f *fakeSteps) Backup(_ context.Context, j *Job) error {
	j.Backup = "/private/backup"
	j.DumpSHA256 = "verified"
	return f.step("backup")
}
func (f *fakeSteps) Apply(context.Context, Job) error    { return f.step("apply") }
func (f *fakeSteps) Health(context.Context, Job) error   { return f.step("health") }
func (f *fakeSteps) Rollback(context.Context, Job) error { return f.step("restore_dump_and_old_image") }
func TestUpdateStateMachine(t *testing.T) {
	for _, tc := range []struct {
		name, fail, phase string
		calls             []string
	}{
		{"success", "", "succeeded", []string{"verify", "backup", "apply", "health"}},
		{"release mismatch", "verify", "failed", []string{"verify"}},
		{"backup failure", "backup", "failed", []string{"verify", "backup"}},
		{"migration failure", "apply", "rolled_back", []string{"verify", "backup", "apply", "restore_dump_and_old_image"}},
		{"health failure", "health", "rolled_back", []string{"verify", "backup", "apply", "health", "restore_dump_and_old_image"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			j := Job{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Phase: "queued", CreatedAt: time.Now()}
			f := &fakeSteps{fail: tc.fail}
			_ = execute(context.Background(), root, &j, f)
			saved, e := load(root, j.ID)
			if e != nil || saved.Phase != tc.phase || !reflect.DeepEqual(f.calls, tc.calls) {
				t.Fatal(e, saved, f.calls)
			}
			if e = execute(context.Background(), root, &saved, f); e != nil && tc.phase == "succeeded" {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(f.calls, tc.calls) {
				t.Fatal("terminal task replayed")
			}
		})
	}
}
func TestAppKilledRecoveryDoesNotReplayReplacement(t *testing.T) {
	for _, phase := range []string{"applying", "checking", "rolling_back"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			j := Job{ID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Phase: phase, CreatedAt: time.Now(), Backup: "verified", DumpSHA256: "proof"}
			if e := save(root, j); e != nil {
				t.Fatal(e)
			}
			cold, e := load(root, j.ID)
			if e != nil {
				t.Fatal(e)
			}
			f := &fakeSteps{}
			if e = execute(context.Background(), root, &cold, f); e != nil || cold.Phase != "rolled_back" || !reflect.DeepEqual(f.calls, []string{"restore_dump_and_old_image"}) {
				t.Fatal(e, cold, f.calls)
			}
		})
	}
}
func TestFailedRollbackRequiresOperator(t *testing.T) {
	j := Job{ID: "cccccccccccccccccccccccccccccccc", Phase: "applying"}
	f := &fakeSteps{fail: "restore_dump_and_old_image"}
	if e := execute(context.Background(), t.TempDir(), &j, f); e == nil || j.Phase != "manual_recovery" {
		t.Fatal(e, j)
	}
}
func TestUpdateAdmissionLockAndInvalidID(t *testing.T) {
	r := t.TempDir()
	release, e := lock(r, "admission")
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	if _, e = lock(r, "admission"); !errors.Is(e, ErrBusy) {
		t.Fatal(e)
	}
	if _, e = load(r, "../../secret"); !errors.Is(e, ErrUnsafe) {
		t.Fatal(e)
	}
}
func TestComposeOnlyAppImageChanges(t *testing.T) {
	old := "dingding229/mediatool@sha256:" + stringRepeat("a", 64)
	next := "dingding229/mediatool@sha256:" + stringRepeat("b", 64)
	before := []byte("services:\n  app:\n    image: " + old + "\n    user: '65532:65532'\n  postgres:\n    image: postgres:18\n")
	after, e := composeImage(before, old, next)
	if e != nil || string(after) != "services:\n  app:\n    image: "+next+"\n    user: '65532:65532'\n  postgres:\n    image: postgres:18\n" {
		t.Fatal(e, string(after))
	}
	for _, bad := range []string{string(before) + "  app:\n    image: " + old + "\n", "services:\n  postgres:\n    image: " + old + "\n"} {
		if _, e := composeImage([]byte(bad), old, next); e == nil {
			t.Fatal("ambiguous compose accepted")
		}
	}
}

type rejectedBeforeReplace struct{ fakeSteps }

func (f *rejectedBeforeReplace) Apply(context.Context, Job) error {
	f.calls = append(f.calls, "apply")
	return errors.Join(errNotApplied, errors.New("fresh digest changed"))
}
func TestFreshDigestChangeBeforeReplaceNeverRestoresDatabase(t *testing.T) {
	j := Job{ID: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Phase: "queued"}
	s := &rejectedBeforeReplace{}
	if e := execute(t.Context(), t.TempDir(), &j, s); e == nil || j.Phase != "failed" || !reflect.DeepEqual(s.calls, []string{"verify", "backup", "apply"}) {
		t.Fatal(e, j, s.calls)
	}
}

type canceledSteps struct{ fakeSteps }

func (f *canceledSteps) Rollback(ctx context.Context, _ Job) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return f.step("recovery_with_fresh_context")
}
func TestRollbackGetsIndependentBoundedContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	j := Job{ID: "ffffffffffffffffffffffffffffffff", Phase: "checking"}
	s := &canceledSteps{}
	if e := execute(ctx, t.TempDir(), &j, s); e != nil || j.Phase != "rolled_back" {
		t.Fatal(e, j)
	}
}

func TestManualRecoveryBlocksAnotherUpdate(t *testing.T) {
	root := t.TempDir()
	j := Job{ID: "1234567890abcdef1234567890abcdef", Phase: "manual_recovery", CreatedAt: time.Now()}
	if e := save(root, j); e != nil {
		t.Fatal(e)
	}
	s := Service{Root: root}
	if _, e := s.Start(t.Context(), "admin", "0.28.1", "digest"); !errors.Is(e, ErrBusy) {
		t.Fatal(e)
	}
}
