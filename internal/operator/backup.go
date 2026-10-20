package operator

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

func backupVerify(root, archive string) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
	defer cancel()
	o := newOwner(ctx, root)
	if o.err != nil {
		return o.err
	}
	nonce, e := Random(6)
	if e != nil {
		return e
	}
	clone := "media_tool_restore_" + nonce
	created := false
	defer func() {
		if created {
			o.err = nil
			o.docker("exec", "-T", "postgres", "sh", "-c", `dropdb -U "$POSTGRES_USER" "$1"`, "sh", clone)
			err = errors.Join(err, o.err)
		}
	}()
	var source map[string]any
	if archive == "" {
		dir := filepath.Join(root, "data/backups")
		if e = os.MkdirAll(dir, 0700); e != nil {
			return e
		}
		if e = os.Chmod(dir, 0700); e != nil {
			return e
		}
		archive = filepath.Join(dir, "media-tool-"+time.Now().UTC().Format("20060102T150405Z")+"-"+nonce+".dump")
		args := append(append([]string{}, o.base...), "exec", "-T", "app", "/app/media-tool", "backup", "verify", "-source-snapshot")
		exporter := exec.CommandContext(ctx, args[0], args[1:]...)
		exporter.Dir = root
		stdin, e := exporter.StdinPipe()
		if e != nil {
			return e
		}
		stdout, e := exporter.StdoutPipe()
		if e != nil {
			_ = stdin.Close()
			return e
		}
		if e = exporter.Start(); e != nil {
			_ = stdin.Close()
			return errors.New("snapshot exporter unavailable")
		}
		released := false
		defer func() {
			if !released {
				_ = exporter.Process.Kill()
				_ = exporter.Wait()
			}
		}()
		if e = json.NewDecoder(io.LimitReader(stdout, 1<<20)).Decode(&source); e != nil {
			return errors.New("snapshot receipt invalid")
		}
		snapshot, ok := source["snapshot"].(string)
		if !ok || !regexp.MustCompile(`^[0-9a-fA-F]+-[0-9a-fA-F]+-\d+$`).MatchString(snapshot) {
			return errors.New("invalid snapshot identity")
		}
		a := append(append([]string{}, o.base...), "exec", "-T", "postgres", "sh", "-c", `pg_dump -Fc --snapshot "$1" -U "$POSTGRES_USER" -d "$POSTGRES_DB"`, "sh", snapshot)
		dump := exec.CommandContext(ctx, a[0], a[1:]...)
		dump.Dir = root
		if _, e = dumpTo(dump, archive, false); e != nil {
			return e
		}
		if _, e = fmt.Fprintln(stdin, "release"); e != nil {
			return e
		}
		if e = stdin.Close(); e != nil {
			return e
		}
		if e = exporter.Wait(); e != nil {
			return errors.New("snapshot exporter failed")
		}
		released = true
	}
	st, e := os.Lstat(archive)
	if e != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
		return errors.New("archive must be a private regular file")
	}
	input, e := os.Open(archive)
	if e != nil {
		return e
	}
	defer func() { err = errors.Join(err, input.Close()) }()
	h := sha256.New()
	if _, e = io.Copy(h, input); e != nil {
		return e
	}
	if _, e = input.Seek(0, 0); e != nil {
		return e
	}
	o.docker("exec", "-T", "postgres", "sh", "-c", `createdb -U "$POSTGRES_USER" "$1"`, "sh", clone)
	if o.err != nil {
		return o.err
	}
	created = true
	var reader io.Reader = input
	tool := "pg_restore --exit-on-error --no-owner"
	if filepath.Ext(archive) == ".gz" {
		gz, e := gzip.NewReader(input)
		if e != nil {
			return e
		}
		defer func() { err = errors.Join(err, gz.Close()) }()
		reader = gz
		tool = "psql -v ON_ERROR_STOP=1"
	}
	a := append(append([]string{}, o.base...), "exec", "-T", "postgres", "sh", "-c", tool+` -U "$POSTGRES_USER" -d "$1"`, "sh", clone)
	restore := exec.CommandContext(ctx, a[0], a[1:]...)
	restore.Dir = root
	restore.Stdin = reader
	restore.Stdout = io.Discard
	restore.Stderr = io.Discard
	if e = restore.Run(); e != nil {
		return errors.New("private clone restore failed")
	}
	raw := o.docker("exec", "-T", "-e", "BACKUP_VERIFY_DB="+clone, "app", "/app/media-tool", "backup", "verify", "-database", clone)
	if o.err != nil {
		return o.err
	}
	var verified map[string]any
	if e = json.Unmarshal([]byte(raw), &verified); e != nil {
		return errors.New("restore receipt invalid")
	}
	if source != nil && string(jsonBytes(source["tables"])) != string(jsonBytes(verified["initialTables"])) {
		return errors.New("snapshot and restored checksums differ")
	}
	o.docker("exec", "-T", "postgres", "sh", "-c", `dropdb -U "$POSTGRES_USER" "$1"`, "sh", clone)
	if o.err != nil {
		return o.err
	}
	created = false
	var matched any
	if source != nil {
		matched = true
	}
	receipt := map[string]any{"archive": archive, "sha256": hex.EncodeToString(h.Sum(nil)), "permissions": "0600", "sourceSnapshotMatched": matched, "existingBaselineArchive": source == nil, "restoreDatabaseWasTemporary": true, "temporaryDatabaseDeleted": true, "credentialsMasterKeyIncluded": false, "restored": verified}
	dir := filepath.Join(root, "data/diagnostics")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	if e = Atomic(filepath.Join(dir, "backup-restore-"+nonce+".json"), jsonBytes(receipt)); e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(receipt)
}
