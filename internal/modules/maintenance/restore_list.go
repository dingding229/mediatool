package maintenance

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
)

// Keep live progress, auth and maintenance policy readable during the atomic
// import. Matching whole TOC identifiers also covers constraints/comments and
// actual index/owned-sequence names (not guessed naming conventions).
func (e Engine) restoreList(ctx context.Context, dump string) (string, func(), error) {
	noop := func() {}
	rows, err := e.DB.Query(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND (table_name LIKE 'ops\_%' ESCAPE '\' OR table_name LIKE 'administrator\_%' ESCAPE '\' OR table_name IN('open115_authorizations','integrations')) AND table_name<>'ops_restore_commit'`)
	if err != nil {
		return "", noop, err
	}
	tables := []string{}
	for rows.Next() {
		var t string
		if err = rows.Scan(&t); err != nil {
			rows.Close()
			return "", noop, err
		}
		tables = append(tables, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", noop, err
	}
	names := map[string]bool{}
	for _, t := range tables {
		names[t] = true
	}
	rows, err = e.DB.Query(ctx, `SELECT indexname FROM pg_indexes WHERE schemaname='public' AND tablename=ANY($1) UNION SELECT sequence_name FROM information_schema.sequences s WHERE sequence_schema='public' AND EXISTS(SELECT 1 FROM information_schema.columns c WHERE c.table_schema='public' AND c.table_name=ANY($1) AND c.column_default LIKE '%'||s.sequence_name||'%')`, tables)
	if err != nil {
		return "", noop, err
	}
	for rows.Next() {
		var n string
		if err = rows.Scan(&n); err != nil {
			rows.Close()
			return "", noop, err
		}
		names[n] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", noop, err
	}
	file, err := os.OpenFile(dump, os.O_RDONLY|noFollowFlag(), 0)
	if err != nil {
		return "", noop, err
	}
	var toc bytes.Buffer
	err = e.pg(ctx, "pg_restore", e.DSN, []string{"--list"}, file, &limitedWriter{Writer: &toc, Left: 16 << 20})
	closeErr := file.Close()
	if err = errors.Join(err, closeErr); err != nil {
		return "", noop, err
	}
	if toc.Len() >= 16<<20 {
		return "", noop, errors.New("恢复对象清单过大，已拒绝")
	}
	var out strings.Builder
	for _, line := range strings.Split(toc.String(), "\n") {
		if strings.HasPrefix(line, ";") {
			out.WriteString(line + "\n")
			continue
		}
		skip := preserveTOCEntry(line, names)
		if !skip {
			out.WriteString(line + "\n")
		}
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

func preserveTOCEntry(line string, names map[string]bool) bool {
	if strings.Contains(line, " SCHEMA - public ") || strings.Contains(line, " COMMENT - SCHEMA public ") {
		return true
	}
	fields := strings.Fields(line)
	for i, field := range fields {
		if field != "public" {
			continue
		}
		// The final field is the owner, not an object identifier. An owner named
		// "integrations" must not accidentally exclude every business object.
		for j := i + 1; j < len(fields)-1 && j <= i+2; j++ {
			if names[strings.SplitN(fields[j], ".", 2)[0]] {
				return true
			}
		}
		return false
	}
	return false
}
