package maintenance

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	deferutil "media-tool/internal/platform/cleanup"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/platform/backupverify"
)

type Command func(context.Context, string, []string, []string, io.Reader, io.Writer) error

func runCommand(ctx context.Context, name string, args, env []string, in io.Reader, out io.Writer) error {
	if name != "pg_dump" && name != "pg_restore" && name != "psql" {
		return ErrUnsafe
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=/tmp", "TZ=Asia/Shanghai"}, env...)
	cmd.Stdin = in
	cmd.Stdout = out
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{Writer: &stderr, Left: 4096}
	if err := cmd.Run(); err != nil {
		if name == "psql" {
			return fmt.Errorf("数据库原子导入失败（数据详情不输出）：%w", err)
		}
		return fmt.Errorf("%s失败: %s: %w", name, strings.TrimSpace(stderr.String()), err)
	}
	return nil
}

type limitedWriter struct {
	io.Writer
	Left int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) > w.Left {
		p = p[:w.Left]
	}
	if len(p) > 0 {
		if _, err := w.Writer.Write(p); err != nil {
			return 0, err
		}
		w.Left -= len(p)
	}
	return n, nil
}
func (e Engine) pg(ctx context.Context, name, dsn string, args []string, in io.Reader, out io.Writer) error {
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() == "" || u.User == nil {
		return ErrUnsafe
	}
	password, _ := u.User.Password()
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	env := []string{"PGHOST=" + u.Hostname(), "PGPORT=" + port, "PGDATABASE=" + strings.TrimPrefix(u.Path, "/"), "PGUSER=" + u.User.Username(), "PGPASSWORD=" + password, "PGCONNECT_TIMEOUT=10", "PGAPPNAME=mediatool-maintenance"}
	for q, k := range map[string]string{"sslmode": "PGSSLMODE", "sslrootcert": "PGSSLROOTCERT", "sslcert": "PGSSLCERT", "sslkey": "PGSSLKEY"} {
		if v := u.Query().Get(q); v != "" {
			env = append(env, k+"="+v)
		}
	}
	run := e.Run
	if run == nil {
		run = runCommand
	}
	return run(ctx, name, args, env, in, out)
}

type FileProof struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Bytes  int64  `json:"bytes"`
}
type Manifest struct {
	ID             string                        `json:"id"`
	Format         int                           `json:"format"`
	Schema         int64                         `json:"schema"`
	Revision       string                        `json:"revision"`
	CreatedAt      time.Time                     `json:"createdAt"`
	KeyFingerprint string                        `json:"keyFingerprint"`
	DatabaseDigest string                        `json:"databaseDigest"`
	OutputsDigest  string                        `json:"outputsDigest"`
	Files          []FileProof                   `json:"files"`
	Tables         map[string]backupverify.Table `json:"tables"`
	Protected      map[string]string             `json:"protected"`
}

var protectedTables = []string{"manual_download_tasks", "administrator_accounts", "application_settings", "directory_classification", "integrations", "open115_authorizations", "storage_connections", "media_users", "media_user_library_grants", "player_ua_policy", "player_ua_rules", "player_ip_block_rules", "player_ip_block_policy", "playback_grants", "external_api_keys", "cloud_files", "media_assets", "media_items", "media_asset_bindings", "strm_entries", "managed_media_artifacts"}

func Protected(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (map[string]string, error) {
	out := map[string]string{}
	for _, name := range protectedTables {
		var raw string
		expression := "to_jsonb(t)"
		if name == "player_ip_block_policy" {
			expression = "(to_jsonb(t)-'last_counter_batch')"
		}
		if name == "player_ip_block_rules" {
			expression = "(to_jsonb(t)-'hit_count'-'last_hit_at')"
		}
		if name == "external_api_keys" {
			expression = "(to_jsonb(t)-'last_used_at')"
		}
		sql := `SELECT coalesce(md5(string_agg(` + expression + `::text,E'\n' ORDER BY ` + expression + `::text)),md5('')) FROM ` + pgx.Identifier{name}.Sanitize() + ` t`
		if err := db.QueryRow(ctx, sql).Scan(&raw); err != nil {
			return nil, err
		}
		out[name] = raw
	}
	return out, nil
}
func digestFile(file string) (string, int64, error) {
	f, err := os.OpenFile(file, os.O_RDONLY|noFollowFlag(), 0)
	if err != nil {
		return "", 0, err
	}
	defer deferutil.Close(f, "Read/descriptor teardown only; primary operations handle their own I/O result")
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", 0, ErrUnsafe
	}
	h := shaHasher()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", n, err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), n, nil
}
func shaHasher() hashWriter { return newSHA() }

// Check archive identity before any restoration; archive names come only from UUIDs.
func (e Engine) VerifyBundle(ctx context.Context, id string) (Manifest, string, error) {
	var m Manifest
	dir, err := e.dir(id)
	if err != nil {
		return m, "", err
	}
	var raw []byte
	var state, digest string
	if err = e.DB.QueryRow(ctx, `SELECT state,digest FROM ops_backups WHERE id=$1`, id).Scan(&state, &digest); err != nil || state != "ready" {
		return m, "", ErrUnsafe
	}
	raw, err = os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return m, "", err
	}
	if hash(raw) != digest || json.Unmarshal(raw, &m) != nil || m.Format != 1 || m.ID != id {
		return m, "", ErrUnsafe
	}
	if m.KeyFingerprint != hash([]byte("key:"+e.Key)) {
		return m, "", errors.New("备份主密钥与当前主密钥不匹配，无法安全解密，已拒绝恢复；请由所有者核对原主密钥")
	}
	for file, expected := range map[string]string{"database.dump": m.DatabaseDigest, "outputs.tar": m.OutputsDigest} {
		actual, _, err := digestFile(filepath.Join(dir, file))
		if err != nil || actual != expected {
			return m, "", errors.New("备份文件摘要不一致")
		}
	}
	return m, dir, nil
}
func (e Engine) Backup(ctx context.Context, id string) (err error) {
	dir, err := e.dir(id)
	if err != nil {
		return err
	}
	tag, err := e.DB.Exec(ctx, `UPDATE ops_backups SET state='running' WHERE id=$1 AND state='pending'`, id)
	if err != nil || tag.RowsAffected() != 1 {
		return ErrConflict
	}
	defer func() {
		if err != nil {
			cleanup, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()
			var partial int64
			for _, name := range []string{"database.dump", "outputs.tar", "manifest.json"} {
				if info, statErr := os.Lstat(filepath.Join(dir, name)); statErr == nil && info.Mode().IsRegular() {
					partial += info.Size()
				}
			}
			if _, checkedErr := e.DB.Exec(cleanup, `UPDATE ops_backups SET state='failed',message=$2,bytes=$3,completed_at=now() WHERE id=$1`, id, err.Error(), partial); checkedErr != nil {
				err = errors.Join(err, checkedErr)
				return
			}
			e.notice(cleanup, "ops.backup.failed", "备份失败", err.Error())
		}
	}()
	if err = privateDir(e.Root); err != nil {
		return err
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		return errors.New("备份目录已存在，不重复执行未知结果")
	}
	tx, err := e.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		deferutil.Rollback(cleanup, tx)
	}()
	var snapshot string
	if err = tx.QueryRow(ctx, `SELECT pg_export_snapshot()`).Scan(&snapshot); err != nil {
		return err
	}
	m := Manifest{ID: id, Format: 1, Revision: e.Revision, CreatedAt: time.Now(), KeyFingerprint: hash([]byte("key:" + e.Key)), Files: []FileProof{}}
	if err = tx.QueryRow(ctx, SchemaSQL).Scan(&m.Schema); err != nil {
		return err
	}
	m.Protected, err = Protected(ctx, tx)
	if err != nil {
		return err
	}
	// Reuse full inventory from a held export; this second export is not used for pg_dump.
	// Tables must be read in the same snapshot as the dump.
	m.Tables, err = Inventory(ctx, tx)
	if err != nil {
		return err
	}
	dump, err := os.OpenFile(filepath.Join(dir, "database.dump"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	err = e.pg(ctx, "pg_dump", e.DSN, []string{"--format=custom", "--schema=public", "--no-owner", "--no-acl", "--snapshot=" + snapshot}, nil, dump)
	syncErr := dump.Sync()
	closeErr := dump.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if syncErr != nil {
		return syncErr
	}
	m.DatabaseDigest, _, err = digestFile(filepath.Join(dir, "database.dump"))
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT s.output_path,coalesce((SELECT encode(j.content_digest,'hex')FROM media_artifact_journal j WHERE j.relative_path=s.output_path AND j.asset_id=s.asset_id AND j.artifact_kind='strm' AND j.state='applied' ORDER BY j.updated_at DESC LIMIT 1),'')FROM strm_entries s UNION SELECT relative_path,encode(content_digest,'hex') FROM managed_media_artifacts ORDER BY 1`)
	if err != nil {
		return err
	}
	type owned struct{ path, digest string }
	files := []owned{}
	for rows.Next() {
		var f owned
		if err = rows.Scan(&f.path, &f.digest); err != nil {
			break
		}
		files = append(files, f)
	}
	rows.Close()
	if err != nil {
		return err
	}
	if rows.Err() != nil {
		return rows.Err()
	}
	out, err := os.OpenFile(filepath.Join(dir, "outputs.tar"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(out)
	defer deferutil.Close(out, "Read/descriptor teardown only; primary operations handle their own I/O result")
	fs, err := os.OpenRoot(e.OutputRoot)
	if err != nil {
		return err
	}
	defer deferutil.Close(fs, "Read/descriptor teardown only; primary operations handle their own I/O result")
	seen := map[string]bool{}
	for _, file := range files {
		if seen[file.path] {
			continue
		}
		seen[file.path] = true
		if !safeRelative(file.path) {
			return ErrUnsafe
		}
		for parent := path.Dir(file.path); parent != "."; parent = path.Dir(parent) {
			info, parentErr := fs.Lstat(parent)
			if parentErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return errors.New("托管输出父目录无效或为符号链接: " + file.path)
			}
		}
		info, statErr := fs.Lstat(file.path)
		if statErr != nil || !info.Mode().IsRegular() || info.Size() > 64<<20 {
			return errors.New("托管输出缺失、非普通文件或过大: " + file.path)
		}
		src, openErr := fs.OpenFile(file.path, os.O_RDONLY|noFollowFlag(), 0)
		if openErr != nil {
			return openErr
		}
		raw, readErr := io.ReadAll(io.LimitReader(src, (64<<20)+1))
		deferutil.Close(src, "Read/descriptor teardown only; primary operations handle their own I/O result")
		if readErr != nil || int64(len(raw)) != info.Size() {
			return errors.New("托管文件读取期间发生变化")
		}
		digest := hash(raw)
		if file.digest != "" && digest != file.digest {
			return errors.New("托管输出与归属摘要不符: " + file.path)
		}
		if err = tw.WriteHeader(&tar.Header{Name: file.path, Mode: 0600, Size: int64(len(raw)), ModTime: info.ModTime()}); err != nil {
			return err
		}
		if _, err = tw.Write(raw); err != nil {
			return err
		}
		m.Files = append(m.Files, FileProof{Path: file.path, Digest: digest, Bytes: int64(len(raw))})
	}
	if err = tw.Close(); err != nil {
		return err
	}
	if err = out.Sync(); err != nil {
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	m.OutputsDigest, _, err = digestFile(filepath.Join(dir, "outputs.tar"))
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(m, "", "  ")
	mf, err := os.OpenFile(filepath.Join(dir, "manifest.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = mf.Write(raw)
	syncErr = mf.Sync()
	closeErr = mf.Close()
	if err == nil {
		err = closeErr
	}
	if syncErr != nil {
		return syncErr
	}
	if err != nil {
		return err
	}
	directory, directoryErr := os.Open(dir)
	if directoryErr != nil {
		return directoryErr
	}
	syncErr = directory.Sync()
	deferutil.Close(directory, "Read/descriptor teardown only; primary operations handle their own I/O result")
	if syncErr != nil {
		return syncErr
	}
	var total int64
	for _, name := range []string{"database.dump", "outputs.tar", "manifest.json"} {
		i, statErr := os.Stat(filepath.Join(dir, name))
		if statErr != nil {
			return statErr
		}
		total += i.Size()
	}
	_, err = e.DB.Exec(ctx, `UPDATE ops_backups SET state='ready',digest=$2,manifest=$3,bytes=$4,completed_at=now(),message='数据库一致快照与托管输出已校验；主密钥和云端原媒体不在备份中' WHERE id=$1`, id, hash(raw), raw, total)
	if err == nil {
		e.notice(ctx, "ops.backup.ready", "备份已完成", id)
	}
	return err
}
func safeRelative(p string) bool {
	return p != "" && p != "." && !strings.HasPrefix(p, "/") && path.Clean(p) == p && !strings.Contains(p, "\\") && !strings.HasPrefix(p, "../") && !strings.ContainsAny(p, "\x00\r\n")
}
func (e Engine) Drill(ctx context.Context, id string) (err error) {
	var backup string
	err = e.DB.QueryRow(ctx, `UPDATE ops_drills SET state='running' WHERE id=$1 AND state='pending' RETURNING backup_id::text`, id).Scan(&backup)
	if err != nil {
		return err
	}
	evidence := map[string]any{"created": false, "databaseDeleted": false}
	var report any = evidence
	defer func() {
		cleanup, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		state := "verified"
		message := "临时库恢复、表摘要、密文和迁移幂等已验证；未连接业务Worker"
		if err != nil {
			state = "failed"
			message = err.Error()
		}
		raw, _ := json.Marshal(report)
		if _, checkedErr := e.DB.Exec(cleanup, `UPDATE ops_drills SET state=$2,report=$3,message=$4,completed_at=now() WHERE id=$1`, id, state, raw, message); checkedErr != nil {
			err = errors.Join(err, checkedErr)
			return
		}
		e.notice(cleanup, "ops.drill."+state, "备份恢复演练", message)
	}()
	m, dir, err := e.VerifyBundle(ctx, backup)
	if err != nil {
		return err
	}
	name := "media_tool_restore_" + strings.ReplaceAll(id, "-", "")[:12]
	u, _ := url.Parse(e.DSN)
	if strings.TrimPrefix(u.Path, "/") == name {
		return ErrUnsafe
	}
	if _, err = e.DB.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()+` TEMPLATE template0`); err != nil {
		return err
	}
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, dropErr := e.DB.Exec(clean, `DROP DATABASE `+pgx.Identifier{name}.Sanitize()+` WITH(FORCE)`)
		if dropErr == nil {
			evidence["databaseDeleted"] = true
		}
		if dropErr != nil {
			err = errors.Join(err, errors.New("临时演练库清理失败，请核对 "+name))
		}
	}()
	evidence["created"] = true
	evidence["createdDatabase"] = name
	proofRaw, _ := json.Marshal(evidence)
	if _, err = e.DB.Exec(ctx, `UPDATE ops_drills SET report=$2 WHERE id=$1`, id, proofRaw); err != nil {
		return err
	}
	u.Path = "/" + name
	dsn := u.String()
	dump, err := os.Open(filepath.Join(dir, "database.dump"))
	if err != nil {
		return err
	}
	err = e.pg(ctx, "pg_restore", dsn, []string{"--schema=public", "--exit-on-error", "--no-owner", "--no-acl", "--dbname=" + name}, dump, io.Discard)
	deferutil.Close(dump, "Read/descriptor teardown only; primary operations handle their own I/O result")
	if err != nil {
		return err
	}
	verification, verifyErr := e.verifyRestored(ctx, dsn, name, m, false)
	evidence["verification"] = verification
	return verifyErr
}
func Inventory(ctx context.Context, db backupverify.Querier) (map[string]backupverify.Table, error) {
	return backupverify.Inventory(ctx, db, nil)
}
func sameTables(a, b map[string]backupverify.Table) bool {
	if len(a) != len(b) {
		return false
	}
	for name, x := range a {
		y, ok := b[name]
		if !ok || x.Rows != y.Rows || x.Digest != y.Digest {
			return false
		}
	}
	return true
}
func (e Engine) notice(ctx context.Context, kind, title, body string) {
	raw, _ := json.Marshal(map[string]string{"title": title, "message": body})
	if _, checkedErr := e.DB.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)VALUES($1,$2,$1,$3)`, newID(), kind, raw); checkedErr != nil {
		deferutil.Report(checkedErr, "internal/modules/maintenance/backup.go:notice")
		return
	}
}
