package tooling

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ReadValues is data-only: it never evaluates shell substitutions or commands.
func ReadValues(path string) (map[string]string, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer func() { _ = f.Close() }()
	out := map[string]string{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !ok {
			return nil, errors.New("invalid configuration line")
		}
		v = strings.TrimSpace(v)
		if len(v) > 1 && ((v[0] == '\'' && v[len(v)-1] == '\'') || (v[0] == '"' && v[len(v)-1] == '"')) {
			v = v[1 : len(v)-1]
		}
		out[strings.TrimSpace(k)] = v
	}
	return out, s.Err()
}
func TestURLs(database, redis string) (string, string, error) {
	d, e := url.Parse(database)
	if e != nil || d.Host == "" || (d.Scheme != "postgres" && d.Scheme != "postgresql") || d.Path != "/media_tool_test" || d.Fragment != "" {
		return "", "", errors.New("tests may only use media_tool_test")
	}
	r, e := url.Parse(redis)
	if e != nil || r.Host == "" || (r.Scheme != "redis" && r.Scheme != "rediss") || r.Fragment != "" {
		return "", "", errors.New("isolated Redis test URL required")
	}
	r.Path = "/15"
	r.RawPath = ""
	q := r.Query()
	q.Del("db")
	r.RawQuery = q.Encode()
	return d.String(), r.String(), nil
}
func TestEnvironment(root string, getenv func(string) string) (map[string]string, error) {
	vals, e := ReadValues(filepath.Join(root, ".env.test"))
	if e != nil && !os.IsNotExist(e) {
		return nil, e
	}
	if vals == nil {
		vals = map[string]string{}
	}
	db := getenv("TEST_DATABASE_URL")
	if db == "" {
		db = vals["TEST_DATABASE_URL"]
	}
	redis := getenv("TEST_REDIS_URL")
	if redis == "" {
		redis = vals["TEST_REDIS_URL"]
	}
	if redis == "" {
		redis = getenv("REDIS_URL")
	}
	if redis == "" {
		v, e := ReadValues(filepath.Join(root, ".env"))
		if e != nil {
			return nil, errors.New("TEST_REDIS_URL required")
		}
		redis = v["REDIS_URL"]
	}
	db, redis, e = TestURLs(db, redis)
	if e != nil {
		return nil, e
	}
	out := map[string]string{"DATABASE_URL": db, "TEST_DATABASE_URL": db, "REDIS_URL": redis, "TEST_REDIS_URL": redis, "APP_ENV": "development", "APP_PUBLIC_URL": "http://localhost:18084"}
	for _, k := range []string{"APP_SECRET_KEY", "SESSION_SECRET_KEY", "STRM_API_KEY"} {
		out[k], e = Token()
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func initEnvironment(root string, ci bool) error {
	v := map[string]string{}
	for _, k := range []string{"SESSION_SECRET_KEY", "APP_SECRET_KEY", "STRM_API_KEY", "POSTGRES_PASSWORD", "REDIS_PASSWORD"} {
		s, e := Token()
		if e != nil {
			return e
		}
		v[k] = s
	}
	if ci {
		v["DATABASE_URL"] = "postgres://media_tool:ci-postgres-fixture@127.0.0.1:5432/media_tool_test?sslmode=disable"
		v["REDIS_URL"] = "redis://127.0.0.1:6379/15"
	} else {
		raw, e := os.ReadFile(filepath.Join(root, ".env.example"))
		if e != nil {
			return e
		}
		text := string(raw)
		for k, s := range v {
			text = strings.ReplaceAll(text, "CHANGE_ME_"+k, s)
			text = strings.ReplaceAll(text, k+"=\n", k+"="+s+"\n")
		}
		if e := exclusive(filepath.Join(root, ".env"), []byte(text)); e != nil {
			return e
		}
		if e := os.MkdirAll(filepath.Join(root, "data/strm"), 0700); e != nil {
			return e
		}
		_, e = fmt.Fprintln(os.Stdout, "Created private .env; existing credentials are never overwritten. Register the first administrator in the local UI.")
		return e
	}
	var text strings.Builder
	for _, k := range []string{"SESSION_SECRET_KEY", "APP_SECRET_KEY", "STRM_API_KEY", "DATABASE_URL", "REDIS_URL"} {
		fmt.Fprintf(&text, "%s=%s\n", k, v[k])
	}
	// Refuse either existing file before writing either configuration.
	for _, name := range []string{".env", ".env.test"} {
		if _, e := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(e) {
			return errors.New("configuration already exists; unchanged")
		}
	}
	if e := exclusive(filepath.Join(root, ".env"), []byte(text.String())); e != nil {
		return e
	}
	return exclusive(filepath.Join(root, ".env.test"), []byte("TEST_DATABASE_URL="+v["DATABASE_URL"]+"\nTEST_REDIS_URL="+v["REDIS_URL"]+"\n"))
}
func exclusive(path string, b []byte) (err error) {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer func() {
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
	}()
	if _, e = f.Write(b); e != nil {
		return e
	}
	return f.Sync()
}
