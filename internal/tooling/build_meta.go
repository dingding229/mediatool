package tooling

import (
	"encoding/json"
	"errors"
	"fmt"
	"media-tool/internal/platform/buildinfo"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type BuildMetadata struct {
	Version, Commit, BuildTime, LDFlags, Tags string
	Schema, MinimumSchema                     int64
}

var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

func buildMetadata(root, ref string, now time.Time) (BuildMetadata, error) {
	git := func(args ...string) (string, error) {
		c := exec.Command("git", args...)
		c.Dir = root
		b, e := c.Output()
		return strings.TrimSpace(string(b)), e
	}
	commit, e := git("rev-parse", "HEAD")
	if e != nil || !commitPattern.MatchString(commit) {
		return BuildMetadata{}, errors.New("Git commit required for build metadata")
	}
	version := ""
	tagged := strings.HasPrefix(ref, "refs/tags/")
	if tagged {
		tag := strings.TrimPrefix(ref, "refs/tags/")
		version = strings.TrimPrefix(tag, "v")
		if tag != "v"+version || !buildinfo.StableVersion(version) {
			return BuildMetadata{}, errors.New("release tag must be vX.Y.Z")
		}
		target, e := git("rev-list", "-n", "1", tag)
		if e != nil || target != commit {
			return BuildMetadata{}, errors.New("release tag must point to the checked out commit")
		}
	} else {
		tags, e := git("tag", "--merged", "HEAD", "--list", "v*")
		if e != nil {
			return BuildMetadata{}, e
		}
		base := "0.0.0"
		for _, tag := range strings.Fields(tags) {
			v := strings.TrimPrefix(tag, "v")
			if buildinfo.StableVersion(v) {
				if n, ok := buildinfo.Compare(v, base); ok && n > 0 {
					base = v
				}
			}
		}
		version = base + "-dev+" + commit[:12]
	}
	at := now.UTC().Format(time.RFC3339)
	flags := "-s -w -X media-tool/internal/platform/buildinfo.Version=" + version + " -X media-tool/internal/platform/buildinfo.Commit=" + commit + " -X media-tool/internal/platform/buildinfo.BuildTime=" + at
	tags := "dingding229/mediatool:sha-" + commit + ",dingding229/mediatool:latest"
	if tagged {
		tags += ",dingding229/mediatool:v" + version
	}
	return BuildMetadata{version, commit, at, flags, tags, buildinfo.Schema, buildinfo.MinimumSchema}, nil
}
func buildMetaCommand(root string, args []string) error {
	if len(args) != 1 {
		return errors.New("build-meta env|github|json")
	}
	m, e := buildMetadata(root, os.Getenv("GITHUB_REF"), time.Now())
	if e != nil {
		return e
	}
	switch args[0] {
	case "ldflags":
		_, e = fmt.Fprintln(os.Stdout, m.LDFlags)
		return e
	case "env":
		for _, v := range [][2]string{{"MEDIA_TOOL_BUILD_VERSION", m.Version}, {"MEDIA_TOOL_BUILD_COMMIT", m.Commit}, {"MEDIA_TOOL_BUILD_TIME", m.BuildTime}, {"MEDIA_TOOL_LDFLAGS", m.LDFlags}} {
			if _, e = fmt.Fprintf(os.Stdout, "export %s='%s'\n", v[0], strings.ReplaceAll(v[1], "'", "'\"'\"'")); e != nil {
				return e
			}
		}
		return nil
	case "github":
		_, e = fmt.Fprintf(os.Stdout, "version=%s\ncommit=%s\nbuild_time=%s\nschema=%d\nminimum_schema=%d\ntags=%s\n", m.Version, m.Commit, m.BuildTime, m.Schema, m.MinimumSchema, m.Tags)
		return e
	case "json":
		return json.NewEncoder(os.Stdout).Encode(m)
	default:
		return errors.New("build-meta env|github|json")
	}
}
