package onlineupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/jackc/pgx/v5"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRealDumpRestoresDataAndIrreversibleDDL(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated media_tool_test required")
	}
	cfg, e := pgx.ParseConfig(dsn)
	if e != nil || cfg.Database != "media_tool_test" || os.Getenv("MEDIA_TOOL_TEST_LOCKED") != "1" {
		t.Fatal("serialized test database only")
	}
	ctx := t.Context()
	db, e := pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(ctx, `DROP TABLE IF EXISTS f8b_before,f8b_after;CREATE TABLE f8b_before(value text);INSERT INTO f8b_before VALUES('original')`); e != nil {
		t.Fatal(e)
	}
	_ = db.Close(ctx)
	t.Cleanup(func() {
		c, e := pgx.Connect(context.Background(), dsn)
		if e == nil {
			_, _ = c.Exec(context.Background(), `DROP TABLE IF EXISTS f8b_before,f8b_after`)
			_ = c.Close(context.Background())
		}
	})
	root := t.TempDir()
	r := &runner{dsn: dsn}
	r.runPG = testPG(t)
	f, e := os.Create(filepath.Join(root, "database.dump"))
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.New()
	if e = r.pg(ctx, "pg_dump", []string{"-Fc", "--no-password"}, nil, io.MultiWriter(f, h)); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	db, e = pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(ctx, `UPDATE f8b_before SET value='changed';ALTER TABLE f8b_before ADD COLUMN irreversible text;CREATE TABLE f8b_after(id int)`); e != nil {
		t.Fatal(e)
	}
	_ = db.Close(ctx)
	j := Job{DumpSHA256: hex.EncodeToString(h.Sum(nil))}
	if e = r.restoreDatabase(ctx, root, j); e != nil {
		t.Fatal(e)
	}
	db, e = pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close(ctx)
	var value string
	var extra, added int
	if e = db.QueryRow(ctx, `SELECT value FROM f8b_before`).Scan(&value); e != nil {
		t.Fatal(e)
	}
	_ = db.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name='f8b_before' AND column_name='irreversible'`).Scan(&extra)
	_ = db.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_name='f8b_after'`).Scan(&added)
	if value != "original" || extra != 0 || added != 0 {
		t.Fatal(value, extra, added)
	}
	j.DumpSHA256 = strings.Repeat("0", 64)
	if e = r.restoreDatabase(ctx, root, j); e == nil {
		t.Fatal("tampered backup accepted")
	}
}
func testPG(t *testing.T) func(context.Context, string, []string, []string, io.Reader, io.Writer) error {
	return func(ctx context.Context, name string, args, env []string, in io.Reader, out io.Writer) error {
		container := os.Getenv("TEST_POSTGRES_CONTAINER")
		if container == "" {
			container = "media-tool-postgres-1"
		}
		args = append([]string(nil), args...)
		for i, arg := range args {
			if strings.HasPrefix(arg, "--use-list=") {
				id, _ := newID()
				remote := "/tmp/mediatool-f8b-" + id
				list, e := os.Open(strings.TrimPrefix(arg, "--use-list="))
				if e != nil {
					return e
				}
				c := exec.CommandContext(ctx, "docker", "exec", "-i", container, "sh", "-c", "umask 077; cat > \"$1\"", "sh", remote+".list")
				c.Stdin = list
				e = c.Run()
				_ = list.Close()
				if e != nil {
					return e
				}
				defer exec.Command("docker", "exec", container, "rm", "-f", remote+".list", remote+".dump").Run()
				args[i] = "--use-list=" + remote + ".list"
				c = exec.CommandContext(ctx, "docker", "exec", "-i", container, "sh", "-c", "umask 077; cat > \"$1\"", "sh", remote+".dump")
				c.Stdin = in
				if e = c.Run(); e != nil {
					return e
				}
				in = nil
				args = append(args, remote+".dump")
			}
		}
		a := []string{"exec", "-i"}
		for _, v := range env {
			if strings.HasPrefix(v, "PG") && !strings.HasPrefix(v, "PGHOST=") && !strings.HasPrefix(v, "PGPORT=") {
				a = append(a, "-e", v)
			}
		}
		a = append(a, "-e", "PGHOST=127.0.0.1", "-e", "PGPORT=5432", container, name)
		a = append(a, args...)
		c := exec.CommandContext(ctx, "docker", a...)
		c.Stdin = in
		c.Stdout = out
		var stderr strings.Builder
		c.Stderr = &stderr
		e := c.Run()
		if e != nil {
			t.Log("test PG command error", name, stderr.String())
		}
		return e
	}
}
