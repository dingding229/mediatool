package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/nativelibrary"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNativeDetailsParsesMoviePeopleRatingAndRegionalFallback(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/movie/9001" {
			t.Fatalf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":9001,"title":"合成电影","original_title":"Synthetic Movie","release_date":"2026-01-02","overview":"overview","poster_path":"/poster.jpg","backdrop_path":"/backdrop.jpg","vote_average":8.5,"runtime":123,"genres":[{"id":18,"name":"Drama"}],"external_ids":{"imdb_id":"tt9001"},"release_dates":{"results":[{"iso_3166_1":"CN","release_dates":[{"certification":""}]},{"iso_3166_1":"US","release_dates":[{"certification":"PG-13"}]}]},"credits":{"cast":[{"id":1,"name":"Actor One","character":"Lead","profile_path":"/actor.jpg"}],"crew":[{"id":2,"name":"Director One","job":"Director","department":"Directing"}]},"images":{"posters":[{"file_path":"/poster.jpg"}],"backdrops":[{"file_path":"/backdrop.jpg"}],"logos":[{"file_path":"/logo.png"}]}}`))
	}))
	defer server.Close()
	client := Client{Token: "synthetic-token", Language: "zh-CN", HTTP: server.Client(), BaseURL: server.URL}
	got, err := client.NativeDetails(context.Background(), nativelibrary.TMDBTypeMovie, 9001, nil, "CN")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "合成电影" || got.PremiereDate != "2026-01-02" || got.CommunityRating == nil || *got.CommunityRating != 8.5 || got.OfficialRating != "PG-13" || got.RuntimeMinutes != 123 || got.ImdbID != "tt9001" || len(got.People) != 2 || got.LogoPath != "/logo.png" {
		t.Fatalf("%+v", got)
	}
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}
func TestNativeDetailsParsesSeasonAndRejectsIdentityOrUnsafePayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":7001,"season_number":1,"name":"Season One","overview":"season","episodes":[{"id":71,"season_number":1,"episode_number":1,"name":"Episode One","overview":"ep","air_date":"2026-01-03","still_path":"/still.jpg","runtime":45}]}`))
	}))
	defer server.Close()
	client := Client{HTTP: server.Client(), BaseURL: server.URL}
	season := 1
	got, err := client.NativeDetails(context.Background(), nativelibrary.TMDBTypeSeason, 7001, &season, "CN")
	if err != nil || len(got.Episodes) != 1 || got.Episodes[0].StillPath != "/still.jpg" {
		t.Fatal(got, err)
	}
	bad := 2
	if _, err = client.NativeDetails(context.Background(), nativelibrary.TMDBTypeSeason, 7001, &bad, "CN"); err == nil {
		t.Fatal("wrong season accepted")
	}
}
func TestNativeDetails429RetriesWithExponentialBackoffAndTimeout(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":9002,"title":"ok"}`))
	}))
	defer server.Close()
	client := Client{HTTP: server.Client(), BaseURL: server.URL}
	start := time.Now()
	_, err := client.NativeDetails(context.Background(), nativelibrary.TMDBTypeMovie, 9002, nil, "CN")
	if err != nil || calls.Load() != 3 {
		t.Fatal(err, calls.Load())
	}
	if time.Since(start) < 2*time.Second {
		t.Fatal("429 was not backed off")
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(200 * time.Millisecond) }))
	defer slow.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = (Client{HTTP: slow.Client(), BaseURL: slow.URL}).NativeDetails(ctx, nativelibrary.TMDBTypeMovie, 9003, nil, "CN")
	if !errors.Is(err, ErrRequest) {
		t.Fatal(err)
	}
}
func TestNativeDetailsPayloadAndImageBounds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":9004,"title":"x","poster_path":"https://private.invalid/image"}`))
	}))
	defer server.Close()
	_, err := (Client{HTTP: server.Client(), BaseURL: server.URL}).NativeDetails(context.Background(), nativelibrary.TMDBTypeMovie, 9004, nil, "CN")
	if err == nil || !strings.Contains(err.Error(), "TMDB") {
		t.Fatal(err)
	}
	if d := ExponentialBackoff(2, &ResponseError{Status: 429}); d < 4*time.Minute {
		t.Fatal(d)
	}
}
func TestNativeSnapshotRetainsClassificationWithoutChangingLegacyProjection(t *testing.T) {
	for _, tc := range []struct {
		kind           nativelibrary.TMDBDetailType
		body, language string
		countries      []string
	}{
		{nativelibrary.TMDBTypeMovie, `{"id":700,"title":"电影","original_language":"zh","production_countries":[{"iso_3166_1":"TW"}],"origin_country":["TW"],"genres":[{"id":18,"name":"Drama"}]}`, "zh", []string{"TW"}},
		{nativelibrary.TMDBTypeSeries, `{"id":700,"name":"剧集","original_language":"cn","origin_country":["HK"],"genres":[{"id":18,"name":"Drama"}]}`, "cn", []string{"HK"}},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Error("non-read-only provider request")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			c := Client{HTTP: server.Client(), BaseURL: server.URL}
			snapshot, e := c.NativeSnapshot(context.Background(), tc.kind, 700, nil, "CN")
			if e != nil || snapshot.Classification == nil || !snapshot.Classification.Valid() {
				t.Fatal(snapshot, e)
			}
			f := snapshot.Classification
			if f.OriginalLanguage != tc.language || !reflect.DeepEqual(f.Countries, tc.countries) || !reflect.DeepEqual(f.GenreIDs, []int{18}) {
				t.Fatal(f)
			}
			raw, _ := json.Marshal(snapshot)
			if !strings.Contains(string(raw), `"classification"`) {
				t.Fatal("cache dropped classification facts")
			}
			legacy, e := c.NativeDetails(context.Background(), tc.kind, 700, nil, "CN")
			if e != nil {
				t.Fatal(e)
			}
			raw, _ = json.Marshal(legacy)
			if strings.Contains(string(raw), `"classification"`) {
				t.Fatal("legacy/native wire changed")
			}
		})
	}
}

func TestT3FutureScrapesCollectTaglineKeywordsAndEpisodeRating(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/movie/9001" {
			if !strings.Contains(r.URL.Query().Get("append_to_response"), "keywords") {
				t.Error("keywords not requested")
			}
			_, _ = w.Write([]byte(`{"id":9001,"title":"新刮削","tagline":"标语","keywords":{"keywords":[{"name":"离线"}]}}`))
		} else {
			_, _ = w.Write([]byte(`{"id":71,"season_number":1,"name":"第一季","episodes":[{"id":711,"season_number":1,"episode_number":1,"name":"第一集","vote_average":7.2}]}`))
		}
	}))
	defer server.Close()
	client := Client{HTTP: server.Client(), BaseURL: server.URL}
	movie, e := client.NativeDetails(context.Background(), nativelibrary.TMDBTypeMovie, 9001, nil, "CN")
	if e != nil || movie.Tagline != "标语" || len(movie.Tags) != 1 || movie.Tags[0] != "离线" {
		t.Fatal(movie, e)
	}
	n := 1
	season, e := client.NativeDetails(context.Background(), nativelibrary.TMDBTypeSeason, 9002, &n, "CN")
	if e != nil || len(season.Episodes) != 1 || season.Episodes[0].CommunityRating == nil || *season.Episodes[0].CommunityRating != 7.2 {
		t.Fatal(season, e)
	}
}
