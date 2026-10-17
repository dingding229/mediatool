package embyapi

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
)

type discoveryStub struct {
	result nativelibrary.DiscoveryResult
	seen   nativelibrary.DiscoveryQuery
	err    error
}

func (d *discoveryStub) Discover(_ context.Context, _ contracts.User, _ contracts.PermissionChecker, q nativelibrary.DiscoveryQuery) (nativelibrary.DiscoveryResult, error) {
	d.seen = q
	return d.result, d.err
}
func TestDiscoveryHTTPGoldensAndHEAD(t *testing.T) {
	movie, err := dto.NewBaseItemDto("100001", "Synthetic Movie", dto.Movie)
	if err != nil {
		t.Fatal(err)
	}
	movie.ServerId = strings.Repeat("a", 32)
	episode, err := dto.NewBaseItemDto("100004", "Synthetic Episode", dto.Episode)
	if err != nil {
		t.Fatal(err)
	}
	episode.SeriesId = "100002"
	episode.SeasonId = "100003"
	episode.SeriesName = "Synthetic Show"
	season, err := dto.NewBaseItemDto("100003", "特别篇", dto.Season)
	if err != nil {
		t.Fatal(err)
	}
	zero := 0
	season.IndexNumber = &zero
	season.SeriesId = "100002"
	for _, tc := range []struct {
		name, path string
		item       dto.BaseItemDto
	}{{"seasons", "/Shows/100002/Seasons", season}, {"episodes", "/Shows/100002/Episodes", episode}, {"latest", "/Items/Latest", movie}, {"hints", "/Search/Hints?SearchTerm=Synthetic", movie}, {"genres", "/Genres", movie}, {"filters", "/Items/Filters", movie}, {"similar", "/Items/100001/Similar", movie}} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &discoveryStub{result: nativelibrary.DiscoveryResult{Items: []dto.BaseItemDto{tc.item}, Total: 1, Hints: dto.SearchHintResult{SearchHints: dto.Array[dto.SearchHint]{{Id: "100004", ItemId: "100004", Name: "Synthetic Episode", Type: dto.Episode, Series: "Synthetic Show", SeriesId: "100002", PrimaryImageTag: "synthetic-tag"}}, TotalRecordCount: 1}, Genres: []dto.NameIdPair{{Name: "Drama", Id: "genre:87da8bbbea6db324c6f7da5ab1312dbded82b2e944d50f2a7781a21b433f4aa8"}}, Filters: dto.ItemFilters{Genres: dto.Array[string]{"Drama"}, Years: dto.Array[int]{2025}, OfficialRatings: dto.Array[dto.NameValuePair]{{Name: "G", Value: "G"}}}}}
			s := browseUnitServer(t, &browseStub{})
			s.options.Discovery = stub
			w := browseUnitCall(s, "GET", "/emby"+tc.path)
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			var document any
			decoder := json.NewDecoder(bytes.NewReader(w.Body.Bytes()))
			decoder.UseNumber()
			if err := decoder.Decode(&document); err != nil {
				t.Fatal(err)
			}
			checkBrowseHTTPShape(t, document, "")
			raw, err := json.MarshalIndent(document, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			raw = append(raw, '\n')
			path := filepath.Join("testdata", "discovery_http", tc.name+".json")
			if *updateBrowseGolden {
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, raw, 0644); err != nil {
					t.Fatal(err)
				}
			}
			expected, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(raw, expected) {
				t.Fatal("discovery golden mismatch", path)
			}
			if head := browseUnitCall(s, "HEAD", tc.path); head.Code != 200 || head.Body.Len() != 0 {
				t.Fatal("HEAD", head.Code)
			}
		})
	}
}
func TestDiscoveryHTTPParameterContractsAndEmptyArrays(t *testing.T) {
	stub := &discoveryStub{}
	s := browseUnitServer(t, &browseStub{})
	s.options.Discovery = stub
	for _, path := range []string{"/Items/Latest", "/Users/" + strings.Repeat("a", 32) + "/Items/Latest", "/Search/Hints?SearchTerm=abc", "/Items/100001/Similar"} {
		w := browseUnitCall(s, "GET", path)
		if w.Code != 200 || stub.seen.Items.Limit != 20 {
			t.Fatal(path, w.Code, stub.seen)
		}
	}
	w := browseUnitCall(s, "GET", "/mediabrowser//sHoWs/100002/EpIsOdEs?SeAsOnId=100003&Season=0&StartItemId=100004&StartIndex=2&Limit=5&Fields=Overview&IsMissing=1")
	if w.Code != 200 || stub.seen.Items.Limit != 5 || stub.seen.Items.StartIndex != 2 || stub.seen.Season == nil || *stub.seen.Season != 0 || stub.seen.SeasonID != "100003" || stub.seen.StartItemID != "100004" {
		t.Fatal(w.Code, stub.seen)
	}
	for _, path := range []string{"/Shows/1/Episodes?Season=-1", "/Shows/1/Episodes?Season=10001", "/Shows/1/Episodes?IsMissing=x", "/Shows/1/Episodes?StartItemId=%2Fetc", "/Items/Latest?GroupItems=x", "/Items/Latest?Limit=101", "/Search/Hints?Limit=101&SearchTerm=a", "/Items/1/Similar?Limit=21"} {
		if w := browseUnitCall(s, "GET", path); w.Code != 400 {
			t.Fatal(path, w.Code)
		}
	}
	for _, path := range []string{"/Items/Latest", "/Search/Hints?SearchTerm=a", "/Genres", "/Items/Filters"} {
		w := browseUnitCall(s, "GET", path)
		if w.Code != 200 || strings.Contains(w.Body.String(), "null") {
			t.Fatal("empty discovery arrays", path, w.Body.String())
		}
	}
	stub.err = nativelibrary.ErrDiscoveryTooBroad
	w = browseUnitCall(s, "GET", "/Search/Hints?SearchTerm=a")
	if w.Code != 400 || strings.Contains(w.Body.String(), "candidate limit") {
		t.Fatal("cap error mapping", w.Code, w.Body.String())
	}
}
