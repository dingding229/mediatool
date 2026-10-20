// Package operator is owner-only host orchestration, never an HTTP command API.
// Application operations remain inside the canonical app container.
package operator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var sha = regexp.MustCompile(`^[a-f0-9]{40}$`)
var uuid = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var hub = regexp.MustCompile(`^(?:docker.io/)?dingding229/mediatool(?::sha-([a-f0-9]{40})|@(sha256:[a-f0-9]{64}))$`)
var candidate = regexp.MustCompile(`^media_tool_restore_[a-f0-9]{12}$`)

type Image struct {
	Os, Architecture string
	RepoDigests      []string
	Config           struct{ Labels map[string]string }
}
type Release struct {
	Revision   string `json:"revision"`
	Schema     int64  `json:"schema"`
	Digest     string `json:"digest"`
	Registered bool   `json:"registered"`
}

func CheckImage(ref string, i Image) (Release, error) {
	m := hub.FindStringSubmatch(ref)
	if m == nil || i.Os != "linux" || i.Architecture != "amd64" {
		return Release{}, errors.New("only immutable Docker Hub AMD64 images are accepted")
	}
	r := Release{Revision: i.Config.Labels["org.opencontainers.image.revision"]}
	s, e := strconv.ParseInt(i.Config.Labels["org.opencontainers.image.schema"], 10, 64)
	if e != nil || s < 1 || !sha.MatchString(r.Revision) || (m[1] != "" && m[1] != r.Revision) {
		return r, errors.New("image revision/schema mismatch")
	}
	r.Schema = s
	for _, d := range i.RepoDigests {
		v := hub.FindStringSubmatch(d)
		if v != nil && v[2] != "" && (m[2] == "" || v[2] == m[2]) {
			r.Digest = v[2]
			return r, nil
		}
	}
	return r, errors.New("Docker Hub digest unavailable or changed")
}
func ReplaceImage(text, ref string) (string, error) {
	if !hub.MatchString(ref) || strings.Contains(text, "${") {
		return "", errors.New("private Compose requires direct values and immutable Hub image")
	}
	lines := strings.Split(text, "\n")
	active := false
	n := 0
	for i, l := range lines {
		if strings.HasPrefix(l, "  ") && !strings.HasPrefix(l, "   ") && strings.HasSuffix(strings.TrimSpace(l), ":") {
			active = l == "  app:"
		}
		if active && strings.HasPrefix(l, "    image:") {
			lines[i] = "    image: " + ref
			n++
		}
	}
	if n != 1 {
		return "", errors.New("expected exactly one canonical app image")
	}
	return strings.Join(lines, "\n"), nil
}
func Atomic(path string, b []byte) (err error) {
	if st, e := os.Lstat(path); e == nil && !st.Mode().IsRegular() {
		return errors.New("regular private file required")
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".mediatool-ops-")
	if e != nil {
		return e
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if e = f.Chmod(0600); e != nil {
		_ = f.Close()
		return e
	}
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(f.Name(), path)
}
func Random(n int) (string, error) {
	b := make([]byte, n)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func initCompose(root string) error {
	p := filepath.Join(root, "docker-compose.yml")
	st, e := os.Lstat(p)
	if e != nil || !st.Mode().IsRegular() {
		return errors.New("regular root Compose required")
	}
	b, e := os.ReadFile(p)
	if e != nil {
		return e
	}
	s := string(b)
	changed := false
	for _, k := range []string{"APP_SECRET_KEY", "SESSION_SECRET_KEY", "STRM_API_KEY", "POSTGRES_PASSWORD", "REDIS_PASSWORD"} {
		if strings.Contains(s, "CHANGE_ME_"+k) {
			v, e := Random(32)
			if e != nil {
				return e
			}
			s = strings.ReplaceAll(s, "CHANGE_ME_"+k, v)
			changed = true
		}
	}
	if !changed {
		return errors.New("already initialized; credentials unchanged")
	}
	return Atomic(p, []byte(s))
}
func Run(args []string) error {
	if len(args) > 0 && args[0] == "pg-tool" {
		return pgTool(args[1:])
	}
	f := flag.NewFlagSet("media-toolctl", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	root := f.String("root", "", "existing private deployment root")
	if e := f.Parse(args); e != nil {
		return e
	}
	rest := f.Args()
	if len(rest) < 1 {
		return errors.New("usage: media-toolctl --root DIR <init-compose|image-check|register|execute|update|backup-verify> [argument]")
	}
	if *root == "" {
		return errors.New("explicit deployment root required")
	}
	r, e := filepath.Abs(*root)
	if e != nil {
		return e
	}
	st, e := os.Stat(r)
	if e != nil || !st.IsDir() {
		return errors.New("existing deployment root required")
	}
	lock, e := os.OpenFile(filepath.Join(r, ".update.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer func() { _ = lock.Close() }()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return errors.New("another owned update is running")
	}
	if rest[0] == "init-compose" {
		if len(rest) != 1 {
			return errors.New("unexpected argument")
		}
		return initCompose(r)
	}
	if len(rest) != 2 {
		return errors.New("exactly one image or approved request required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	o := newOwner(ctx, r)
	if o.err != nil {
		return o.err
	}
	var out any
	switch rest[0] {
	case "image-check":
		out = o.inspect(rest[1])
	case "register":
		out = o.register(rest[1])
	case "execute":
		out = o.execute(rest[1])
	case "update":
		out = o.update(rest[1])
	default:
		return errors.New("unknown operator command")
	}
	if o.err != nil {
		return o.err
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}

// Fixed wrappers forward installed PG tools to the same-project postgres only.
func pgTool(args []string) error {
	if len(args) < 1 || !slices.Contains([]string{"pg_dump", "pg_restore"}, args[0]) {
		return errors.New("fixed PostgreSQL tool required")
	}
	id := os.Getenv("MEDIATOOL_PG_CONTAINER")
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(id) {
		return errors.New("owned postgres container required")
	}
	a := []string{"exec", "-i", "-e", "PGHOST=127.0.0.1", "-e", "PGPORT=5432"}
	for _, k := range []string{"PGDATABASE", "PGUSER", "PGPASSWORD", "PGSSLMODE", "PGSSLROOTCERT", "PGSSLCERT", "PGSSLKEY"} {
		if _, ok := os.LookupEnv(k); ok {
			a = append(a, "-e", k)
		}
	}
	a = append(a, id, args[0])
	a = append(a, args[1:]...)
	cmd := exec.Command(os.Getenv("MEDIATOOL_DOCKER"), a...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
func jsonBytes(v any) []byte {
	b, e := json.Marshal(v)
	if e != nil {
		panic("internal receipt is not JSON")
	}
	return b
}
func services(raw string) bool {
	v := strings.Fields(raw)
	slices.Sort(v)
	return slices.Equal(v, []string{"app", "postgres", "redis"})
}
func shellQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'" }
