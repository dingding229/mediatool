package onlineupdate

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/updatecheck"
	"media-tool/internal/platform/buildinfo"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	DB      *pgxpool.Pool
	Checker *updatecheck.Service
	Root    string
}
type Status struct {
	Available      bool   `json:"available"`
	Reason         string `json:"reason"`
	SocketMounted  bool   `json:"socketMounted"`
	PolicyVerified bool   `json:"policyVerified"`
	Job            *Job   `json:"job"`
}
type plan struct {
	Self        Container
	TargetImage string
	JobID       string
}

func New(db *pgxpool.Pool, checker *updatecheck.Service) *Service { return &Service{db, checker, Root} }
func audit(ctx context.Context, db *pgxpool.Pool, actor, action string, details any) error {
	if db == nil {
		return ErrUnsafe
	}
	b, e := json.Marshal(details)
	if e != nil {
		return e
	}
	_, e = db.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES($1,$2,$3::jsonb)`, actor, action, b)
	return e
}
func discover(ctx context.Context) (*docker, error) {
	d := newDocker()
	host, e := os.Hostname()
	if e != nil || !containerID.MatchString(host) {
		return nil, ErrUnsafe
	}
	c, e := d.inspect(ctx, host)
	if e != nil {
		return nil, e
	}
	l := labels(c)
	if l["com.docker.compose.project"] == "" || l["com.docker.compose.service"] != "app" || !validImage(fmtString(c.Config["Image"])) || c.Config["User"] != "65532:65532" {
		return nil, ErrUnsafe
	}
	w := l["com.docker.compose.project.working_dir"]
	if w == "/" || !filepath.IsAbs(w) || filepath.Clean(w) != w || l["com.docker.compose.project.config_files"] != filepath.Join(w, "docker-compose.yml") {
		return nil, ErrUnsafe
	}
	found := false
	for _, m := range c.Mounts {
		if m.Destination == "/data" && m.RW && (m.Type == "bind" || m.Type == "volume") {
			found = true
		}
	}
	if !found {
		return nil, ErrUnsafe
	}
	if _, ok := c.NetworkSettings.Networks[l["com.docker.compose.project"]+"_default"]; !ok {
		return nil, ErrUnsafe
	}
	d.self = c
	return d, nil
}
func fmtString(v any) string { s, _ := v.(string); return s }

type probeTransport struct{ calls int }

func (p *probeTransport) RoundTrip(*http.Request) (*http.Response, error) {
	p.calls++
	return nil, ErrUnsafe
}
func policyCheck(d *docker) bool {
	p := newDocker()
	p.self = d.self
	p.target = d.target
	p.task = d.task
	spy := &probeTransport{}
	p.client = &http.Client{Transport: spy}
	for _, route := range []string{"/containers/foreign/exec", "/images/prune", "/volumes/prune", "/networks/create", "/build", "/plugins/create", "/containers/foreign/stop"} {
		if _, e := p.request(context.Background(), "POST", route, nil); !errors.Is(e, ErrUnsafe) {
			return false
		}
	}
	return spy.calls == 0
}

// PolicyProof exercises the deployed request gate using a transport that can
// never contact the daemon, then reads only this app's inspected identity.
func PolicyProof(ctx context.Context) (map[string]bool, error) {
	d, e := discover(ctx)
	if e != nil {
		return nil, e
	}
	pass := policyCheck(d)
	if !pass {
		return nil, ErrUnsafe
	}
	return map[string]bool{"scopedIdentityVerified": true, "forbiddenRequestsBlockedBeforeTransport": true, "daemonMutationPerformed": false}, nil
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	var v Status
	_, e := os.Stat(socket)
	v.SocketMounted = e == nil
	d, e := discover(ctx)
	v.Available = e == nil
	if v.Available {
		v.PolicyVerified = policyCheck(d)
		v.Available = v.PolicyVerified
	} else {
		v.Reason = "未挂载 docker.sock、权限不足或部署身份不符合安全要求；仍可检查更新"
	}
	j, e := latest(s.Root)
	if errors.Is(e, os.ErrNotExist) {
		e = nil
	}
	v.Job = j
	return v, e
}
func (s *Service) Start(ctx context.Context, actor, version, digest string) (Job, error) {
	var j Job
	unlock, e := lock(s.Root, "admission")
	if e != nil {
		return j, e
	}
	defer unlock()
	old, e := latest(s.Root)
	if e != nil {
		return j, e
	}
	if old != nil && (!old.Done() || old.Phase == "manual_recovery") {
		return j, ErrBusy
	}
	// No cached update-check result is trusted to execute an update.
	r, e := s.Checker.VerifyFresh(ctx)
	if e != nil || r.Version != version || r.Digest != digest {
		a := audit(ctx, s.DB, actor, "online_update.rejected", map[string]any{"reason": "release_hub_mismatch_or_unavailable", "fromVersion": buildinfo.Version, "requestedVersion": safeVersion(version)})
		if a != nil {
			return j, a
		}
		return j, updatecheck.ErrUntrusted
	}
	base := strings.Split(buildinfo.Version, "-")[0]
	n, ok := buildinfo.Compare(base, r.Version)
	if !ok || n >= 0 {
		return j, ErrUnsafe
	}
	if buildinfo.Schema < r.MinimumSchema {
		return j, ErrUnsafe
	}
	d, e := discover(ctx)
	if e != nil || !policyCheck(d) {
		return j, ErrUnsafe
	}
	id, e := newID()
	if e != nil {
		return j, e
	}
	j = Job{ID: id, Actor: actor, FromVersion: buildinfo.Version, FromImage: fmtString(d.self.Config["Image"]), Target: r, CreatedAt: time.Now().UTC()}
	p := plan{d.self, updatecheck.Repository + "@" + r.Digest, id}
	b, e := json.Marshal(p)
	if e != nil {
		return j, e
	}
	if e = atomicFile(filepath.Join(s.Root, id+".plan"), b); e != nil {
		return j, e
	}
	if e = advance(s.Root, &j, "queued"); e != nil {
		return j, e
	}
	if e = audit(ctx, s.DB, actor, "online_update.requested", j); e != nil {
		_ = advance(s.Root, &j, "failed")
		return j, e
	}
	go s.launch(id)
	return j, nil
}
func safeVersion(s string) string {
	if buildinfo.StableVersion(s) {
		return s
	}
	return "invalid"
}
func readPlan(root, id string) (plan, error) {
	var p plan
	if !idPattern.MatchString(id) {
		return p, ErrUnsafe
	}
	b, e := os.ReadFile(filepath.Join(root, id+".plan"))
	if e != nil {
		return p, e
	}
	e = json.Unmarshal(b, &p)
	if e != nil || p.JobID != id || !validImage(p.TargetImage) || len(p.Self.ID) != 64 || !containerID.MatchString(p.Self.ID) || labels(p.Self)["com.docker.compose.project"] == "" || labels(p.Self)["com.docker.compose.service"] != "app" {
		return p, ErrUnsafe
	}
	return p, nil
}
func (s *Service) launch(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	unlock, e := lock(s.Root, "launch")
	if e != nil {
		return
	}
	defer unlock()
	j, e := load(s.Root, id)
	if e != nil || j.Done() {
		return
	}
	p, e := readPlan(s.Root, id)
	if e != nil {
		return
	}
	d := newDocker()
	d.self = p.Self
	d.target = p.TargetImage
	d.task = id
	s.launchDocker(ctx, d, id)
}
func notFound(e error) bool { var v dockerError; return errors.As(e, &v) && v.code == 404 }
func (s *Service) failQueued(ctx context.Context, id, reason string) {
	j, e := load(s.Root, id)
	if e != nil || j.Phase != "queued" {
		return
	}
	j.Failure = reason
	if e = advance(s.Root, &j, "failed"); e != nil {
		deferutil.Report(e, "persist online update failure")
		return
	}
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	deferutil.Report(audit(auditCtx, s.DB, j.Actor, "online_update.failed", j), "audit online update failure")
}
func (s *Service) startHelper(ctx context.Context, d *docker, id, ref string) {
	_, e := d.request(ctx, "POST", "/containers/"+ref+"/start", nil)
	if e == nil {
		return
	}
	// An EOF/timeout after start may mean the helper is already replacing app.
	// Never overwrite its persisted phase or delete it on an unknown outcome.
	var httpErr dockerError
	if !errors.Is(e, ErrUnsafe) && (!errors.As(e, &httpErr) || httpErr.code >= 500) {
		return
	}
	c, inspectErr := d.inspect(ctx, d.helperName())
	if inspectErr != nil || c.State.Running || labels(c)[label] != id || labels(c)["io.mediatool.project"] != d.project() {
		return
	}
	// A definite failure plus a fresh stopped-state observation permits cleanup
	// of this task only. No volumes are deleted; uncertain removals are retried.
	if _, e = d.request(ctx, "DELETE", "/containers/"+d.helperName()+"?force=true", nil); e != nil {
		return
	}
	s.failQueued(ctx, id, "updater_start_failed")
}
func (s *Service) launchDocker(ctx context.Context, d *docker, id string) {
	j, e := load(s.Root, id)
	if e != nil || j.Done() {
		return
	}
	c, e := d.inspect(ctx, d.helperName())
	if e == nil {
		if labels(c)[label] != id || labels(c)["io.mediatool.project"] != d.project() {
			s.failQueued(ctx, id, "updater_identity_mismatch")
			return
		}
		if !c.State.Running {
			s.startHelper(ctx, d, id, d.helperName())
		}
		return
	}
	// Unavailable is not absent. Reconcile after restart, never duplicate a helper
	// based on a lost inspect response.
	if !notFound(e) {
		return
	}
	j, e = load(s.Root, id)
	if e != nil || j.Done() {
		return
	}
	if _, e = d.request(ctx, "POST", "/images/create?fromImage="+urlQuery(d.target)+"&platform=linux%2Famd64", nil); e != nil {
		s.failQueued(ctx, id, "image_pull_failed")
		return
	}
	b, e := d.request(ctx, "GET", "/images/"+d.target+"/json", nil)
	if e != nil {
		s.failQueued(ctx, id, "image_inspect_failed")
		return
	}
	var image struct {
		Architecture, Os string
		Config           struct{ Labels map[string]string }
	}
	if json.Unmarshal(b, &image) != nil || image.Architecture != "amd64" || image.Os != "linux" || image.Config.Labels["org.opencontainers.image.revision"] != j.Target.Commit || image.Config.Labels["org.opencontainers.image.version"] != j.Target.Version || image.Config.Labels["org.opencontainers.image.schema"] != strconv.FormatInt(j.Target.Schema, 10) || image.Config.Labels["org.opencontainers.image.source"] != "https://github.com/"+updatecheck.Repository {
		s.failQueued(ctx, id, "image_provenance_failed")
		return
	}
	body, e := json.Marshal(d.helperConfig())
	if e != nil {
		return
	}
	b, e = d.request(ctx, "POST", "/containers/create?name="+d.helperName(), body)
	if e != nil {
		var v dockerError
		if errors.Is(e, ErrUnsafe) || (errors.As(e, &v) && v.code >= 400 && v.code < 500 && v.code != 409) {
			s.failQueued(ctx, id, "updater_create_failed")
		}
		return
	}
	var created struct {
		ID string `json:"Id"`
	}
	if json.Unmarshal(b, &created) != nil || !containerID.MatchString(created.ID) {
		return
	}
	s.startHelper(ctx, d, id, created.ID)
}

func (s *Service) Run(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		j, e := latest(s.Root)
		if e == nil && j != nil && !j.Done() {
			go s.launch(j.ID)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
