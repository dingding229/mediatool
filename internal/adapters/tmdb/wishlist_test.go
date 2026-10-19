package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicListBoundedLocalPagingAndIdentity(t *testing.T) {
	for _, mode := range []string{"valid", "oversize", "unknown", "uncertain"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer HOST_TOKEN" || r.URL.Path != "/list/42" {
					t.Error("wrong typed request")
				}
				items := []map[string]any{}
				count := 21
				if mode == "oversize" {
					count = 201
				}
				for i := range count {
					kind := "movie"
					if mode == "unknown" {
						kind = "person"
					}
					items = append(items, map[string]any{"id": i + 1, "title": fmt.Sprint("Film", i), "media_type": kind, "release_date": "2026-01-01", "poster_path": "TOKEN_URL_IGNORED"})
				}
				extra := 0
				if mode == "uncertain" {
					extra = 1
				}
				json.NewEncoder(w).Encode(map[string]any{"items": items, "item_count": count + extra})
			}))
			defer server.Close()
			c := Client{Token: "HOST_TOKEN", BaseURL: server.URL}
			items, more, e := c.PublicList(context.Background(), 42, 1)
			if mode != "valid" {
				if e == nil {
					t.Fatal(mode)
				}
				return
			}
			if e != nil || len(items) != 20 || !more {
				t.Fatal(len(items), more, e)
			}
			items, more, e = c.PublicList(context.Background(), 42, 2)
			if e != nil || len(items) != 1 || more {
				t.Fatal(len(items), more, e)
			}
			for _, v := range items {
				if strings.Contains(v.Title, "http") {
					t.Fatal(v)
				}
			}
		})
	}
}
