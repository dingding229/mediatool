package maintenance

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
)

// Only TABLE DATA is imported, in FK order. Never DROP/TRUNCATE live tables:
// ordinary SELECT (including administrator authentication) stays MVCC-readable.
func (e Engine) restoreList(ctx context.Context, dump string, order []string) (string, func(), error) {
	noop := func() {}
	file, err := os.OpenFile(dump, os.O_RDONLY|noFollowFlag(), 0)
	if err != nil {
		return "", noop, err
	}
	var toc bytes.Buffer
	err = e.pg(ctx, "pg_restore", e.DSN, []string{"--list"}, file, &limitedWriter{Writer: &toc, Left: 16 << 20})
	err = errors.Join(err, file.Close())
	if err != nil {
		return "", noop, err
	}
	if toc.Len() >= 16<<20 {
		return "", noop, ErrUnsafe
	}
	data := map[string]string{}
	for _, line := range strings.Split(toc.String(), "\n") {
		f := strings.Fields(line)
		if len(f) == 8 && f[3] == "TABLE" && f[4] == "DATA" && f[5] == "public" {
			data[f[6]] = line
		}
	}
	var out strings.Builder
	for _, name := range order {
		line, ok := data[name]
		if !ok {
			return "", noop, errors.New("候选数据库缺少业务表数据对象：" + name)
		}
		out.WriteString(line + "\n")
	}
	list, err := os.CreateTemp(e.Root, ".restore-*.list")
	if err != nil {
		return "", noop, err
	}
	_, err = io.WriteString(list, out.String())
	err = errors.Join(err, list.Sync(), list.Close())
	clean := func() { _ = os.Remove(list.Name()) }
	if err != nil {
		clean()
		return "", noop, err
	}
	return list.Name(), clean, nil
}

// Kept as an exact identifier parser for metadata-only exports/tests, never an
// owner substring heuristic.
func preserveTOCEntry(line string, names map[string]bool) bool {
	if strings.Contains(line, " SCHEMA - public ") || strings.Contains(line, " COMMENT - SCHEMA public ") {
		return true
	}
	fields := strings.Fields(line)
	for i, f := range fields {
		if f == "public" {
			for j := i + 1; j < len(fields)-1 && j <= i+2; j++ {
				if names[strings.SplitN(fields[j], ".", 2)[0]] {
					return true
				}
			}
			return false
		}
	}
	return false
}
