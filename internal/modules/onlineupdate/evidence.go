package onlineupdate

import (
	"bytes"
	"encoding/json"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// stderr is untrusted and may quote full commands/configuration. Redact every
// environment value first (longest first), then all URLs and auth-like fields.
// Never persist Docker request/response bodies or environment snapshots in /data.
var evidenceURL = regexp.MustCompile(`(?i)(?:https?|postgres(?:ql)?|redis)://[^\s"'<>]+`)
var evidenceSecret = regexp.MustCompile(`(?i)(password|token|cookie|authorization|api[_-]?key|dsn)(\s*[:=]\s*)[^\s,;]+`)

func (r *runner) evidence(stage string, stderr []byte, failed bool) error {
	if !filepath.IsAbs(r.root) || !idPattern.MatchString(r.d.task) {
		return ErrUnsafe
	}
	text := string(stderr)
	values := stringsValue(r.d.self.Config["Env"])
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	// Replace full values, including non-URL secret formats, without keeping a copy.
	for _, env := range values {
		_, value, ok := strings.Cut(env, "=")
		if ok && len(value) >= 3 {
			text = strings.ReplaceAll(text, value, "[redacted]")
			if u, e := url.Parse(value); e == nil && u.User != nil {
				if p, ok := u.User.Password(); ok && p != "" {
					text = strings.ReplaceAll(text, p, "[redacted]")
				}
			}
		}
	}
	text = evidenceURL.ReplaceAllString(text, "[url redacted]")
	text = evidenceSecret.ReplaceAllString(text, "$1$2[redacted]")
	if len(text) > 16384 {
		text = text[:16384] + " [truncated]"
	}
	b, e := json.Marshal(struct {
		At     time.Time `json:"at"`
		Stage  string    `json:"stage"`
		Failed bool      `json:"failed"`
		Stderr string    `json:"stderr"`
	}{time.Now().UTC(), stage, failed, text})
	if e != nil {
		return e
	}
	dir := filepath.Join(r.root, r.d.task+".evidence")
	if e := prepare(dir); e != nil {
		return e
	}
	return atomicFile(filepath.Join(dir, strconv.FormatInt(time.Now().UnixNano(), 10)+"-"+stage+".stderr.json"), b)
}

// Limit collection before redaction as well as persistence; pipe writers must
// always consume their input to avoid blocking subprocesses.
type stderrBuffer struct{ bytes.Buffer }

func (b *stderrBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len() < 65536 {
		remaining := 65536 - b.Len()
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}

func (r *runner) stageEvidence(stage string, result error) error {
	text := ""
	if result != nil {
		text = result.Error()
	}
	return r.evidence(stage, []byte(text), result != nil)
}
