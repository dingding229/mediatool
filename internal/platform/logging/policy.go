package logging

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"time"
)

// Policy is persisted with application settings. A past/zero deadline disables debug.
type Policy struct {
	RetentionDays int       `json:"retentionDays"`
	DebugUntil    time.Time `json:"debugUntil"`
}

func DefaultPolicy() Policy { return Policy{RetentionDays: 7} }
func (p Policy) Validate() error {
	if p.RetentionDays < 1 || p.RetentionDays > 90 {
		return errors.New("log retention must be 1–90 days")
	}
	return nil
}
func (p Policy) DebugActive(now time.Time) bool { return p.DebugUntil.After(now) }
func (s *Store) ApplyPolicy(p Policy, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policy = p
	s.Level.Set(slog.LevelInfo)
	if p.DebugActive(now) {
		s.Level.Set(slog.LevelDebug)
	}
}

// Prune rewrites at most one bounded log file and removes at most batch records
// per invocation. os.Root keeps all operations inside the private log directory.
// Non-log/malformed lines are retained, never inferred to be expired.
func (s *Store) Prune(ctx context.Context, cutoff time.Time, batch int) (removed int, result error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || batch < 1 {
		return 0, errors.New("invalid log retention operation")
	}
	for _, name := range s.names() {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		info, err := s.root.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return 0, errors.New("unsafe log retention source")
		}
		in, err := s.root.Open(name)
		if err != nil {
			return 0, err
		}
		n, e := s.pruneFile(ctx, name, in, cutoff, batch)
		closeErr := in.Close()
		if e != nil || closeErr != nil {
			return n, errors.Join(e, closeErr)
		}
		if n > 0 {
			return n, nil
		}
	}
	return 0, nil
}
func (s *Store) pruneFile(ctx context.Context, name string, in *os.File, cutoff time.Time, batch int) (n int, result error) {
	const temporary = "retention.tmp"
	// O_EXCL refuses pre-existing links and unfinished rewrites; these are not logs.
	out, err := s.root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return 0, err
	}
	defer func() {
		result = errors.Join(result, out.Close())
		if result != nil || n == 0 {
			_ = s.root.Remove(temporary)
		}
	}()
	scan := bufio.NewReaderSize(in, 64<<10)
	for {
		if err = ctx.Err(); err != nil {
			return n, err
		}
		line, e := scan.ReadBytes('\n')
		if len(line) > 1<<20 {
			return n, errors.New("oversized log record")
		}
		var stamp struct{ Time time.Time }
		if len(line) > 0 {
			expired := json.Unmarshal(line, &stamp) == nil && !stamp.Time.IsZero() && stamp.Time.Before(cutoff)
			if expired {
				n++
			} else if _, err = out.Write(line); err != nil {
				return n, err
			}
		}
		if e != nil {
			if e != io.EOF {
				return n, e
			}
			break
		}
		if n >= batch {
			if _, err = io.Copy(out, scan); err != nil {
				return n, err
			}
			break
		}
	}
	if n == 0 {
		return 0, nil
	}
	if err = out.Sync(); err != nil {
		return n, err
	}
	if name == "current.jsonl" {
		if err = s.file.Close(); err != nil {
			return n, err
		}
		s.file = nil
		defer func() { result = errors.Join(result, s.openCurrent()) }()
	}
	err = s.root.Rename(temporary, name)
	if err == nil {
		s.queryEpoch.Add(1)
	}
	return n, err
}

// Maintain registers a daily retention sweep, serviced in 1000-record batches
// at one-second intervals. Settings/debug expiry refresh every minute; no SQL
// log deletes or database locks, and a failed settings read disables debug.
func (s *Store) Maintain(ctx context.Context, load func(context.Context) (Policy, error), logger *slog.Logger) {
	logger.Info("log retention scheduler registered", "module", "logging", "batch", 1000)
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	nextPolicy := time.Time{}
	day := ""
	sweeping := false
	p := DefaultPolicy()
	for {
		now := time.Now()
		if !now.Before(nextPolicy) {
			v, err := load(ctx)
			if err != nil {
				v = DefaultPolicy()
				logger.Warn("日志设置读取失败，Debug 已关闭", "module", "logging")
			}
			p = v
			s.ApplyPolicy(p, now)
			nextPolicy = now.Add(time.Minute)
		}
		s.mu.Lock()
		if !s.policy.DebugActive(now) {
			s.Level.Set(slog.LevelInfo)
		}
		s.mu.Unlock()
		if today := now.UTC().Format("2006-01-02"); day != today {
			day = today
			sweeping = true
		}
		if sweeping {
			n, err := s.Prune(ctx, now.AddDate(0, 0, -p.RetentionDays), 1000)
			if err != nil {
				logger.Warn("日志保留清理暂缓", "module", "logging")
				sweeping = false
			} else if n == 0 {
				sweeping = false
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}
