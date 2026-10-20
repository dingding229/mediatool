package operator

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// dumpTo never stores uncompressed stdout or raw command errors in public output.
func dumpTo(cmd *exec.Cmd, path string, compress bool) (digest string, err error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return "", e
	}
	defer func() {
		if e := f.Close(); err == nil {
			err = e
		}
	}()
	h := sha256.New()
	var out io.Writer = io.MultiWriter(f, h)
	var gz *gzip.Writer
	if compress {
		gz = gzip.NewWriter(out)
		out = gz
	}
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	if e = cmd.Run(); e != nil {
		return "", errors.New("private snapshot export failed")
	}
	if gz != nil {
		if e = gz.Close(); e != nil {
			return "", e
		}
	}
	if e = f.Sync(); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (o *owner) update(ref string) map[string]any {
	release := o.inspect(ref)
	if o.err != nil {
		return nil
	}
	nonce, e := Random(3)
	o.fail(e)
	parent := filepath.Join(o.root, "private")
	o.fail(os.MkdirAll(parent, 0700))
	o.fail(os.Chmod(parent, 0700))
	dir := filepath.Join(parent, "update-"+time.Now().UTC().Format("20060102T150405Z")+"-"+nonce)
	if o.err == nil {
		o.fail(os.Mkdir(dir, 0700))
	}
	before, e := os.ReadFile(o.compose)
	o.fail(e)
	if o.err == nil {
		o.fail(Atomic(filepath.Join(dir, "docker-compose.yml"), before))
	}
	if o.err != nil {
		return nil
	}
	args := append(append([]string{}, o.base...), "exec", "-T", "postgres", "sh", "-c", `exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB"`)
	cmd := exec.CommandContext(o.ctx, args[0], args[1:]...)
	cmd.Dir = o.root
	digest, e := dumpTo(cmd, filepath.Join(dir, "database.sql.gz"), true)
	o.fail(e)
	o.write(filepath.Join(dir, "backup-receipt.json"), map[string]string{"sha256": digest})
	after, e := ReplaceImage(string(before), "dingding229/mediatool@"+release.Digest)
	o.fail(e)
	if o.err == nil {
		o.fail(Atomic(o.compose, []byte(after)))
	}
	o.docker("up", "-d", "--no-deps", "--wait", "--wait-timeout", "180", "app")
	o.docker("ps", "app", "postgres", "redis")
	return map[string]any{"installed": o.err == nil, "image": "dingding229/mediatool@" + release.Digest, "backup": dir, "automaticSchemaDowngrade": false}
}
