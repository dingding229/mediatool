package tmdb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestIdentityDetailsCollectsBothAliasShapesAndTranslationsOnce(t *testing.T) {
	for _, kind := range []string{"movie", "series"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				prefix := "movie"
				if kind == "series" {
					prefix = "tv"
				}
				if r.URL.Path != "/"+prefix+"/300146" || r.URL.Query().Get("append_to_response") != "alternative_titles,translations,external_ids" {
					t.Error(r.URL.Path, r.URL.RawQuery)
				}
				if r.Header.Get("Authorization") != "Bearer fixture-only" {
					t.Error("missing provider auth")
				}
				key := "titles"
				if kind == "series" {
					key = "results"
				}
				json.NewEncoder(w).Encode(map[string]any{"id": 300146, "name": "嫁金钗", "title": "嫁金钗", "alternative_titles": map[string]any{key: []any{map[string]any{"title": "Jia Jin Chai"}, map[string]any{"title": "Jia Jin Chai"}, map[string]any{"title": ""}}}, "translations": map[string]any{"translations": []any{map[string]any{"data": map[string]any{"name": "Where the Mask Ends", "title": "Transfer Gold Hairpin"}}}}, "external_ids": map[string]any{"imdb_id": "tt42576855"}})
			}))
			defer server.Close()
			item, err := (Client{BaseURL: server.URL, HTTP: server.Client(), Token: "fixture-only"}).IdentityDetails(context.Background(), kind, 300146)
			if err != nil || calls != 1 || item.IMDBID != "tt42576855" || !reflect.DeepEqual(item.CandidateTitles, []string{"Jia Jin Chai", "Where the Mask Ends", "Transfer Gold Hairpin"}) {
				t.Fatal(item, calls, err)
			}
			raw, _ := json.Marshal(item)
			if !json.Valid(raw) {
				t.Fatal("invalid snapshot")
			}
		})
	}
}
func TestIdentityDetailsRejectsWrongID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"id":2,"name":"Unrelated"}`)) }))
	defer server.Close()
	if _, err := (Client{BaseURL: server.URL, HTTP: server.Client()}).IdentityDetails(context.Background(), "series", 300146); err == nil {
		t.Fatal("wrong identity accepted")
	}
}
