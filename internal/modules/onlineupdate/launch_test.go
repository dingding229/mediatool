package onlineupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func queuedFixture(t *testing.T) (*Service, *docker) {
	t.Helper()
	d := testDocker()
	s := &Service{Root: t.TempDir()}
	if e := save(s.Root, Job{ID: d.task, Phase: "queued", CreatedAt: time.Now()}); e != nil {
		t.Fatal(e)
	}
	return s, d
}
func TestUncertainHelperInspectNeverCreatesAnother(t *testing.T) {
	s, d := queuedFixture(t)
	calls := 0
	d.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || !strings.Contains(r.URL.Path, "/containers/") {
			t.Fatal("unexpected write", r.Method, r.URL)
		}
		return nil, errors.New("lost inspect response")
	})}
	s.launchDocker(t.Context(), d, d.task)
	j, e := load(s.Root, d.task)
	if e != nil || j.Phase != "queued" || calls != 1 {
		t.Fatal(e, j, calls)
	}
}
func TestLostStartResponsePreservesRunningHelperAndPhase(t *testing.T) {
	s, d := queuedFixture(t)
	running := false
	starts := 0
	d.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/start") {
			starts++
			running = true
			j, e := load(s.Root, d.task)
			if e != nil {
				t.Fatal(e)
			}
			if e = advance(s.Root, &j, "applying"); e != nil {
				t.Fatal(e)
			}
			return nil, errors.New("response lost after start")
		}
		if r.Method != "GET" {
			t.Fatal("unexpected API", r.Method, r.URL)
		}
		c := Container{ID: strings.Repeat("e", 64), Config: map[string]any{"Labels": map[string]any{label: d.task, "io.mediatool.project": d.project()}}}
		c.State.Running = running
		b, _ := json.Marshal(c)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(b)))}, nil
	})}
	s.startHelper(t.Context(), d, d.task, d.helperName())
	s.launchDocker(t.Context(), d, d.task)
	j, e := load(s.Root, d.task)
	if e != nil || j.Phase != "applying" || starts != 1 {
		t.Fatal(e, j, starts)
	}
}
func TestDefiniteStartFailureOnlyCleansOwnedStoppedHelper(t *testing.T) {
	s, d := queuedFixture(t)
	deleted := 0
	d.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		code := 200
		body := []byte(`{}`)
		switch r.Method {
		case "GET":
			c := Container{ID: strings.Repeat("e", 64), Config: map[string]any{"Labels": map[string]any{label: d.task, "io.mediatool.project": d.project()}}}
			body, _ = json.Marshal(c)
		case "POST":
			code = 400
		case "DELETE":
			deleted++
			code = 204
		default:
			t.Fatal(r.Method)
		}
		return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}
	s.startHelper(context.Background(), d, d.task, d.helperName())
	j, e := load(s.Root, d.task)
	if e != nil || j.Phase != "failed" || j.Failure != "updater_start_failed" || deleted != 1 {
		t.Fatal(e, j, deleted)
	}
}

func TestUnknownDockerStateNeverRestoresDatabase(t *testing.T) {
	d := testDocker()
	work := t.TempDir()
	d.self.Config["Labels"].(map[string]any)["com.docker.compose.project.working_dir"] = work
	dir := filepath.Join(work, "private", "backup")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	dump := []byte("verified synthetic dump")
	sum := sha256.Sum256(dump)
	old := fmtString(d.self.Config["Image"])
	j := Job{FromImage: old, Backup: dir, DumpSHA256: hex.EncodeToString(sum[:])}
	j.Target.Digest = strings.TrimPrefix(d.target, "dingding229/mediatool@")
	before := []byte("services:\n  app:\n    image: " + old + "\n")
	after, e := composeImage(before, old, d.target)
	if e != nil {
		t.Fatal(e)
	}
	for p, b := range map[string][]byte{filepath.Join(dir, "database.dump"): dump, filepath.Join(dir, "docker-compose.before.yml"): before, filepath.Join(work, "docker-compose.yml"): after} {
		if e = os.WriteFile(p, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	d.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("Docker inspect unavailable") })}
	r := runner{d: d, runPG: func(context.Context, string, []string, []string, io.Reader, io.Writer) error {
		t.Fatal("database restoration attempted without stopped-app proof")
		return nil
	}}
	if e = r.Rollback(t.Context(), j); e == nil {
		t.Fatal("unknown daemon state accepted")
	}
	current, e := os.ReadFile(filepath.Join(work, "docker-compose.yml"))
	if e != nil || string(current) != string(after) {
		t.Fatal("Compose altered on unknown state", e)
	}
}
