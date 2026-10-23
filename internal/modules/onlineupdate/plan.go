package onlineupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// The shared app data directory must never become a second plaintext copy of
// parent secrets: workers share its UID on hosts without Landlock. Only the root
// updater can read the full snapshot in the host-private backup directory.
func configurationProof(c Container) string {
	env := append([]string(nil), stringsValue(c.Config["Env"])...)
	sort.Strings(env)
	config := map[string]any{}
	for k, v := range c.Config {
		config[k] = v
	}
	config["Env"] = env
	mounts := append([]Mount(nil), c.Mounts...)
	sort.Slice(mounts, func(i, j int) bool {
		a, _ := json.Marshal(mounts[i])
		b, _ := json.Marshal(mounts[j])
		return string(a) < string(b)
	})
	b, _ := json.Marshal(struct {
		Config, Host map[string]any
		Mounts       []Mount
	}{config, c.HostConfig, mounts})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func newPlan(c Container, target, id string) plan {
	proof := configurationProof(c)
	originalLabels := labels(c)
	safeLabels := map[string]any{}
	for _, k := range []string{"com.docker.compose.project", "com.docker.compose.service", "com.docker.compose.project.working_dir", "com.docker.compose.project.config_files"} {
		safeLabels[k] = originalLabels[k]
	}
	config := map[string]any{"Image": c.Config["Image"], "User": c.Config["User"], "Labels": safeLabels}
	safe := Container{ID: c.ID, Name: c.Name, Image: c.Image, Config: config}
	for _, m := range c.Mounts {
		if m.Destination == "/data" {
			safe.Mounts = append(safe.Mounts, m)
		}
	}
	safe.NetworkSettings.Networks = map[string]json.RawMessage{originalLabels["com.docker.compose.project"] + "_default": json.RawMessage(`{}`)}
	return plan{Self: safe, TargetImage: target, JobID: id, ConfigurationProof: proof}
}
func originalContainer(ctx context.Context, d *docker, p plan, j Job) (Container, error) {
	var c Container
	if j.Phase == "applying" || j.Phase == "checking" || j.Phase == "rolling_back" {
		// A durable full snapshot is written before the applying checkpoint. This
		// path is outside /data and is never mounted into the persistent app/worker.
		work := labels(p.Self)["com.docker.compose.project.working_dir"]
		if j.Backup == "" || filepath.Dir(j.Backup) != filepath.Join(work, "private") {
			return c, ErrUnsafe
		}
		b, e := os.ReadFile(filepath.Join(j.Backup, "container.before.json"))
		if e != nil {
			return c, ErrUnsafe
		}
		if json.Unmarshal(b, &c) != nil {
			return c, ErrUnsafe
		}
	} else {
		var e error
		c, e = d.inspect(ctx, p.Self.ID)
		if e != nil {
			return c, e
		}
	}
	if c.ID != p.Self.ID || c.Name != p.Self.Name || fmtString(c.Config["Image"]) != j.FromImage || c.Config["User"] != "65532:65532" || labels(c)["com.docker.compose.project"] != d.project() || labels(c)["com.docker.compose.service"] != "app" || configurationProof(c) != p.ConfigurationProof {
		return Container{}, ErrUnsafe
	}
	return c, nil
}
