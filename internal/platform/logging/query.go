package logging

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	deferutil "media-tool/internal/platform/cleanup"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Browsing is bounded independently of retention; total describes this window,
// never an invented count of all rotated logs or Docker stdout.
const LogQueryFileBytes int64 = 2 << 20
const LogQueryMaxRecords = 20000

type Query struct {
	Keyword  string
	Level    string
	Instance string
	Module   string
	From     time.Time
	To       time.Time
	Before   time.Time
	Page     int
	PageSize int
}
type QueryResult struct {
	Items        []map[string]any `json:"items"`
	Total        int              `json:"total"`
	Page         int              `json:"page"`
	PageSize     int              `json:"pageSize"`
	Modules      []string         `json:"modules"`
	Before       time.Time        `json:"before"`
	Truncated    bool             `json:"truncated"`
	MaxFileBytes int64            `json:"maxFileBytes"`
	MaxRecords   int              `json:"maxRecords"`
}

func (q Query) Validate() error {
	if q.Page < 1 || q.Page > 10000 || q.PageSize < 1 || q.PageSize > 200 || utf8.RuneCountInString(q.Keyword) > 200 || len(q.Module) > 120 || len(q.Instance) > 64 {
		return errors.New("invalid log query")
	}
	if q.Level != "" {
		if _, err := ParseLevel(q.Level); err != nil {
			return err
		}
	}
	if !q.From.IsZero() && !q.To.IsZero() && q.From.After(q.To) {
		return errors.New("invalid time range")
	}
	return nil
}
func logModule(row map[string]any) string {
	for _, key := range []string{"module", "component", "provider"} {
		if v, ok := row[key].(string); ok && v != "" && len(v) <= 120 {
			return v
		}
	}
	if source, ok := row["source"].(map[string]any); ok {
		file, _ := source["file"].(string)
		parts := strings.Split(file, "/")
		for i, p := range parts {
			if p == "internal" && len(parts) > i+2 {
				return parts[i+2]
			}
		}
		if strings.HasPrefix(file, "cmd/") {
			return "app"
		}
	}
	if row["msg"] == "HTTP request" {
		return "httpapi"
	}
	if v, ok := row["queue"].(string); ok && v != "" && len(v) < 100 {
		return "worker/" + v
	}
	return "app"
}

type queryFile struct {
	file *os.File
	size int64
}

func (s *Store) queryFiles() ([]queryFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	files := []queryFile{}
	closeAll := func() {
		for _, f := range files {
			deferutil.Close(f.file, "Read/descriptor teardown only; primary operations handle their own I/O result")
		}
	}
	for _, name := range s.names() {
		info, err := s.root.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			closeAll()
			return nil, errors.New("unsafe log source")
		}
		f, err := s.root.Open(name)
		if err != nil {
			closeAll()
			return nil, err
		}
		stat, err := f.Stat()
		if err != nil || !stat.Mode().IsRegular() {
			deferutil.Close(f, "Read/descriptor teardown only; primary operations handle their own I/O result")
			closeAll()
			return nil, errors.New("unsafe log source")
		}
		files = append(files, queryFile{f, stat.Size()})
	}
	return files, nil
}
func (s *Store) Query(ctx context.Context, q Query) (QueryResult, error) {
	out := QueryResult{Items: []map[string]any{}, Modules: []string{}, Page: q.Page, PageSize: q.PageSize, MaxFileBytes: LogQueryFileBytes, MaxRecords: LogQueryMaxRecords}
	if err := q.Validate(); err != nil {
		return out, err
	}
	out.Before = q.Before
	if out.Before.IsZero() {
		out.Before = time.Now().UTC()
	}
	level := q.Level
	if level != "" {
		v, _ := ParseLevel(level)
		level = v.String()
	}
	files, err := s.queryFiles()
	if err != nil {
		return out, err
	}
	defer func() {
		for _, f := range files {
			deferutil.Close(f.file, "Read/descriptor teardown only; primary operations handle their own I/O result")
		}
	}()
	window := []map[string]any{}
	for index, file := range files {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if len(window) >= LogQueryMaxRecords {
			if index < len(files) {
				out.Truncated = true
			}
			break
		}
		offset := max(int64(0), file.size-LogQueryFileBytes)
		if offset > 0 {
			out.Truncated = true
		}
		if _, err = file.file.Seek(offset, io.SeekStart); err != nil {
			return out, err
		}
		scanner := bufio.NewScanner(io.LimitReader(file.file, file.size-offset))
		scanner.Buffer(make([]byte, 4096), 1<<20)
		remaining := LogQueryMaxRecords - len(window)
		records := make([]map[string]any, 0, min(remaining, 256))
		next := 0
		first := true
		for scanner.Scan() {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			if first && offset > 0 {
				first = false
				continue
			}
			first = false
			var row map[string]any
			if json.Unmarshal(scanner.Bytes(), &row) != nil || row == nil {
				continue
			}
			rawTime, _ := row["time"].(string)
			at, err := time.Parse(time.RFC3339Nano, rawTime)
			if err != nil || at.After(out.Before) {
				continue
			}
			row["module"] = logModule(row)
			if len(records) < remaining {
				records = append(records, row)
			} else {
				records[next] = row
				next = (next + 1) % remaining
				out.Truncated = true
			}
		}
		if err := scanner.Err(); err != nil {
			return out, err
		}
		// Circular buffers retain the newest records, then enumerate newest first.
		if len(records) == remaining && next > 0 {
			records = append(records[next:], records[:next]...)
		}
		for i := len(records) - 1; i >= 0; i-- {
			window = append(window, records[i])
		}
	}
	modules := map[string]bool{}
	keyword := strings.ToLower(strings.TrimSpace(q.Keyword))
	start := (q.Page - 1) * q.PageSize
	for _, row := range window {
		module := fmt.Sprint(row["module"])
		modules[module] = true
		if level != "" && !strings.EqualFold(fmt.Sprint(row["level"]), level) {
			continue
		}
		if q.Instance != "" && fmt.Sprint(row["instance"]) != q.Instance {
			continue
		}
		if q.Module != "" && module != q.Module {
			continue
		}
		rawTime, _ := row["time"].(string)
		at, _ := time.Parse(time.RFC3339Nano, rawTime)
		if (!q.From.IsZero() && at.Before(q.From)) || (!q.To.IsZero() && at.After(q.To)) {
			continue
		}
		if keyword != "" {
			raw, _ := json.Marshal(row)
			if !strings.Contains(strings.ToLower(string(raw)), keyword) {
				continue
			}
		}
		if out.Total >= start && len(out.Items) < q.PageSize {
			out.Items = append(out.Items, row)
		}
		out.Total++
	}
	for m := range modules {
		out.Modules = append(out.Modules, m)
	}
	sort.Strings(out.Modules)
	return out, nil
}
