// site-compat-audit reads local immutable reference configs, never accounts,
// provider URLs, cookies or databases. Its report proves static loading only.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"syscall"

	"media-tool/internal/modules/sites"
)

type row struct {
	ID         string   `json:"id"`
	State      string   `json:"state"`
	Issues     int      `json:"issueCount"`
	IssueCodes []string `json:"issueCodes,omitempty"`
}
type report struct {
	SourceCommit            string         `json:"sourceCommit"`
	Definitions             int            `json:"definitions"`
	Loaded                  int            `json:"loaded"`
	ParserSupported         int            `json:"parserSupported"`
	Partial                 int            `json:"partial"`
	Invalid                 int            `json:"invalid"`
	StaticConfigurationOnly bool           `json:"staticConfigurationOnly"`
	AccountVerified         bool           `json:"accountVerified"`
	Rows                    []row          `json:"rows"`
	IssueDefinitionCounts   map[string]int `json:"issueDefinitionCounts"`
}

func main() {
	directory := flag.String("directory", "", "explicit local site_config directory")
	commit := flag.String("source-commit", "", "immutable upstream 40-character commit")
	flag.Parse()
	result, err := audit(*directory, *commit)
	if err != nil {
		stage := "configuration"
		var failure *auditFailure
		if errors.As(err, &failure) {
			stage = failure.stage
		}
		reason := "unavailable"
		if errors.Is(err, os.ErrPermission) {
			reason = "permission"
		} else if errors.Is(err, os.ErrNotExist) {
			reason = "missing"
		} else if errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.EOPNOTSUPP) {
			reason = "platform_unsupported"
		}
		fmt.Fprintf(os.Stderr, "Site configuration audit failed (%s/%s); paths and definitions omitted.\n", stage, reason)
		os.Exit(1)
	}
	if json.NewEncoder(os.Stdout).Encode(result) != nil {
		os.Exit(1)
	}
}

type auditFailure struct {
	stage string
	cause error
}

func (failure *auditFailure) Error() string { return "site audit stage unavailable" }
func (failure *auditFailure) Unwrap() error { return failure.cause }

func audit(directory, commit string) (report, error) {
	result := report{SourceCommit: commit, StaticConfigurationOnly: true, Rows: []row{}, IssueDefinitionCounts: map[string]int{}}
	if directory == "" || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(commit) {
		return result, errors.New("explicit directory and fixed commit required")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return result, &auditFailure{"directory_open", err}
	}
	defer root.Close()
	read := func(path string) ([]byte, error) {
		file, err := root.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		raw, err := io.ReadAll(io.LimitReader(file, (2<<20)+1))
		if err != nil || len(raw) > 2<<20 {
			return nil, errors.New("configuration exceeds limit")
		}
		return raw, nil
	}
	common, err := read("commons/NexusPHP.json")
	if err != nil {
		return result, &auditFailure{"common_read", err}
	}
	entries, err := os.ReadDir(filepath.Join(directory, "sites"))
	if err != nil {
		return result, &auditFailure{"sites_list", err}
	}
	idPattern := regexp.MustCompile(`^[a-z0-9_]{1,80}$`)
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := entry.Name()[:len(entry.Name())-len(".json")]
		if !idPattern.MatchString(id) {
			return result, &auditFailure{"filename_validation", errors.New("invalid configuration filename")}
		}
		result.Definitions++
		raw, err := read("sites/" + entry.Name())
		if err != nil {
			return result, &auditFailure{"definition_read", err}
		}
		config, err := sites.Decode(raw, common)
		if err != nil || config.ID != id {
			result.Invalid++
			result.Rows = append(result.Rows, row{ID: id, State: "invalid", Issues: 1, IssueCodes: []string{"invalid_definition"}})
			result.IssueDefinitionCounts["invalid_definition"]++
			continue
		}
		result.Loaded++
		compatibility := sites.Compatibility(config)
		state, _ := compatibility["state"].(string)
		notes, _ := compatibility["notes"].([]string)
		if state == "parser_supported" {
			result.ParserSupported++
		} else {
			result.Partial++
		}
		codes, _ := compatibility["issueCodes"].([]string)
		for _, code := range codes {
			result.IssueDefinitionCounts[code]++
		}
		result.Rows = append(result.Rows, row{ID: id, State: state, Issues: len(notes), IssueCodes: codes})
	}
	return result, nil
}
