package maintenance

import (
	"bytes"
	"context"
	"errors"
	deferutil "media-tool/internal/platform/cleanup"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var restoredSequenceValue = regexp.MustCompile(`(?m)^SELECT pg_catalog\.setval\('((?:[^']|'')+)', ([0-9]+), (?:true|false)\);$`)

// Read only sequence TOC objects into a bounded buffer. Never parse business
// COPY rows as SQL, and never execute archive setval (it could rewind live IDs).
func (e Engine) backupSequenceValues(ctx context.Context, dump string) (map[string]int64, error) {
	f, err := os.OpenFile(dump, os.O_RDONLY|noFollowFlag(), 0)
	if err != nil {
		return nil, err
	}
	defer deferutil.Close(f, "private sequence archive reader")
	var toc bytes.Buffer
	if err = e.pg(ctx, "pg_restore", e.DSN, []string{"--list"}, f, &limitedWriter{Writer: &toc, Left: 16 << 20}); err != nil {
		return nil, err
	}
	if toc.Len() >= 16<<20 {
		return nil, ErrUnsafe
	}
	lines := []string{}
	for _, line := range strings.Split(toc.String(), "\n") {
		v := strings.Fields(line)
		if len(v) == 8 && v[3] == "SEQUENCE" && v[4] == "SET" && v[5] == "public" {
			lines = append(lines, line)
		}
	}
	out := map[string]int64{}
	if len(lines) == 0 {
		return out, nil
	}
	list, err := os.CreateTemp(e.Root, ".restore-sequences-*.list")
	if err != nil {
		return nil, err
	}
	defer func() {
		deferutil.Ignore(os.Remove(list.Name()), "private temporary sequence list after command completion")
	}()
	_, err = list.WriteString(strings.Join(lines, "\n") + "\n")
	err = errors.Join(err, list.Close())
	if err != nil {
		return nil, err
	}
	if _, err = f.Seek(0, 0); err != nil {
		return nil, err
	}
	var sql bytes.Buffer
	if err = e.pg(ctx, "pg_restore", e.DSN, []string{"--file=-", "--data-only", "--use-list=" + list.Name()}, f, &limitedWriter{Writer: &sql, Left: 1 << 20}); err != nil {
		return nil, err
	}
	if sql.Len() >= 1<<20 {
		return nil, ErrUnsafe
	}
	for _, m := range restoredSequenceValue.FindAllStringSubmatch(sql.String(), -1) {
		id := strings.ReplaceAll(m[1], "''", "'")
		if !strings.HasPrefix(id, "public.") {
			return nil, ErrUnsafe
		}
		name := strings.TrimPrefix(id, "public.")
		if strings.HasPrefix(name, `"`) && strings.HasSuffix(name, `"`) {
			name = strings.ReplaceAll(name[1:len(name)-1], `""`, `"`)
		}
		n, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil {
			return nil, ErrUnsafe
		}
		out[name] = n
	}
	if len(out) != len(lines) {
		return nil, errors.New("备份序列对象无法完整核对")
	}
	return out, nil
}
