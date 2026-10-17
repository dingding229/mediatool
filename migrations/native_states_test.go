package migrations_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"media-tool/internal/platform/buildinfo"
	"media-tool/migrations"
)

const nativeMigration = "00055_native_emby_schema.sql"

// Read all literals of each Go enum type, not a manually maintained subset.
// Adding a new constant without updating its CHECK (or vice versa) must fail.
func declaredEnum(t *testing.T, relativePath, typeName string) []string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate enum definitions")
	}
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(filepath.Dir(here), "..", relativePath), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var values []string
	for _, declaration := range file.Decls {
		group, ok := declaration.(*ast.GenDecl)
		if !ok || group.Tok != token.CONST {
			continue
		}
		currentType := ""
		for _, spec := range group.Specs {
			value := spec.(*ast.ValueSpec)
			if value.Type != nil {
				ident, ok := value.Type.(*ast.Ident)
				if !ok {
					t.Fatal("enum constants must use their named type")
				}
				currentType = ident.Name
			}
			if currentType != typeName {
				continue
			}
			if len(value.Names) != 1 || len(value.Values) != 1 {
				t.Fatal("persistent enum constants must declare one explicit string literal")
			}
			literal, ok := value.Values[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				t.Fatal("persistent enum constant is not a string literal")
			}
			text, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			if slices.Contains(values, text) {
				t.Fatalf("duplicate %s constant %q", typeName, text)
			}
			values = append(values, text)
		}
	}
	if len(values) == 0 {
		t.Fatalf("no constants declared for %s", typeName)
	}
	slices.Sort(values)
	return values
}

func TestNativeSchemaEnumChecksMatchAllGoConstants(t *testing.T) {
	pattern := regexp.MustCompile(`(?is)CONSTRAINT\s+(\w+)\s+CHECK\s*\(\s*\w+\s+IN\s*\(([^)]*)\)\s*\)`)
	drops := regexp.MustCompile(`(?is)DROP\s+CONSTRAINT\s+(\w+)`)
	literals := regexp.MustCompile(`'([^']*)'`)
	checks := map[string][]string{}
	for _, filename := range []string{nativeMigration, "00056_native_library_granularity.sql", "00057_native_asset_assignment.sql", "00058_native_tmdb_details.sql", "00059_native_probe_attempts.sql", "00060_native_still_attempts.sql", "00061_native_playback_reports.sql"} {
		raw, err := migrations.FS.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		part, _, ok := strings.Cut(string(raw), "-- +goose Down")
		if !ok {
			t.Fatal("Down migration missing", filename)
		}
		for _, match := range drops.FindAllStringSubmatch(part, -1) {
			delete(checks, match[1])
		}
		for _, match := range pattern.FindAllStringSubmatch(part, -1) {
			if _, exists := checks[match[1]]; exists {
				t.Fatalf("duplicate constraint %s", match[1])
			}
			for _, literal := range literals.FindAllStringSubmatch(match[2], -1) {
				checks[match[1]] = append(checks[match[1]], literal[1])
			}
		}
	}
	cases := []struct{ constraint, file, enum string }{
		{"native_item_ids_entity_type_check", "internal/modules/nativelibrary/states.go", "EntityType"},
		{"native_libraries_collection_type_check", "internal/modules/nativelibrary/states.go", "CollectionType"},
		{"native_asset_libraries_source_check", "internal/modules/nativelibrary/states.go", "AssetLibrarySource"},
		{"media_user_legacy_grants_media_type_check", "internal/modules/mediausers/states.go", "LegacyMediaType"},
		{"media_probe_results_state_check", "internal/modules/mediaprobe/states.go", "ProbeState"},
		{"media_probe_results_error_class_check", "internal/modules/mediaprobe/linksource/types.go", "Class"},
		{"media_probe_results_error_reason_check", "internal/modules/mediaprobe/linksource/types.go", "Reason"},
		{"media_stills_state_check", "internal/modules/mediaprobe/states.go", "StillState"},
		{"media_stills_error_class_check", "internal/modules/mediaprobe/linksource/types.go", "Class"},
		{"media_stills_error_reason_check", "internal/modules/mediaprobe/linksource/types.go", "Reason"},
		{"native_playback_sessions_state_check", "internal/modules/mediausers/states.go", "PlaybackSessionState"},
		{"native_server_settings_library_granularity_check", "internal/modules/nativelibrary/states.go", "LibraryGranularity"},
		{"native_tmdb_details_type_check", "internal/modules/nativelibrary/states.go", "TMDBDetailType"},
		{"native_tmdb_details_state_check", "internal/modules/nativelibrary/states.go", "TMDBDetailState"},
	}
	if len(checks) != len(cases) {
		t.Fatalf("enum CHECK count=%d, covered=%d", len(checks), len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.constraint, func(t *testing.T) {
			got := checks[tc.constraint]
			slices.Sort(got)
			want := declaredEnum(t, tc.file, tc.enum)
			if tc.constraint == "media_probe_results_error_reason_check" {
				want = append(want, declaredEnum(t, "internal/modules/mediaprobe/probequeue/model.go", "AttemptReason")...)
				slices.Sort(want)
				if len(slices.Compact(slices.Clone(want))) != len(want) {
					t.Fatal("duplicate error reason across sources")
				}
			}
			if tc.constraint == "media_stills_error_reason_check" {
				want = append(want, declaredEnum(t, "internal/modules/mediaprobe/stillqueue/states.go", "AttemptReason")...)
				slices.Sort(want)
				if len(slices.Compact(slices.Clone(want))) != len(want) {
					t.Fatal("duplicate still error reason")
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("SQL=%v Go=%v", got, want)
			}
		})
	}
}

func TestBuildAndDockerSchemaMatchLatestMigration(t *testing.T) {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var latest int64
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			t.Fatal("migration filename has no numeric prefix", entry.Name())
		}
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		if version > latest {
			latest = version
		}
	}
	if buildinfo.Schema != latest {
		t.Fatalf("build schema=%d latest migration=%d", buildinfo.Schema, latest)
	}
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate Dockerfile")
	}
	dockerfile, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "deploy", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	label := fmt.Sprintf(`org.opencontainers.image.schema="%d"`, latest)
	if !strings.Contains(string(dockerfile), label) {
		t.Fatalf("Dockerfile must declare %s", label)
	}
}
