// Package updatecheck only reads fixed public release sources. It never pulls
// images, executes updates, accesses Docker, or accepts an operator-supplied URL.
package updatecheck

import (
	"encoding/json"
	"errors"
	"io"
	"media-tool/internal/platform/buildinfo"
	"regexp"
	"strings"
	"time"
)

const Repository = "dingding229/mediatool"
const MetadataStart = "<!-- mediatool-release-metadata\n"

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var ErrUntrusted = errors.New("发布信息与镜像校验不一致，未提示更新")

type Metadata struct {
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	Digest        string `json:"digest"`
	MinimumSchema int64  `json:"minimumSchema"`
	Schema        int64  `json:"schema"`
	Platform      string `json:"platform"`
	Repository    string `json:"repository"`
}

func (v Metadata) Validate() error {
	if !buildinfo.StableVersion(v.Version) || !commitPattern.MatchString(v.Commit) || !digestPattern.MatchString(v.Digest) || v.MinimumSchema < 1 || v.Schema < v.MinimumSchema || v.Schema > 1000000 || v.Platform != "linux/amd64" || v.Repository != Repository {
		return ErrUntrusted
	}
	return nil
}
func ReleaseBody(notes string, m Metadata) (string, error) {
	if e := m.Validate(); e != nil {
		return "", e
	}
	if strings.Contains(notes, "mediatool-release-metadata") || len(notes) > 64000 {
		return "", ErrUntrusted
	}
	b, e := json.Marshal(m)
	if e != nil {
		return "", e
	}
	return strings.TrimSpace(notes) + "\n\n## 镜像与数据库\n\n- AMD64 不可变镜像：`" + Repository + "@" + m.Digest + "`\n- 最低 schema：" + jsonNumber(m.MinimumSchema) + "；目标 schema：" + jsonNumber(m.Schema) + "\n\n" + MetadataStart + string(b) + "\n-->\n", nil
}
func jsonNumber(v int64) string { b, _ := json.Marshal(v); return string(b) }

type Release struct {
	Metadata
	Tag         string    `json:"tag"`
	Notes       string    `json:"notes"`
	PublishedAt time.Time `json:"publishedAt"`
	URL         string    `json:"url"`
}

func parseRelease(tag, body string, at time.Time) (Release, error) {
	out := Release{}
	if !strings.HasPrefix(tag, "v") || !buildinfo.StableVersion(strings.TrimPrefix(tag, "v")) || len(body) > 96000 || at.IsZero() || at.After(time.Now().Add(5*time.Minute)) || strings.Count(body, MetadataStart) != 1 {
		return out, ErrUntrusted
	}
	parts := strings.SplitN(body, MetadataStart, 2)
	end := strings.Index(parts[1], "\n-->")
	if end < 0 || len(strings.TrimSpace(parts[1][end+4:])) != 0 {
		return out, ErrUntrusted
	}
	dec := json.NewDecoder(strings.NewReader(parts[1][:end]))
	dec.DisallowUnknownFields()
	if e := dec.Decode(&out.Metadata); e != nil || out.Metadata.Validate() != nil || tag != "v"+out.Version {
		return out, ErrUntrusted
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return out, ErrUntrusted
	}
	out.Tag, out.PublishedAt, out.Notes, out.URL = tag, at, strings.TrimSpace(parts[0]), "https://github.com/"+Repository+"/releases/tag/"+tag
	return out, nil
}
