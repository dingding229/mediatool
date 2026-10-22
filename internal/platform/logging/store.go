package logging

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	deferutil "media-tool/internal/platform/cleanup"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const MaxLogFileBytes int64 = 10 << 20
const LogArchives = 4

type Store struct {
	queryMu    sync.Mutex
	queryCache *querySnapshot
	queryEpoch atomic.Uint64

	mu     sync.Mutex
	root   *os.Root
	file   *os.File
	size   int64
	closed bool
	Level  slog.LevelVar
	policy Policy
}
type FileInfo struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}
type Summary struct {
	Files       []FileInfo `json:"files"`
	TotalBytes  int64      `json:"totalBytes"`
	LimitBytes  int64      `json:"limitBytes"`
	Level       string     `json:"level"`
	Policy      Policy     `json:"policy"`
	DebugActive bool       `json:"debugActive"`
}

func Open(dir, level string) (*Store, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, errors.New("log directory unavailable")
	}
	stat, err := os.Lstat(dir)
	if err != nil || stat.Mode()&os.ModeSymlink != 0 || !stat.IsDir() {
		return nil, errors.New("log directory must not be a link")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	s := &Store{root: root, policy: DefaultPolicy()}
	v, err := ParseLevel(level)
	if err != nil {
		deferutil.Close(root, "Read/descriptor teardown only; primary operations handle their own I/O result")
		return nil, err
	}
	s.Level.Set(v)
	if err = s.openCurrent(); err != nil {
		deferutil.Close(root, "Read/descriptor teardown only; primary operations handle their own I/O result")
		return nil, err
	}
	return s, nil
}
func (s *Store) openCurrent() error {
	if info, e := s.root.Lstat("current.jsonl"); e == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return errors.New("unsafe log file")
	}
	f, e := s.root.OpenFile("current.jsonl", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if e != nil {
		return e
	}
	stat, e := f.Stat()
	if e != nil {
		deferutil.Close(f, "Read/descriptor teardown only; primary operations handle their own I/O result")
		return e
	}
	s.file = f
	s.size = stat.Size()
	return nil
}
func (s *Store) names() []string {
	out := []string{"current.jsonl"}
	for i := 1; i <= LogArchives; i++ {
		out = append(out, fmt.Sprintf("archive-%d.jsonl", i))
	}
	return out
}
func (s *Store) rotate() error {
	reopen := func(cause error) error {
		if s.file != nil {
			return cause
		}
		if err := s.openCurrent(); err != nil {
			return errors.Join(cause, err)
		}
		return cause
	}
	if err := s.file.Close(); err != nil {
		s.file = nil
		return reopen(err)
	}
	s.file = nil
	names := s.names()
	for i := len(names) - 1; i >= 1; i-- {
		if info, e := s.root.Lstat(names[i]); e == nil {
			if !info.Mode().IsRegular() {
				return reopen(errors.New("unsafe log archive"))
			}
			if e = s.root.Remove(names[i]); e != nil {
				return reopen(e)
			}
		}
		if _, e := s.root.Lstat(names[i-1]); e == nil {
			if e = s.root.Rename(names[i-1], names[i]); e != nil {
				return reopen(e)
			}
		}
	}
	if err := s.openCurrent(); err != nil {
		return reopen(err)
	}
	return nil
}
func (s *Store) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, errors.New("log closed")
	}
	if s.file == nil {
		if err := s.openCurrent(); err != nil {
			return 0, err
		}
	}
	if len(p) > 1<<20 {
		return 0, errors.New("log record too large")
	}
	if s.size+int64(len(p)) > MaxLogFileBytes {
		if err := s.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := s.file.Write(p)
	s.size += int64(n)
	return n, err
}
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	var err error
	if s.file != nil {
		err = s.file.Close()
		s.file = nil
	}
	return errors.Join(err, s.root.Close())
}
func (s *Store) Stats() (Summary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Summary{Files: []FileInfo{}, LimitBytes: MaxLogFileBytes * (LogArchives + 1), Level: strings.ToLower(s.Level.Level().String()), Policy: s.policy, DebugActive: s.policy.DebugActive(time.Now())}
	for _, name := range s.names() {
		info, err := s.root.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return out, errors.New("log metadata unavailable")
		}
		out.Files = append(out.Files, FileInfo{Name: name, Size: info.Size(), Modified: info.ModTime()})
		out.TotalBytes += info.Size()
	}
	return out, nil
}
func (s *Store) Read(level string, limit int) ([]map[string]any, error) {
	if limit < 1 || limit > 200 {
		return nil, errors.New("invalid log limit")
	}
	if level != "" {
		if _, err := ParseLevel(level); err != nil {
			return nil, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []map[string]any{}
	for _, name := range s.names() {
		f, err := s.root.Open(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		stat, err := f.Stat()
		if err != nil || !stat.Mode().IsRegular() {
			deferutil.Close(f, "Read/descriptor teardown only; primary operations handle their own I/O result")
			return nil, errors.New("unsafe log source")
		}
		offset := max(int64(0), stat.Size()-(2<<20))
		if _, checkedErr := f.Seek(offset, io.SeekStart); checkedErr != nil {
			deferutil.Close(f, "Failed read-only seek; release descriptor before returning the primary error")
			return nil, checkedErr
		}
		scanner := bufio.NewScanner(io.LimitReader(f, 2<<20))
		scanner.Buffer(make([]byte, 4096), 1<<20)
		records := []map[string]any{}
		first := true
		for scanner.Scan() {
			if first && offset > 0 {
				first = false
				continue
			}
			first = false
			var row map[string]any
			if json.Unmarshal(scanner.Bytes(), &row) != nil {
				continue
			}
			if level != "" && !strings.EqualFold(fmt.Sprint(row["level"]), level) {
				continue
			}
			records = append(records, row)
		}
		deferutil.Close(f, "Read/descriptor teardown only; primary operations handle their own I/O result")
		if scanner.Err() != nil {
			return nil, scanner.Err()
		}
		for i := len(records) - 1; i >= 0 && len(out) < limit; i-- {
			out = append(out, records[i])
		}
		if len(out) == limit {
			break
		}
	}
	return out, nil
}
func (s *Store) Clear() (result error) {
	s.queryEpoch.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("log closed")
	}
	// Closing/removing a log is not a permanent close of the Store. Reopen on
	// every exit, including an unsafe archive or a failed removal.
	defer func() {
		if s.file == nil {
			result = errors.Join(result, s.openCurrent())
		}
	}()
	if s.file != nil {
		err := s.file.Close()
		s.file = nil
		if err != nil {
			return err
		}
	}
	for _, name := range s.names() {
		info, err := s.root.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("unsafe log clear target")
		}
		if err = s.root.Remove(name); err != nil {
			return err
		}
	}
	return s.openCurrent()
}
func (s *Store) SetLevel(value string) error {
	v, err := ParseLevel(value)
	if err != nil {
		return err
	}
	s.Level.Set(v)
	return nil
}
