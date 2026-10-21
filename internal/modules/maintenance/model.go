package maintenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	deferutil "media-tool/internal/platform/cleanup"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/scheduling"
)

const SchemaSQL = `SELECT coalesce(max(version_id),0) FROM(SELECT DISTINCT ON(version_id)version_id,is_applied FROM goose_db_version ORDER BY version_id,id DESC)v WHERE is_applied`

var ErrConflict = errors.New("维护状态已变化，请刷新后重新预览")
var ErrUnsafe = errors.New("维护请求未通过安全检查")
var UUID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var SHA = regexp.MustCompile(`^[a-f0-9]{40}$`)
var Digest = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Policy struct {
	BackupEnabled   bool    `json:"backupEnabled"`
	BackupCron      string  `json:"backupCron"`
	KeepDays        int     `json:"keepDays"`
	KeepCount       int     `json:"keepCount"`
	SampleDays      int     `json:"sampleDays"`
	AlertsEnabled   bool    `json:"alertsEnabled"`
	MemoryPercent   float64 `json:"memoryPercent"`
	DiskFreeGB      float64 `json:"diskFreeGB"`
	LoadPerCPU      float64 `json:"loadPerCPU"`
	FailedJobs      int     `json:"failedJobs"`
	Consecutive     int     `json:"consecutive"`
	CooldownMinutes int     `json:"cooldownMinutes"`
}

func Defaults() Policy {
	return Policy{BackupCron: "0 3 * * *", KeepDays: 14, KeepCount: 10, SampleDays: 14, AlertsEnabled: true, MemoryPercent: 90, DiskFreeGB: 5, LoadPerCPU: 2, FailedJobs: 3, Consecutive: 3, CooldownMinutes: 60}
}

type PolicyRecord struct {
	Value      Policy    `json:"value"`
	Revision   int64     `json:"revision"`
	NextBackup time.Time `json:"nextBackupAt"`
}

func (p Policy) Validate() error {
	if _, err := scheduling.Next(p.BackupCron, time.Now(), time.Hour); err != nil {
		return err
	}
	if p.KeepDays < 1 || p.KeepDays > 365 || p.KeepCount < 2 || p.KeepCount > 100 || p.SampleDays < 1 || p.SampleDays > 90 || p.MemoryPercent < 1 || p.MemoryPercent > 100 || p.DiskFreeGB < 0 || p.DiskFreeGB > 100000 || p.LoadPerCPU <= 0 || p.LoadPerCPU > 100 || p.FailedJobs < 1 || p.FailedJobs > 100000 || p.Consecutive < 1 || p.Consecutive > 60 || p.CooldownMinutes < 5 || p.CooldownMinutes > 10080 {
		return ErrUnsafe
	}
	return nil
}

type Engine struct {
	DB                                             *pgxpool.Pool
	Redis                                          *redis.Client
	DSN, Root, OutputRoot, Key, HostProc, Revision string
	Run                                            Command
	prepareRestore                                 func(context.Context, WebRestore, Manifest, string) (string, func() error, error)
	validateRestore                                func(context.Context, Manifest, string) error
}
type Backup struct {
	ID          string          `json:"id"`
	State       string          `json:"state"`
	Origin      string          `json:"origin"`
	Pinned      bool            `json:"pinned"`
	Bytes       int64           `json:"bytes"`
	Digest      string          `json:"digest"`
	Manifest    json.RawMessage `json:"manifest"`
	Message     string          `json:"message"`
	CreatedAt   time.Time       `json:"createdAt"`
	CompletedAt *time.Time      `json:"completedAt"`
}
type Drill struct {
	ID          string          `json:"id"`
	BackupID    string          `json:"backupId"`
	State       string          `json:"state"`
	Report      json.RawMessage `json:"report"`
	Message     string          `json:"message"`
	CreatedAt   time.Time       `json:"createdAt"`
	CompletedAt *time.Time      `json:"completedAt"`
}

func (e Engine) Policy(ctx context.Context) (PolicyRecord, error) {
	r := PolicyRecord{Value: Defaults()}
	var raw []byte
	err := e.DB.QueryRow(ctx, `SELECT value,revision,next_backup_at FROM ops_policy WHERE singleton`).Scan(&raw, &r.Revision, &r.NextBackup)
	if err == nil {
		err = json.Unmarshal(raw, &r.Value)
	}
	return r, err
}
func (e Engine) SavePolicy(ctx context.Context, r PolicyRecord) (PolicyRecord, error) {
	if err := r.Value.Validate(); err != nil {
		return r, err
	}
	next, _ := scheduling.Next(r.Value.BackupCron, time.Now(), time.Hour)
	raw, _ := json.Marshal(r.Value)
	tag, err := e.DB.Exec(ctx, `UPDATE ops_policy SET value=$1,revision=revision+1,next_backup_at=$3,updated_at=now() WHERE singleton AND revision=$2`, raw, r.Revision, next)
	if err != nil {
		return r, err
	}
	if tag.RowsAffected() != 1 {
		return r, ErrConflict
	}
	return e.Policy(ctx)
}
func hash(raw []byte) string { s := sha256.Sum256(raw); return hex.EncodeToString(s[:]) }
func (e Engine) dir(id string) (string, error) {
	if !UUID.MatchString(id) || e.Root == "" || !filepath.IsAbs(e.Root) {
		return "", ErrUnsafe
	}
	return filepath.Join(e.Root, id), nil
}
func privateDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrUnsafe
	}
	return os.Chmod(path, 0700)
}
func (e Engine) QueueBackup(ctx context.Context, origin string) (string, error) {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer deferutil.Rollback(ctx, tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(91220880)`); err != nil {
		return "", err
	}
	if strings.HasPrefix(origin, "cron:") {
		var existing string
		err := tx.QueryRow(ctx, `SELECT id::text FROM ops_backups WHERE origin=$1 ORDER BY created_at LIMIT 1`, origin).Scan(&existing)
		if err == nil {
			return existing, tx.Commit(ctx)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ops_backups WHERE state IN('pending','running')) OR EXISTS(SELECT 1 FROM ops_drills WHERE state IN('pending','running')) OR EXISTS(SELECT 1 FROM ops_web_restores WHERE state IN('pending','running','unknown')) OR (SELECT paused FROM ops_mode WHERE singleton)`).Scan(&active); err != nil {
		return "", err
	}
	if active {
		return "", ErrConflict
	}
	id := jobs.UUID()
	job := jobs.UUID()
	payload, _ := json.Marshal(map[string]string{"id": id})
	if _, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,payload,max_attempts,queue)VALUES($1,'ops.backup',$2,1,'maintenance')`, job, payload); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ops_backups(id,origin,job_id)VALUES($1,$2,$3)`, id, origin, job); err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}
func (e Engine) QueueDrill(ctx context.Context, id string) (string, error) {
	if !UUID.MatchString(id) {
		return "", ErrUnsafe
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer deferutil.Rollback(ctx, tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(91220880)`); err != nil {
		return "", err
	}
	var state string
	if err = tx.QueryRow(ctx, `SELECT state FROM ops_backups WHERE id=$1 FOR UPDATE`, id).Scan(&state); err != nil || state != "ready" {
		return "", ErrUnsafe
	}
	var active bool
	if checkedErr := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ops_drills WHERE state IN('pending','running')) OR EXISTS(SELECT 1 FROM ops_backups WHERE state IN('pending','running')) OR EXISTS(SELECT 1 FROM ops_web_restores WHERE state IN('pending','running','unknown')) OR (SELECT paused FROM ops_mode WHERE singleton)`).Scan(&active); checkedErr != nil {
		return "", checkedErr
	}
	if active {
		return "", ErrConflict
	}
	drill := jobs.UUID()
	job := jobs.UUID()
	payload, _ := json.Marshal(map[string]string{"id": drill})
	if _, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,payload,max_attempts,queue)VALUES($1,'ops.drill',$2,1,'maintenance')`, job, payload); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ops_drills(id,backup_id,job_id)VALUES($1,$2,$3)`, drill, id, job); err != nil {
		return "", err
	}
	return drill, tx.Commit(ctx)
}
