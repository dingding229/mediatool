package onlineupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"media-tool/internal/modules/updatecheck"
	deferutil "media-tool/internal/platform/cleanup"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type runner struct {
	d        *docker
	root     string
	proxyURL string
	dsn      string
	runPG    func(context.Context, string, []string, []string, io.Reader, io.Writer) error
}

func (r *runner) Verify(ctx context.Context, j Job) error {
	v, e := updatecheck.New(nil, slog.Default()).VerifyFresh(ctx)
	if e != nil || v.Metadata != j.Target.Metadata {
		return updatecheck.ErrUntrusted
	}
	if r.d.target != updatecheck.Repository+"@"+v.Digest {
		return ErrUnsafe
	}
	return nil
}
func (r *runner) pg(ctx context.Context, name string, args []string, in io.Reader, out io.Writer) error {
	if name != "pg_dump" && name != "pg_restore" && name != "psql" {
		return ErrUnsafe
	}
	u, e := url.Parse(r.dsn)
	if e != nil || u.Hostname() == "" || u.User == nil {
		return ErrUnsafe
	}
	pw, _ := u.User.Password()
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	c := exec.CommandContext(ctx, name, args...)
	c.Env = []string{"PATH=" + os.Getenv("PATH"), "PGHOST=" + u.Hostname(), "PGPORT=" + port, "PGUSER=" + u.User.Username(), "PGPASSWORD=" + pw, "PGDATABASE=" + strings.TrimPrefix(u.Path, "/"), "PGCONNECT_TIMEOUT=10", "PGSSLMODE=" + u.Query().Get("sslmode")}
	if r.runPG != nil {
		return r.runPG(ctx, name, args, c.Env, in, out)
	}
	c.Stdin = in
	c.Stdout = out
	c.Stderr = io.Discard
	if e = c.Run(); e != nil {
		return errors.New("数据库备份/恢复命令失败，详情仅由运维检查私有备份")
	}
	return nil
}
func (r *runner) compose(ctx context.Context, args ...string) ([]byte, error) {
	// Fixed argv only. Every Docker request is forced through our local allowlist.
	allowed := reflect.DeepEqual(args, []string{"config", "--format", "json"}) || reflect.DeepEqual(args, []string{"up", "-d", "--no-deps", "--no-build", "--pull", "never", "--wait", "--wait-timeout", "180", "app"})
	if !allowed {
		return nil, ErrUnsafe
	}
	a := append([]string{"compose", "--project-directory", r.work(), "-f", filepath.Join(r.work(), "docker-compose.yml")}, args...)
	c := exec.CommandContext(ctx, "docker", a...)
	c.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=/tmp", "DOCKER_HOST=" + r.proxyURL, "DOCKER_API_VERSION=1.47", "COMPOSE_PROJECT_NAME=" + r.d.project()}
	var b bytes.Buffer
	c.Stdout = &b
	c.Stderr = io.Discard
	if e := c.Run(); e != nil {
		return nil, errors.New("app-only Compose 操作失败，未操作其他服务")
	}
	return b.Bytes(), nil
}
func (r *runner) Backup(ctx context.Context, j *Job) error {
	name := "online-update-" + j.CreatedAt.UTC().Format("20060102T150405Z") + "-" + j.ID
	dir := filepath.Join(r.work(), "private", name)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	before, e := os.ReadFile(filepath.Join(r.work(), "docker-compose.yml"))
	if e != nil {
		return e
	}
	if e = atomicFile(filepath.Join(dir, "docker-compose.before.yml"), before); e != nil {
		return e
	}
	snapshot, e := json.Marshal(r.d.self)
	if e != nil {
		return e
	}
	if e = atomicFile(filepath.Join(dir, "container.before.json"), snapshot); e != nil {
		return e
	}
	b, e := r.compose(ctx, "config", "--format", "json")
	if e != nil {
		return e
	}
	if e = atomicFile(filepath.Join(dir, "compose.before.json"), b); e != nil {
		return e
	}
	f, e := os.OpenFile(filepath.Join(dir, "database.dump.partial"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	h := sha256.New()
	e = r.pg(ctx, "pg_dump", []string{"-Fc", "--no-password"}, nil, io.MultiWriter(f, h))
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if e = os.Rename(filepath.Join(dir, "database.dump.partial"), filepath.Join(dir, "database.dump")); e != nil {
		return e
	}
	f, e = os.Open(filepath.Join(dir, "database.dump"))
	if e != nil {
		return e
	}
	defer deferutil.Close(f, "Descriptor teardown only; primary operation and persistence results are checked")
	if e = r.pg(ctx, "pg_restore", []string{"--list"}, f, io.Discard); e != nil {
		return e
	}
	dirFile, e := os.Open(dir)
	if e != nil {
		return e
	}
	syncErr := dirFile.Sync()
	closeErr := dirFile.Close()
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	j.Backup = filepath.Join(labels(r.d.self)["com.docker.compose.project.working_dir"], "private", name)
	j.DumpSHA256 = hex.EncodeToString(h.Sum(nil))
	return nil
}
func (r *runner) work() string { return labels(r.d.self)["com.docker.compose.project.working_dir"] }
func (r *runner) backupDir(j Job) string {
	return filepath.Join(r.work(), "private", filepath.Base(j.Backup))
}
func composeImage(before []byte, old, new string) ([]byte, error) {
	if !validImage(old) || !validImage(new) {
		return nil, ErrUnsafe
	}
	lines := strings.Split(string(before), "\n")
	inApp := false
	count := 0
	appIndent := -1
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if trim == "app:" {
			if inApp || appIndent >= 0 {
				return nil, ErrUnsafe
			}
			inApp = true
			appIndent = indent
			continue
		}
		if inApp && trim != "" && !strings.HasPrefix(trim, "#") && indent <= appIndent {
			inApp = false
		}
		if inApp && strings.HasPrefix(trim, "image:") {
			value := strings.TrimSpace(strings.TrimPrefix(trim, "image:"))
			if value != old && value != "\""+old+"\"" && value != "'"+old+"'" {
				return nil, ErrUnsafe
			}
			lines[i] = strings.Replace(line, old, new, 1)
			count++
		}
	}
	if count != 1 {
		return nil, ErrUnsafe
	}
	return []byte(strings.Join(lines, "\n")), nil
}
func (r *runner) Apply(ctx context.Context, j Job) (result error) {
	changed := false
	defer func() {
		if result != nil && !changed {
			result = errors.Join(errNotApplied, result)
		}
	}()
	if e := r.Verify(ctx, j); e != nil {
		return e
	} // immediately before touching Compose
	before, e := os.ReadFile(filepath.Join(r.backupDir(j), "docker-compose.before.yml"))
	if e != nil {
		return e
	}
	live, e := os.ReadFile(filepath.Join(r.work(), "docker-compose.yml"))
	if e != nil || !bytes.Equal(live, before) {
		return ErrUnsafe
	}
	next, e := composeImage(before, j.FromImage, r.d.target)
	if e != nil {
		return e
	}
	changed = true
	if e = atomicFile(filepath.Join(r.work(), "docker-compose.yml"), next); e != nil {
		return e
	}
	after, e := r.compose(ctx, "config", "--format", "json")
	if e != nil {
		return e
	}
	original, e := os.ReadFile(filepath.Join(r.backupDir(j), "compose.before.json"))
	if e != nil {
		return e
	}
	var a, b map[string]any
	if json.Unmarshal(after, &a) != nil || json.Unmarshal(original, &b) != nil {
		return ErrUnsafe
	}
	services, ok := b["services"].(map[string]any)
	if !ok {
		return ErrUnsafe
	}
	app, ok := services["app"].(map[string]any)
	if !ok {
		return ErrUnsafe
	}
	app["image"] = r.d.target
	if !reflect.DeepEqual(a, b) {
		return ErrUnsafe
	}
	_, e = r.compose(ctx, "up", "-d", "--no-deps", "--no-build", "--pull", "never", "--wait", "--wait-timeout", "180", "app")
	return e
}
func (r *runner) Health(ctx context.Context, j Job) error {
	c, e := r.d.inspect(ctx, r.d.appName())
	if e != nil {
		return e
	}
	if !c.State.Running || c.State.Health.Status != "healthy" || fmtString(c.Config["Image"]) != r.d.target || labels(c)["com.docker.compose.service"] != "app" || labels(c)["com.docker.compose.project"] != r.d.project() {
		return ErrUnsafe
	}
	if labels(c)["org.opencontainers.image.revision"] != j.Target.Commit || labels(c)["org.opencontainers.image.version"] != j.Target.Version {
		return ErrUnsafe
	}
	addr := env(c, "APP_ADDR")
	_, port, e := net.SplitHostPort(addr)
	if e != nil {
		return ErrUnsafe
	}
	var network struct{ IPAddress string }
	if json.Unmarshal(c.NetworkSettings.Networks[r.d.project()+"_default"], &network) != nil || net.ParseIP(network.IPAddress) == nil {
		return ErrUnsafe
	}
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrUnsafe }}
	req, e := http.NewRequestWithContext(ctx, "GET", "http://"+net.JoinHostPort(network.IPAddress, port)+"/readyz", nil)
	if e != nil {
		return e
	}
	resp, e := client.Do(req)
	if e != nil {
		return e
	}
	defer deferutil.Close(resp.Body, "Descriptor teardown only; primary operation and persistence results are checked")
	if resp.StatusCode != 200 {
		return ErrUnsafe
	}
	db, e := pgx.Connect(ctx, r.dsn)
	if e != nil {
		return e
	}
	defer deferutil.CloseContext(ctx, db, "Query already checked; release connection")
	var schema int64
	if e = db.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&schema); e != nil {
		return e
	}
	if schema != j.Target.Schema {
		return ErrUnsafe
	}
	return nil
}
func (r *runner) Rollback(ctx context.Context, j Job) error {
	dir := r.backupDir(j)
	dump, e := os.Open(filepath.Join(dir, "database.dump"))
	if e != nil {
		return e
	}
	defer deferutil.Close(dump, "Descriptor teardown only; primary operation and persistence results are checked")
	h := sha256.New()
	if _, e = io.Copy(h, dump); e != nil || j.DumpSHA256 == "" || hex.EncodeToString(h.Sum(nil)) != j.DumpSHA256 {
		return ErrUnsafe
	}
	if _, e = dump.Seek(0, 0); e != nil {
		return e
	}
	before, e := os.ReadFile(filepath.Join(dir, "docker-compose.before.yml"))
	if e != nil {
		return e
	}
	live, readErr := os.ReadFile(filepath.Join(r.work(), "docker-compose.yml"))
	expected, expectedErr := composeImage(before, j.FromImage, updatecheck.Repository+"@"+j.Target.Digest)
	if readErr != nil || expectedErr != nil || (!bytes.Equal(live, before) && !bytes.Equal(live, expected)) {
		return ErrUnsafe
	}
	// Unknown Docker state is not "app stopped". Never restore a live database
	// after a lost inspect response, nor overwrite a concurrent operator image.
	c, inspectErr := r.d.inspect(ctx, r.d.appName())
	if inspectErr != nil && !notFound(inspectErr) {
		return inspectErr
	}
	if inspectErr == nil {
		image := fmtString(c.Config["Image"])
		if image != j.FromImage && image != updatecheck.Repository+"@"+j.Target.Digest {
			return ErrUnsafe
		}
		if c.State.Running {
			if _, e = r.d.request(ctx, "POST", "/containers/"+r.d.appName()+"/stop?t=30", nil); e != nil {
				return e
			}
		}
	}
	if e = atomicFile(filepath.Join(r.work(), "docker-compose.yml"), before); e != nil {
		return e
	}

	if e = r.restoreDatabase(ctx, dir, j); e != nil {
		return e
	}

	// The old immutable image is retained. Creation checks now require its digest.
	r.d.target = j.FromImage
	_, e = r.compose(ctx, "up", "-d", "--no-deps", "--no-build", "--pull", "never", "--wait", "--wait-timeout", "180", "app")
	if e != nil {
		return e
	}
	c, e = r.d.inspect(ctx, r.d.appName())
	if e != nil || c.State.Health.Status != "healthy" || fmtString(c.Config["Image"]) != j.FromImage {
		return ErrUnsafe
	}
	return nil
}

func (r *runner) restoreDatabase(ctx context.Context, dir string, j Job) error {
	dump, e := os.Open(filepath.Join(dir, "database.dump"))
	if e != nil {
		return e
	}
	defer deferutil.Close(dump, "Descriptor teardown only; primary operation and persistence results are checked")
	h := sha256.New()
	if _, e = io.Copy(h, dump); e != nil || hex.EncodeToString(h.Sum(nil)) != j.DumpSHA256 {
		return ErrUnsafe
	}
	if _, e = dump.Seek(0, 0); e != nil {
		return e
	}
	// Remove post-backup DDL as well as data, atomically. Exclude only the fixed
	// public-schema TOC entry, then recreate it ourselves in psql's transaction.
	var toc bytes.Buffer
	if e = r.pg(ctx, "pg_restore", []string{"--list"}, dump, &toc); e != nil {
		return e
	}
	lines := strings.Split(toc.String(), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.Contains(line, " SCHEMA - public ") {
			continue
		}
		if strings.HasPrefix(line, ";") || strings.TrimSpace(line) == "" || publicTOC(line) {
			kept = append(kept, line)
		}
	}
	list := filepath.Join(dir, "restore.list")
	if e = atomicFile(list, []byte(strings.Join(kept, "\n"))); e != nil {
		return e
	}
	if _, e = dump.Seek(0, 0); e != nil {
		return e
	}
	sql, e := os.CreateTemp("/tmp", "restore-*.sql")
	if e != nil {
		return e
	}
	defer func() { _ = os.Remove(sql.Name()) }()
	defer deferutil.Close(sql, "Descriptor teardown only; primary operation and persistence results are checked")
	if e = r.pg(ctx, "pg_restore", []string{"--file=-", "--no-owner", "--no-privileges", "--use-list=" + list}, dump, sql); e != nil {
		return e
	}
	if _, e = sql.Seek(0, 0); e != nil {
		return e
	}
	input := io.MultiReader(strings.NewReader("DROP SCHEMA public CASCADE; CREATE SCHEMA public;\n"), sql)
	if e = r.pg(ctx, "psql", []string{"-X", "--set=ON_ERROR_STOP=1", "--single-transaction", "--file=-", "--no-password"}, input, io.Discard); e != nil {
		return e
	}

	return nil
}

// RunUpdater executes only a persisted, pre-admitted task from the mounted app data.
func RunUpdater(ctx context.Context, id string) error {
	if !idPattern.MatchString(id) {
		return ErrUnsafe
	}
	unlock, e := lock(Root, "execution")
	if e != nil {
		return e
	}
	defer unlock()
	j, e := load(Root, id)
	if e != nil {
		return e
	}
	if j.Done() {
		return nil
	}
	p, e := readPlan(Root, id)
	if e != nil {
		return e
	}
	d := newDocker()
	d.self = p.Self
	d.target = p.TargetImage
	d.task = id
	original, identityErr := originalContainer(ctx, d, p, j)
	if identityErr != nil {
		if errors.Is(identityErr, ErrUnsafe) {
			j.Failure = "execution_identity_unavailable"
			phase := "failed"
			if j.Phase == "applying" || j.Phase == "checking" || j.Phase == "rolling_back" {
				phase = "manual_recovery"
			}
			if persistErr := advance(Root, &j, phase); persistErr != nil {
				return persistErr
			}
		}
		return identityErr
	}
	d.self = original
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return e
	}
	srv := &http.Server{Handler: http.HandlerFunc(d.proxy), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(listener) }()
	defer deferutil.Close(srv, "Descriptor teardown only; primary operation and persistence results are checked")
	r := &runner{d: d, root: Root, proxyURL: "tcp://" + listener.Addr().String(), dsn: env(p.Self, "DATABASE_URL")}
	e = execute(ctx, Root, &j, r)
	auditCtx, auditDone := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer auditDone()
	db, dbErr := pgxpool.New(auditCtx, r.dsn)
	if dbErr == nil {
		defer db.Close()
		if a := audit(auditCtx, db, j.Actor, "online_update."+j.Phase, j); a != nil && e == nil {
			e = a
		}
	}
	return e
}

func publicTOC(line string) bool {
	f := strings.Fields(line)
	if len(f) < 5 {
		return false
	}
	for _, kind := range []string{"TABLE DATA", "TABLE ATTACH", "TABLE", "SEQUENCE SET", "SEQUENCE OWNED BY", "SEQUENCE", "INDEX ATTACH", "INDEX", "FK CONSTRAINT", "CONSTRAINT", "TRIGGER", "RULE", "FUNCTION", "PROCEDURE", "TYPE", "DOMAIN", "DEFAULT", "MATERIALIZED VIEW DATA", "MATERIALIZED VIEW", "VIEW", "COMMENT", "POLICY", "ROW SECURITY", "ACL"} {
		k := strings.Fields(kind)
		if len(f) > 3+len(k) && strings.Join(f[3:3+len(k)], " ") == kind {
			return f[3+len(k)] == "public"
		}
	}
	return f[3] == "EXTENSION" || (f[3] == "COMMENT" && len(f) > 5 && f[4] == "-" && f[5] == "EXTENSION")
}
