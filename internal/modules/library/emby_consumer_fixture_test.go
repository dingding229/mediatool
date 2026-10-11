package library

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"media-tool/internal/modules/metadata"
)

type consumerFixtureCase struct {
	Name               string   `json:"name"`
	Season             int      `json:"season"`
	Episodes           []int    `json:"episodes"`
	Output             string   `json:"output"`
	Sidecar            bool     `json:"sidecar"`
	Projected          bool     `json:"projected"`
	Outputs            []string `json:"outputs,omitempty"`
	RepresentativeOnly bool     `json:"representativeOnly"`
}

// Only self-contained, fictional fixtures. No database, provider credentials,
// real media identity or mutation of an existing production output is involved.
func writeConsumerFixtures(t *testing.T, root string) []consumerFixtureCase {
	t.Helper()
	cases := []consumerFixtureCase{
		{Name: "single-nfo", Season: 1, Episodes: []int{2}, Sidecar: true},
		{Name: "continuous-nfo", Season: 1, Episodes: []int{1, 2, 3}, Sidecar: true},
		{Name: "sparse-nfo", Season: 1, Episodes: []int{1, 3}, Sidecar: true},
		{Name: "continuous-name-only", Season: 1, Episodes: []int{1, 2, 3}},
		{Name: "sparse-name-only", Season: 1, Episodes: []int{1, 3}},
		{Name: "special-single-nfo", Season: 0, Episodes: []int{1}, Sidecar: true},
		{Name: "special-sparse-nfo", Season: 0, Episodes: []int{1, 3}, Sidecar: true},
		{Name: "sparse-projected", Season: 1, Episodes: []int{1, 3}, Sidecar: true, Projected: true},
		{Name: "special-sparse-projected", Season: 0, Episodes: []int{1, 3}, Sidecar: true, Projected: true},
		{Name: "continuous-projected", Season: 1, Episodes: []int{1, 2, 3}, Sidecar: true, Projected: true},
		{Name: "sparse-representative-candidate", Season: 1, Episodes: []int{1, 3}, Sidecar: true, RepresentativeOnly: true},
	}
	for i := range cases {
		c := &cases[i]
		title := "MediaTool Owned Fixture " + c.Name
		if c.Projected {
			item := metadata.Item{ID: 900000 + i, MediaType: "series", Title: title, Overview: "Owned fictional fixture; no external identity matching."}
			for _, n := range c.Episodes {
				item.EpisodeDetails = append(item.EpisodeDetails, metadata.Episode{ID: 900000 + i*100 + n, SeriesID: item.ID, Season: c.Season, Number: n, Title: "Owned episode " + string(rune('A'+n-1)), Overview: "Fictional owned fixture episode", AirDate: "2026-01-01", Runtime: 1})
			}
			plans, err := AssetOutputProjections("/115/Owned/sample.mp4", "owned-consumer-fixture", item, &c.Season, c.Episodes)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range plans {
				p.Path = strings.TrimPrefix(p.Path, "TV/")
				c.Outputs = append(c.Outputs, p.Path)
				absolute := filepath.Join(root, filepath.FromSlash(p.Path))
				if err := os.MkdirAll(filepath.Dir(absolute), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(absolute, []byte("http://samples:8080/Owned-Test-5s.mp4\n"), 0644); err != nil {
					t.Fatal(err)
				}
				parts, err := nfoProjectionFiles(p.Path, "owned-consumer-fixture", item, &c.Season, p.Episodes, p.SourceEpisodes)
				if err != nil {
					t.Fatal(err)
				}
				for _, part := range parts {
					data := string(part.data)
					start := strings.Index(data, "<uniqueid")
					end := strings.Index(data, "</uniqueid>")
					if start >= 0 && end > start {
						data = data[:start] + data[end+len("</uniqueid>"):]
					}
					if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(part.relative)), []byte(data), 0644); err != nil {
						t.Fatal(err)
					}
				}
			}
			c.Output = c.Outputs[0]
			continue
		}
		tag, err := metadata.EpisodeTag(c.Season, c.Episodes)
		if err != nil {
			t.Fatal(err)
		}
		if c.RepresentativeOnly {
			tag, err = metadata.EpisodeTag(c.Season, c.Episodes[:1])
			if err != nil {
				t.Fatal(err)
			}
			tag += " - Coverage(1,3)"
		}
		c.Output = filepath.ToSlash(filepath.Join(title, "Season "+map[bool]string{true: "00", false: "01"}[c.Season == 0], title+" - "+tag+".strm"))
		absolute := filepath.Join(root, filepath.FromSlash(c.Output))
		if err := os.MkdirAll(filepath.Dir(absolute), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte("http://samples:8080/Owned-Test-5s.mp4\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if !c.Sidecar {
			continue
		}
		item := metadata.Item{ID: 900000 + i, MediaType: "series", Title: title, Overview: "Owned fictional fixture; no external identity matching."}
		for _, n := range c.Episodes {
			item.EpisodeDetails = append(item.EpisodeDetails, metadata.Episode{ID: 900000 + i*100 + n, SeriesID: item.ID, Season: c.Season, Number: n, Title: "Owned episode " + string(rune('A'+n-1)), Overview: "Fictional owned fixture episode", AirDate: "2026-01-01", Runtime: 1})
		}
		parts, err := nfoFilesCoverage(c.Output, "owned-consumer-fixture", item, &c.Season, c.Episodes)
		if err != nil {
			t.Fatal(err)
		}
		for _, part := range parts {
			// Remove fictional provider IDs before actual isolated Emby consumption.
			data := string(part.data)
			start := strings.Index(data, "<uniqueid")
			end := strings.Index(data, "</uniqueid>")
			if start >= 0 && end > start {
				data = data[:start] + data[end+len("</uniqueid>"):]
			}
			p := filepath.Join(root, filepath.FromSlash(part.relative))
			if err := os.WriteFile(p, []byte(data), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	raw, err := json.MarshalIndent(cases, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fixtures.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	return cases
}
func TestEmbyConsumerFixtureGenerationIsOwnedAndBounded(t *testing.T) {
	root := t.TempDir()
	cases := writeConsumerFixtures(t, root)
	if len(cases) != 11 {
		t.Fatal("matrix omitted")
	}
	for _, c := range cases {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(c.Output)))
		if err != nil || string(raw) != "http://samples:8080/Owned-Test-5s.mp4\n" {
			t.Fatal("fixture escaped owned static source")
		}
		if c.Sidecar {
			raw, err = os.ReadFile(filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(c.Output, ".strm")+".nfo")))
			if err != nil || strings.Contains(string(raw), "<uniqueid") {
				t.Fatal("fictional provider ID exported")
			}
		}
	}
}
func TestExportOwnedEmbyConsumerFixture(t *testing.T) {
	root := os.Getenv("OWNED_EMBY_CONSUMER_FIXTURES")
	if root == "" {
		t.Skip("explicit owned fixture export only")
	}
	// The native acceptance runner mounts one pre-owned empty directory here.
	if root != "/owned-fixtures" {
		t.Fatal("fixture export is limited to the explicit container mount")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("refuse nonempty export directory", err)
	}
	writeConsumerFixtures(t, root)
}
