package operator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"media-tool/internal/modules/maintenance"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type mount struct{ Source, Destination string }
type container struct {
	Id, Image, Name string
	Config          struct {
		Env         []string
		Image, User string
	}
	Mounts          []mount
	NetworkSettings struct {
		Networks map[string]struct {
			Aliases   []string
			IPAddress string
		}
		Ports map[string][]struct{ HostIp, HostPort string }
	}
}
type owner struct {
	ctx           context.Context
	root, compose string
	base, control []string
	app, pg       container
	env           map[string]string
	err           error
	command       func([]string) (string, error)
}

// Sticky errors stop every subsequent command/write. Failure receipts are written
// separately; an uncertain image/database switch is never automatically undone.
func (o *owner) fail(e error) {
	if o.err == nil {
		o.err = e
	}
}
func (o *owner) call(a ...string) string {
	if o.err != nil {
		return ""
	}
	if o.command != nil {
		v, e := o.command(a)
		o.fail(e)
		return v
	}
	cmd := exec.CommandContext(o.ctx, a[0], a[1:]...)
	cmd.Dir = o.root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	b, e := cmd.Output()
	if e != nil {
		o.fail(fmt.Errorf("fixed maintenance command failed: %s", filepath.Base(a[0])))
	}
	return string(b)
}
func (o *owner) docker(a ...string) string {
	return o.call(append(append([]string{}, o.base...), a...)...)
}
func (o *owner) cli(a ...string) json.RawMessage {
	b := o.call(append(append([]string{}, o.control...), a...)...)
	if o.err == nil && !json.Valid([]byte(b)) {
		o.fail(errors.New("invalid app control receipt"))
	}
	return json.RawMessage(b)
}
func (o *owner) write(p string, v any) {
	if o.err == nil {
		o.fail(Atomic(p, jsonBytes(v)))
	}
}
func (o *owner) sql(q, db string) string {
	if db == "" {
		u, e := url.Parse(o.env["DATABASE_URL"])
		if e != nil {
			o.fail(errors.New("invalid private database configuration"))
			return ""
		}
		db = strings.TrimPrefix(u.Path, "/")
	}
	return strings.TrimSpace(o.docker("exec", "-T", "postgres", "sh", "-c", `exec psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$1" -At -c "$2"`, "sh", db, q))
}
func (o *owner) inspect(ref string) Release {
	if !hub.MatchString(ref) {
		o.fail(errors.New("immutable Hub reference required"))
		return Release{}
	}
	o.call("docker", "pull", "--platform", "linux/amd64", ref)
	var a []Image
	b := o.call("docker", "image", "inspect", ref)
	if o.err != nil {
		return Release{}
	}
	if e := json.Unmarshal([]byte(b), &a); e != nil || len(a) != 1 {
		o.fail(errors.New("invalid image metadata"))
		return Release{}
	}
	r, e := CheckImage(ref, a[0])
	o.fail(e)
	return r
}
func (o *owner) register(ref string) Release {
	r := o.inspect(ref)
	o.cli("register-release", "-revision", r.Revision, "-schema", strconv.FormatInt(r.Schema, 10), "-digest", r.Digest)
	r.Registered = o.err == nil
	return r
}
func newOwner(ctx context.Context, root string) *owner {
	o := &owner{ctx: ctx, root: root, compose: filepath.Join(root, "docker-compose.yml"), env: map[string]string{}}
	st, e := os.Lstat(o.compose)
	if e != nil || !st.Mode().IsRegular() {
		o.fail(errors.New("private regular root Compose required"))
		return o
	}
	o.base = []string{"docker", "compose", "-f", o.compose}
	if !services(o.docker("config", "--services")) {
		o.fail(errors.New("only canonical app/postgres/redis are allowed"))
		return o
	}
	cid := strings.TrimSpace(o.docker("ps", "-q", "app"))
	pgid := strings.TrimSpace(o.docker("ps", "-q", "postgres"))
	if cid == "" || pgid == "" {
		o.fail(errors.New("existing app and postgres required"))
		return o
	}
	for _, v := range []struct {
		id  string
		dst *container
	}{{cid, &o.app}, {pgid, &o.pg}} {
		var list []container
		b := o.call("docker", "inspect", v.id)
		if o.err != nil {
			return o
		}
		if e = json.Unmarshal([]byte(b), &list); e != nil || len(list) != 1 {
			o.fail(errors.New("invalid owned container metadata"))
			return o
		}
		*v.dst = list[0]
	}
	for _, s := range o.app.Config.Env {
		k, v, ok := strings.Cut(s, "=")
		if ok {
			o.env[k] = v
		}
	}
	u, e := url.Parse(o.env["DATABASE_URL"])
	if e != nil || u.Hostname() == "" {
		o.fail(errors.New("owned database required"))
		return o
	}
	matched := false
	for n, p := range o.pg.NetworkSettings.Networks {
		if _, ok := o.app.NetworkSettings.Networks[n]; ok {
			for _, h := range append(p.Aliases, p.IPAddress, strings.TrimPrefix(o.pg.Name, "/")) {
				matched = matched || h == u.Hostname()
			}
		}
	}
	if !matched || (u.Port() != "" && u.Port() != "5432") {
		o.fail(errors.New("database is not the same-project PostgreSQL container"))
	}
	o.control = append(append([]string{}, o.base...), "exec", "-T", "app", "/app/media-tool", "ops")
	return o
}

type request struct {
	ID, Kind, State string
	Payload         struct {
		Schema                      int64
		ImageDigest, TargetRevision string
		Protected                   map[string]string
	}
}

func (o *owner) execute(id string) (receipt map[string]any) {
	if !uuid.MatchString(id) {
		o.fail(errors.New("invalid maintenance request"))
		return nil
	}
	var req request
	b := o.cli("check", "-id", id)
	if o.err != nil {
		return nil
	}
	if e := json.Unmarshal(b, &req); e != nil || req.State != "approved" {
		o.fail(errors.New("only approved unexpired request may run"))
		return nil
	}
	if req.Kind == "restore" {
		// Runs inside the existing app. No copied executable, host PG tools,
		// app stop, database rename or image/Compose change for data recovery.
		raw := o.cli("restore-request", "-id", id)
		var result map[string]any
		if o.err == nil {
			o.fail(json.Unmarshal(raw, &result))
		}
		return result
	}
	var info struct{ Schema int64 }
	b = o.cli("info")
	if o.err == nil {
		o.fail(json.Unmarshal(b, &info))
	}
	var current []Image
	b = []byte(o.call("docker", "image", "inspect", o.app.Image))
	if o.err == nil {
		o.fail(json.Unmarshal(b, &current))
	}
	if len(current) != 1 {
		o.fail(errors.New("current verified image required"))
		return nil
	}
	release, e := CheckImage(o.app.Config.Image, current[0])
	o.fail(e)
	previous := release
	if req.Kind == "rollback" {
		release = o.inspect("dingding229/mediatool@" + req.Payload.ImageDigest)
		if release.Digest != req.Payload.ImageDigest || release.Revision != req.Payload.TargetRevision {
			o.fail(errors.New("target differs from approval"))
		}
	} else if req.Kind != "restore" {
		o.fail(errors.New("unsupported maintenance request"))
	}
	if len(req.Payload.Protected) == 0 {
		o.fail(errors.New("missing protected-data proof"))
	}
	if release.Schema != info.Schema || release.Schema != req.Payload.Schema {
		o.fail(errors.New("schema differs; no automatic downgrade"))
	}
	if o.err != nil {
		return nil
	}
	o.cli("register-release", "-revision", previous.Revision, "-schema", strconv.FormatInt(previous.Schema, 10), "-digest", previous.Digest)
	dir := filepath.Join(o.root, "private", "ops-execution-"+id)
	o.fail(os.MkdirAll(filepath.Dir(dir), 0700))
	if o.err == nil {
		o.fail(os.Mkdir(dir, 0700))
	}
	old, e := os.ReadFile(o.compose)
	o.fail(e)
	if o.err == nil {
		o.fail(Atomic(filepath.Join(dir, "docker-compose.before.yml"), old))
	}
	var offline []string
	var env []string
	if req.Kind == "restore" {
		bin := filepath.Join(dir, "ops-control")
		o.call("docker", "cp", o.app.Id+":/app/media-tool", bin)
		if o.err == nil {
			o.fail(os.Chmod(bin, 0700))
		}
		offline = []string{bin, "ops"}
		env = o.hostEnvironment(dir)
	}
	size, e := strconv.ParseInt(o.sql("SELECT pg_database_size(current_database())", ""), 10, 64)
	o.fail(e)
	space := strings.Fields(strings.TrimSpace(o.docker("exec", "-T", "postgres", "sh", "-c", `df -Pk "$PGDATA"`)))
	if len(space) < 4 {
		o.fail(errors.New("storage headroom unavailable"))
	} else {
		free, e := strconv.ParseInt(space[len(space)-3], 10, 64)
		o.fail(e)
		if free*1024 < size*2+512*1024*1024 {
			o.fail(errors.New("insufficient PostgreSQL storage headroom"))
		}
	}
	if o.err != nil {
		return nil
	}
	o.write(filepath.Join(dir, "preflight-backup.json"), o.cli("backup"))
	o.cli("claim", "-id", id)
	o.write(filepath.Join(dir, "request.json"), req)
	o.write(filepath.Join(dir, "status.json"), map[string]string{"state": "claimed", "id": id})
	if o.err != nil {
		return nil
	}
	stopped, switched := false, false
	defer func() {
		if o.err == nil {
			_ = os.RemoveAll(filepath.Join(dir, "bin"))
			_ = os.Remove(filepath.Join(dir, "ops-control"))
			return
		}
		failure := o.err
		state := "failed"
		if switched {
			state = "uncertain"
		}
		if e := Atomic(filepath.Join(dir, "status.json"), jsonBytes(map[string]any{"state": state, "id": id, "appStopped": stopped, "composeSwitched": switched, "automaticRetryAllowed": false, "error": failure.Error()})); e != nil {
			o.err = errors.Join(failure, e)
		}
		if !switched {
			o.err = nil
			if stopped {
				o.docker("start", "app")
			}
			o.cli("finish", "-id", id, "-message", "宿主执行失败；保持维护暂停，核对私有回执后再处理")
			o.err = errors.Join(failure, o.err)
		}
	}()
	deadline := time.Now().Add(120 * time.Second)
	for o.err == nil {
		n, e := strconv.Atoi(o.sql("SELECT count(*)FROM jobs WHERE state='running' AND kind NOT LIKE 'ops.%'", ""))
		o.fail(e)
		if n == 0 || o.err != nil {
			break
		}
		if time.Now().After(deadline) {
			o.fail(errors.New("business tasks did not drain"))
			break
		}
		select {
		case <-o.ctx.Done():
			o.fail(o.ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	o.docker("stop", "-t", "120", "app")
	stopped = o.err == nil
	live, e := strconv.ParseInt(o.sql("SELECT coalesce(max(version_id),0)FROM(SELECT DISTINCT ON(version_id)version_id,is_applied FROM goose_db_version ORDER BY version_id,id DESC)v WHERE is_applied", ""), 10, 64)
	o.fail(e)
	if live != release.Schema {
		o.fail(errors.New("live schema changed after approval"))
	}
	for table, expected := range req.Payload.Protected {
		if !regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`).MatchString(table) || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(expected) {
			o.fail(errors.New("invalid protection proof"))
			break
		}
		expr := "to_jsonb(t)"
		if table == "external_api_keys" {
			expr = "(to_jsonb(t)-'last_used_at')"
		}
		actual := o.sql("SELECT coalesce(md5(string_agg("+expr+"::text,E'\\n' ORDER BY "+expr+"::text)),md5(''))FROM "+table+" t", "")
		if actual != expected {
			o.fail(errors.New("protected data changed while draining"))
		}
	}
	if req.Kind == "restore" && o.err == nil {
		cmd := exec.CommandContext(o.ctx, offline[0], append(offline[1:], "prepare", "-id", id)...)
		cmd.Dir = o.root
		cmd.Env = env
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		raw, e := cmd.Output()
		if e != nil {
			_ = Atomic(filepath.Join(dir, "offline-error.log"), stderr.Bytes())
			o.fail(errors.New("offline preparation failed; private receipt retained"))
			return nil
		}
		var v struct{ CandidateDatabase string }
		o.fail(json.Unmarshal(raw, &v))
		u, e := url.Parse(o.env["DATABASE_URL"])
		o.fail(e)
		if u == nil || !candidate.MatchString(v.CandidateDatabase) {
			o.fail(errors.New("invalid verified candidate"))
			return nil
		}
		name := strings.TrimPrefix(u.Path, "/")
		if name == "" || name == v.CandidateDatabase || name == "postgres" || strings.HasPrefix(name, "template") {
			o.fail(errors.New("system/same database cannot be restored"))
			return nil
		}
		before := "media_tool_before_" + strings.ReplaceAll(id, "-", "")[:12]
		o.write(filepath.Join(dir, "candidate.json"), map[string]string{"candidateDatabase": v.CandidateDatabase, "oldDatabaseRetained": before, "liveDatabase": name})
		literal := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
		o.sql("SELECT pg_terminate_backend(pid)FROM pg_stat_activity WHERE backend_type='client backend'AND datname IN("+literal(name)+","+literal(v.CandidateDatabase)+")AND pid<>pg_backend_pid()", "postgres")
		if o.err == nil {
			switched = true
			plan, err := databaseSwapSQL(name, v.CandidateDatabase, before)
			o.fail(err)
			if err == nil {
				o.sql(plan, "postgres")
			}
		}
	}
	next, e := ReplaceImage(string(old), "dingding229/mediatool@"+release.Digest)
	o.fail(e)
	if o.err == nil {
		switched = true
		o.fail(Atomic(o.compose, []byte(next)))
	}
	o.docker("up", "-d", "--no-deps", "--wait", "--wait-timeout", "180", "app")
	o.cli("finish", "-id", id, "-success", "-message", "宿主执行完成；原库和回滚备份保留。业务保持暂停，须核对后显式解除维护。")
	receipt = map[string]any{"state": "completed", "id": id, "targetRevision": release.Revision, "schema": release.Schema, "businessPaused": true, "oldDatabasePreserved": true, "rollbackDirectory": dir, "flow": maintenance.RequestFlow("completed", false, false)}
	o.write(filepath.Join(dir, "status.json"), receipt)
	return receipt
}
func (o *owner) hostEnvironment(dir string) []string {
	uid := regexp.MustCompile(`^([1-9][0-9]*):([0-9]+)$`).FindStringSubmatch(o.app.Config.User)
	ports := o.pg.NetworkSettings.Ports["5432/tcp"]
	if uid == nil || len(ports) != 1 || (ports[0].HostIp != "127.0.0.1" && ports[0].HostIp != "::1") {
		o.fail(errors.New("restore requires numeric non-root app and loopback postgres port"))
		return nil
	}
	env := map[string]string{}
	for k, v := range o.env {
		env[k] = v
	}
	env["OPS_RESTORE_UID"] = uid[1]
	env["OPS_RESTORE_GID"] = uid[2]
	u, e := url.Parse(env["DATABASE_URL"])
	if e != nil {
		o.fail(e)
		return nil
	}
	host := ports[0].HostIp
	if host == "::1" {
		host = "[::1]"
	}
	u.Host = host + ":" + ports[0].HostPort
	env["DATABASE_URL"] = u.String()
	for k, def := range map[string]string{"STRM_OUTPUT_DIR": "/data/strm", "OPS_DATA_DIR": "/data/maintenance"} {
		p := env[k]
		if p == "" {
			p = def
		}
		var best mount
		for _, m := range o.app.Mounts {
			if (p == m.Destination || strings.HasPrefix(p, m.Destination+"/")) && len(m.Destination) > len(best.Destination) {
				best = m
			}
		}
		if best.Destination == "" {
			o.fail(errors.New("owned data mount unavailable"))
			return nil
		}
		env[k] = filepath.Join(best.Source, strings.TrimPrefix(p, best.Destination))
	}
	tools := filepath.Join(dir, "bin")
	if o.err == nil {
		o.fail(os.Mkdir(tools, 0700))
	}
	self, e := os.Executable()
	o.fail(e)
	docker, e := exec.LookPath("docker")
	o.fail(e)
	env["MEDIATOOL_DOCKER"] = docker
	env["MEDIATOOL_PG_CONTAINER"] = o.pg.Id
	for _, tool := range []string{"pg_dump", "pg_restore"} {
		if o.err == nil {
			o.fail(Atomic(filepath.Join(tools, tool), []byte("#!/bin/sh\nexec "+shellQuote(self)+" pg-tool "+tool+" \"$@\"\n")))
			o.fail(os.Chmod(filepath.Join(tools, tool), 0700))
		}
	}
	env["PATH"] = tools + ":/usr/bin:/bin"
	out := []string{}
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

func databaseSwapSQL(current, next, previous string) (string, error) {
	if current == "" || len(current) > 63 || strings.ContainsAny(current, "\x00\r\n") || current == "postgres" || strings.HasPrefix(current, "template") || current == next || current == previous || !candidate.MatchString(next) || !strings.HasPrefix(previous, "media_tool_before_") || !candidate.MatchString(strings.Replace(previous, "media_tool_before_", "media_tool_restore_", 1)) {
		return "", errors.New("unsafe database swap plan")
	}
	quote := func(v string) string { return `"` + strings.ReplaceAll(v, `"`, `""`) + `"` }
	return "BEGIN;ALTER DATABASE " + quote(current) + " RENAME TO " + quote(previous) + ";ALTER DATABASE " + quote(next) + " RENAME TO " + quote(current) + ";COMMIT;", nil
}
