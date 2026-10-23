package tooling

import (
	"errors"
	"fmt"
	"media-tool/internal/modules/updatecheck"
	"media-tool/internal/platform/buildinfo"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func releaseNotesCommand(root string, args []string) error {
	if len(args) != 4 {
		return errors.New("release-notes VERSION DIGEST COMMIT SCHEMA")
	}
	version := strings.TrimPrefix(args[0], "v")
	schema, e := strconv.ParseInt(args[3], 10, 64)
	if e != nil || schema != buildinfo.Schema {
		return errors.New("release schema differs from executable")
	}
	b, e := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if e != nil {
		return e
	}
	title := "## [" + version + "]"
	text := string(b)
	i := strings.Index(text, title)
	if i < 0 {
		return errors.New("CHANGELOG version section missing")
	}
	section := text[i:]
	if end := strings.Index(section[3:], "\n## ["); end >= 0 {
		section = section[:end+3]
	}
	body, e := updatecheck.ReleaseBody(section, updatecheck.Metadata{Version: version, Digest: args[1], Commit: args[2], Schema: schema, MinimumSchema: buildinfo.MinimumSchema, Repository: updatecheck.Repository, Platform: "linux/amd64"})
	if e != nil {
		return e
	}
	_, e = fmt.Fprint(os.Stdout, body)
	return e
}
