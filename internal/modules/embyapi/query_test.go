package embyapi

import (
	"strings"
	"testing"
)

func TestQueryDefaultsBoundsAndCaseInsensitiveAllowLists(t *testing.T) {
	q, err := ParseQuery(values(t, "startindex=2&LIMIT=200&sortby=Name,DateCreated&sortorder=descending&fields=Overview,Unknown&includeitemtypes=movie,Episode&excludeitemtypes=Folder&ids=1,2&parentid=abc_1&recursive=TRUE&isFolder=0"))
	if err != nil {
		t.Fatal(err)
	}
	if q.StartIndex != 2 || q.Limit != 200 || len(q.SortBy) != 2 || len(q.Fields) != 1 || q.Fields[0] != "Overview" || q.IncludeItemTypes[0] != "Movie" || q.ExcludeItemTypes[0] != "Folder" || len(q.IDs) != 2 || q.ParentID != "abc_1" || q.Recursive == nil || !*q.Recursive || q.IsFolder == nil || *q.IsFolder {
		t.Fatalf("query=%+v", q)
	}
	q, err = ParseQuery(values(t, ""))
	if err != nil || q.Limit != 100 || len(q.SortBy) != 1 || q.SortBy[0] != "SortName" || q.SortOrder[0] != "Ascending" {
		t.Fatalf("defaults=%+v err=%v", q, err)
	}
}
func TestQueryRejectsInvalidLimitsSortsTypesIdsAndBooleans(t *testing.T) {
	for _, raw := range []string{"Limit=-1", "Limit=oops", "Limit=9223372036854775808", "StartIndex=-1", "SortOrder=Sideways", "Ids=" + repeatComma(201), "ParentId=bad/id", "Recursive=yes"} {
		if _, err := ParseQuery(values(t, raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
func repeatComma(n int) string {
	out := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			out += ","
		}
		out += "id"
	}
	return out
}

func TestValuesScalarAmbiguityRawBoundsAndDecodeOnce(t *testing.T) {
	for _, raw := range []string{"value=%ZZ", "value=%00", "value=%FF", "=empty-key", "value=x;other=y", strings.Repeat("x", 32769), strings.Repeat("x=1&", 512)} {
		if _, err := ParseValues(raw); err == nil {
			t.Fatalf("invalid query accepted: %.50q", raw)
		}
	}
	v := values(t, "Name=hello%2Bworld&NAME=hello%2Bworld&fields=Overview&FIELDS=Genres&value=%252F")
	got, err := v.Scalar("name")
	if err != nil || got != "hello+world" {
		t.Fatal("scalar case folding", got, err)
	}
	got, err = v.Scalar("value")
	if err != nil || got != "%2F" {
		t.Fatal("double decoding", got, err)
	}
	list, err := v.list("Fields", 64)
	if err != nil || strings.Join(list, ",") != "Overview,Genres" {
		t.Fatal("wire ordering lost", list, err)
	}
	v = values(t, "Limit=100&LIMIT=200")
	if _, err = v.Scalar("limit"); err == nil {
		t.Fatal("ambiguous scalar accepted")
	}
	for _, raw := range []string{"true", "TRUE", "TrUe", "1", "false", "FALSE", "FaLsE", "0"} {
		b, err := values(t, "Static="+raw).Boolean("static")
		if err != nil || b == nil {
			t.Fatalf("valid boolean %q: %v", raw, err)
		}
	}
	for _, raw := range []string{"", "yes", "no", "2"} {
		if _, err := values(t, "flag="+raw).Boolean("flag"); err == nil {
			t.Fatalf("invalid boolean %q", raw)
		}
	}
}
func TestQueryPaginationAndAllBounds(t *testing.T) {
	for _, raw := range []string{"StartIndex=1000001", "StartIndex=9223372036854775808", "Limit=-1", "Limit=9223372036854775808", "Ids=" + strings.Repeat("x", 65), "Ids=%27or%271%27%3D%271", "ParentId=" + strings.Repeat("x", 65), "Fields=" + repeatComma(65)} {
		if _, err := ParseQuery(values(t, raw)); err == nil {
			t.Fatalf("accepted out of bounds query %.100q", raw)
		}
	}
	for _, raw := range []string{"Limit=0", "Limit=200", "StartIndex=1000000", "Ids=" + repeatComma(200)} {
		if _, err := ParseQuery(values(t, raw)); err != nil {
			t.Fatalf("valid bound %q: %v", raw, err)
		}
	}
	q, err := ParseQuery(values(t, "SortBy=Name,DateCreated&SortOrder=Descending,Ascending&Fields=Path,ProviderIds,ProviderIds,Unknown"))
	if err != nil || strings.Join(q.Fields, ",") != "ProviderIds" || len(q.SortOrder) != 2 {
		t.Fatal("canonical fields/orders", q, err)
	}
}

// Request parameters observed in real SenPlayer 6.2.1/6.2.3 browsing logs.
// This is protocol compatibility, not a User-Agent exception.
func TestQueryStandardMixedTypesAndLastContentAdded(t *testing.T) {
	for _, raw := range []string{
		"IncludeItemTypes=Series,Movie,Video,MusicVideo,MusicAlbum&SortBy=DateLastContentAdded,DateCreated,SortName&SortOrder=Descending&Recursive=true&StartIndex=0&Limit=30",
		"includeitemtypes=series,movie,video,musicvideo,musicalbum&sortby=datelastcontentadded,datecreated,sortname&sortorder=descending&recursive=TRUE&startindex=0&limit=30&enableUserData=false",
	} {
		q, err := ParseQuery(values(t, raw))
		if err != nil {
			t.Fatal(raw, err)
		}
		if strings.Join(q.IncludeItemTypes, ",") != "Series,Movie,Video,MusicVideo,MusicAlbum" || strings.Join(q.SortBy, ",") != "DateLastContentAdded,DateCreated,SortName" {
			t.Fatalf("lost filter/sort semantics: %+v", q)
		}
	}
	for _, kind := range []string{"Video", "MusicVideo", "MusicAlbum"} {
		q, err := ParseQuery(values(t, "IncludeItemTypes="+kind+"&ExcludeItemTypes="+kind))
		if err != nil || len(q.IncludeItemTypes) != 1 || q.IncludeItemTypes[0] != kind || len(q.ExcludeItemTypes) != 1 || q.ExcludeItemTypes[0] != kind {
			t.Fatalf("standard type %s: %+v %v", kind, q, err)
		}
	}
}

func TestF2TolerantQuerySortPositionsAndTypes(t *testing.T) {
	for _, tc := range []struct {
		raw, sorts, orders, include, exclude string
		limit                                int
	}{
		{"IncludeItemTypes=BoxSet,Trailer,Audio,Person,Playlist&ExcludeItemTypes=Unknown&Limit=500", "SortName", "Ascending", "BoxSet,Trailer,Audio,Person,Playlist", "Unknown", 200},
		{"SortBy=Unknown&SortOrder=Descending", "SortName", "Ascending", "", "", 100},
		{"SortBy=Name,Unknown,DateCreated&SortOrder=Descending,Ascending,Descending", "Name,DateCreated", "Descending,Descending", "", "", 100},
		{"SortBy=Name,DateCreated,ProductionYear&SortOrder=Descending,Descending", "Name,DateCreated,ProductionYear", "Descending,Descending,Ascending", "", "", 100},
		{"SortBy=Name,DateCreated&SortOrder=Descending", "Name,DateCreated", "Descending,Descending", "", "", 100},
		{"SortBy=Name,,DateCreated&SortOrder=Descending,,Descending", "Name,DateCreated", "Descending,Descending", "", "", 100},
		{"SortBy=Name,DateCreated,ProductionYear&SortOrder=Descending,,Descending", "Name,DateCreated,ProductionYear", "Descending,Ascending,Descending", "", "", 100},
		{"SortBy=Name&SortOrder=", "Name", "Ascending", "", "", 100},
		{"SortBy=Name&SortOrder=Ascending,Descending", "Name", "Ascending", "", "", 100},
		{"SortBy=Name%3BDROP%20TABLE%20items&IncludeItemTypes=Movie,BoxSet&Limit=0", "SortName", "Ascending", "Movie,BoxSet", "", 0},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			q, e := ParseQuery(values(t, tc.raw))
			if e != nil || strings.Join(q.SortBy, ",") != tc.sorts || strings.Join(q.SortOrder, ",") != tc.orders || strings.Join(q.IncludeItemTypes, ",") != tc.include || strings.Join(q.ExcludeItemTypes, ",") != tc.exclude || q.Limit != tc.limit {
				t.Fatalf("%+v %v", q, e)
			}
		})
	}
}
