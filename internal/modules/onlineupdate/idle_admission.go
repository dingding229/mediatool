package onlineupdate

// These thin adapters expose the existing read/prepare operations to the idle
// coordinator. The F8b state machine, preflight and replacement are unchanged.
import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/updatecheck"
	"os"
	"path/filepath"
	"strconv"
)

func (s *Service) JobsByActor(actor string) ([]Job, error) {
	entries, e := os.ReadDir(s.Root)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Job{}
	for _, entry := range entries {
		name := entry.Name()
		if filepath.Ext(name) != ".json" || !idPattern.MatchString(name[:len(name)-5]) {
			continue
		}
		j, e := load(s.Root, name[:len(name)-5])
		if e != nil {
			return nil, e
		}
		if j.Actor == actor {
			out = append(out, j)
		}
	}
	return out, nil
}

func (s *Service) IdleTargetPreflight(ctx context.Context, target updatecheck.Release, pull bool) error {
	if e := target.Metadata.Validate(); e != nil {
		return ErrUnsafe
	}
	d, e := discover(ctx)
	if e != nil {
		return e
	}
	if !policyCheck(d) {
		return ErrUnsafe
	}
	d.target = updatecheck.Repository + "@" + target.Digest
	if pull {
		if _, e = d.request(ctx, "POST", "/images/create?fromImage="+urlQuery(d.target)+"&platform=linux%2Famd64", nil); e != nil {
			return e
		}
	}
	raw, e := d.request(ctx, "GET", "/images/"+d.target+"/json", nil)
	if e != nil {
		return e
	}
	var image struct {
		Architecture, Os string
		Config           struct{ Labels map[string]string }
	}
	if json.Unmarshal(raw, &image) != nil || image.Architecture != "amd64" || image.Os != "linux" || image.Config.Labels["org.opencontainers.image.revision"] != target.Commit || image.Config.Labels["org.opencontainers.image.version"] != target.Version || image.Config.Labels["org.opencontainers.image.schema"] != strconv.FormatInt(target.Schema, 10) || image.Config.Labels["org.opencontainers.image.source"] != "https://github.com/"+updatecheck.Repository {
		return ErrUnsafe
	}
	return d.imageStartupPreflight(ctx)
}

func (s *Service) IdleStatus(ctx context.Context) (Status, error) {
	var v Status
	d, e := discover(ctx)
	v.Available = e == nil
	if e == nil {
		v.PolicyVerified = policyCheck(d)
		v.Available = v.PolicyVerified
	}
	v.Job, e = latest(s.Root)
	if os.IsNotExist(e) {
		e = nil
	}
	if e != nil {
		return v, e
	}
	v.Control, e = s.Control(ctx)
	if len(v.Recovery) > 0 {
		v.Available = false
	}
	return v, e
}
