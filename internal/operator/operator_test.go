package operator

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureImage() Image {
	var i Image
	i.Os = "linux"
	i.Architecture = "amd64"
	i.Config.Labels = map[string]string{"org.opencontainers.image.revision": strings.Repeat("a", 40), "org.opencontainers.image.schema": "65"}
	i.RepoDigests = []string{"dingding229/mediatool@sha256:" + strings.Repeat("b", 64)}
	return i
}
func TestImmutableHubAMD64RevisionSchemaAndDigest(t *testing.T) {
	ref := "dingding229/mediatool:sha-" + strings.Repeat("a", 40)
	i := fixtureImage()
	if _, e := CheckImage(ref, i); e != nil {
		t.Fatal(e)
	}
	for _, ref := range []string{"local:test", "ghcr.io/owner/tool:latest", "dingding229/mediatool:latest", "dingding229/mediatool:sha-" + strings.Repeat("c", 40), "dingding229/mediatool@sha256:" + strings.Repeat("c", 64)} {
		if _, e := CheckImage(ref, i); e == nil {
			t.Fatal(ref)
		}
	}
	i.Architecture = "arm64"
	if _, e := CheckImage(ref, i); e == nil {
		t.Fatal("arm64 allowed")
	}
	i = fixtureImage()
	delete(i.Config.Labels, "org.opencontainers.image.schema")
	if _, e := CheckImage(ref, i); e == nil {
		t.Fatal("missing schema allowed")
	}
	i = fixtureImage()
	i.RepoDigests = nil
	if _, e := CheckImage(ref, i); e == nil {
		t.Fatal("missing Hub digest")
	}
}
func TestComposeReorderedServicesChangeOnlyAppAndRefuseInterpolation(t *testing.T) {
	text := "services:\n  postgres:\n    image: postgres:18\n  app:\n    image: old\n    environment:\n      KEY: keep\n  redis:\n    image: redis:8\n"
	ref := "dingding229/mediatool@sha256:" + strings.Repeat("b", 64)
	got, e := ReplaceImage(text, ref)
	if e != nil || got != strings.Replace(text, "image: old", "image: "+ref, 1) {
		t.Fatal(e, got)
	}
	for _, s := range []string{strings.Replace(text, "old", "${IMAGE}", 1), "services:\n  postgres:\n    image: old\n", text + "  app:\n    image: other\n"} {
		if _, e := ReplaceImage(s, ref); e == nil {
			t.Fatal("ambiguous/interpolated Compose accepted")
		}
	}
}
func TestAtomicAndInitializerPreserveCredentialsAndModes(t *testing.T) {
	r := t.TempDir()
	p := filepath.Join(r, "docker-compose.yml")
	raw := []byte("APP_SECRET_KEY: CHANGE_ME_APP_SECRET_KEY\nPOSTGRES_PASSWORD: already-private\n")
	os.WriteFile(p, raw, 0644)
	if e := initCompose(r); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), "CHANGE_ME") || !strings.Contains(string(b), "already-private") {
		t.Fatal("credentials lost")
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
	if e := initCompose(r); e == nil {
		t.Fatal("already initialized overwritten")
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(after, b) {
		t.Fatal("changed existing")
	}
	link := filepath.Join(r, "link")
	os.Symlink(p, link)
	if e := Atomic(link, []byte("bad")); e == nil {
		t.Fatal("followed symlink")
	}
}
func TestStickyFailureNeverRunsAnotherEffect(t *testing.T) {
	ctx, c := context.WithTimeout(context.Background(), time.Second)
	defer c()
	o := &owner{ctx: ctx, root: t.TempDir(), err: errors.New("first command uncertain")}
	p := filepath.Join(o.root, "should-not-exist")
	o.call("touch", p)
	o.write(p, map[string]bool{"bad": true})
	if _, e := os.Stat(p); !os.IsNotExist(e) {
		t.Fatal("replayed after failure")
	}
	if o.err.Error() != "first command uncertain" {
		t.Fatal(o.err)
	}
}
func TestDumpIsPrivateCompressedAndHashesFinalBytes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "snapshot.gz")
	digest, e := dumpTo(exec.Command("printf", "owned synthetic backup"), p, true)
	if e != nil || len(digest) != 64 {
		t.Fatal(e)
	}
	f, e := os.Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	gz, e := gzip.NewReader(f)
	if e != nil {
		t.Fatal(e)
	}
	defer gz.Close()
	b, e := io.ReadAll(gz)
	if e != nil || string(b) != "owned synthetic backup" {
		t.Fatal(e, string(b))
	}
	st, _ := f.Stat()
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
	if _, e = dumpTo(exec.Command("false"), p, true); e == nil {
		t.Fatal("backup overwrite")
	}
}
func TestPGForwarderRejectsArbitraryToolAndContainer(t *testing.T) {
	if e := pgTool([]string{"sh"}); e == nil {
		t.Fatal("shell allowed")
	}
	t.Setenv("MEDIATOOL_PG_CONTAINER", "unowned")
	if e := pgTool([]string{"pg_dump"}); e == nil {
		t.Fatal("unowned container")
	}
}
func TestOnlyCanonicalThreeServices(t *testing.T) {
	if !services("redis app postgres\n") || services("app postgres redis worker") || services("app postgres postgres") {
		t.Fatal("service scope")
	}
}

func TestApprovedRollbackAndUncertainSwitchAreNeverReplayed(t *testing.T) {
	for _, failUp := range []bool{false, true} {
		t.Run(fmt.Sprint(failUp), func(t *testing.T) {
			r := t.TempDir()
			p := filepath.Join(r, "docker-compose.yml")
			os.WriteFile(p, []byte("services:\n  app:\n    image: old\n"), 0600)
			id := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
			digest := "sha256:" + strings.Repeat("b", 64)
			proof := strings.Repeat("c", 32)
			calls := []string{}
			o := &owner{ctx: context.Background(), root: r, compose: p, base: []string{"docker", "compose"}, control: []string{"control"}, env: map[string]string{"DATABASE_URL": "postgres://fixture@postgres/media_tool"}}
			o.app.Image = "current-container-image"
			o.app.Config.Image = "dingding229/mediatool:sha-" + strings.Repeat("a", 40)
			o.command = func(a []string) (string, error) {
				s := strings.Join(a, " ")
				calls = append(calls, s)
				switch {
				case strings.HasPrefix(s, "control check"):
					return string(jsonBytes(map[string]any{"id": id, "kind": "rollback", "state": "approved", "payload": map[string]any{"schema": 65, "targetRevision": strings.Repeat("a", 40), "imageDigest": digest, "protected": map[string]string{"media_assets": proof}}})), nil
				case s == "control info":
					return `{"schema":65}`, nil
				case strings.HasPrefix(s, "docker image inspect"):
					return string(jsonBytes([]Image{fixtureImage()})), nil
				case strings.Contains(s, "pg_database_size"):
					return "1024", nil
				case strings.Contains(s, "df -Pk"):
					return "Filesystem 1024-blocks Used Available Capacity Mounted\nowned 9000000 1 8999999 1% /data\n", nil
				case strings.Contains(s, "count(*)FROM jobs"):
					return "0", nil
				case strings.Contains(s, "goose_db_version"):
					return "65", nil
				case strings.Contains(s, "string_agg"):
					return proof, nil
				case strings.Contains(s, " up ") && failUp:
					return "", errors.New("uncertain health check")
				case strings.HasPrefix(s, "control"):
					return `{}`, nil
				default:
					return "", nil
				}
			}
			out := o.execute(id)
			if (o.err != nil) != failUp {
				t.Fatal(o.err, calls)
			}
			statusPath := filepath.Join(r, "private", "ops-execution-"+id, "status.json")
			b, e := os.ReadFile(statusPath)
			if e != nil {
				t.Fatal(e)
			}
			var v map[string]any
			if e = json.Unmarshal(b, &v); e != nil {
				t.Fatal(e)
			}
			if failUp {
				if v["state"] != "uncertain" || v["automaticRetryAllowed"] != false {
					t.Fatal(string(b))
				}
				for _, s := range calls {
					if strings.Contains(s, " start app") || strings.Contains(s, "-success") {
						t.Fatal("unsafe rollback/replay", s)
					}
				}
			} else if out["state"] != "completed" || out["businessPaused"] != true {
				t.Fatal(out)
			}
			before, _ := os.ReadFile(filepath.Join(r, "private", "ops-execution-"+id, "docker-compose.before.yml"))
			if !strings.Contains(string(before), "image: old") {
				t.Fatal("rollback backup lost")
			}
		})
	}
}
