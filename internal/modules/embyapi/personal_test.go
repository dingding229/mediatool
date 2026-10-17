package embyapi

import (
	"bytes"
	"context"
	"encoding/json"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

type personalStub struct {
	browseStub
	seen  *nativelibrary.PersonalPatch
	out   dto.UserItemDataDto
	calls int
	err   error
}

func (p *personalStub) PersonalData(_ context.Context, u contracts.User, c contracts.PermissionChecker, id string, v *nativelibrary.PersonalPatch) (dto.UserItemDataDto, error) {
	p.calls++
	if id != "100035" || u.ID != strings.Repeat("a", 32) || c == nil {
		return dto.UserItemDataDto{}, nativelibrary.ErrNotFound
	}
	p.seen = v
	return p.out, p.err
}
func TestPersonalHTTPContractAndValidation(t *testing.T) {
	raw, e := os.ReadFile("testdata/contracts/personal-data.json")
	if e != nil {
		t.Fatal(e)
	}
	var sample struct {
		Request struct {
			Position int64 `json:"PlaybackPositionTicks"`
			Count    int   `json:"PlayCount"`
			Played   bool
			Favorite bool   `json:"IsFavorite"`
			Last     string `json:"LastPlayedDate"`
		}
		Response dto.UserItemDataDto
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e = d.Decode(&sample); e != nil {
		t.Fatal(e)
	}
	p := &personalStub{out: sample.Response}
	s := browseUnitServer(t, &p.browseStub)
	s.options.Browse = p
	call := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("X-Emby-Token", "valid-token")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	body, e := json.Marshal(sample.Request)
	if e != nil {
		t.Fatal(e)
	}
	path := "/EmBy/Users/" + strings.Repeat("a", 32) + "/Items/100035/UserData"
	for _, method := range []string{"GET", "POST"} {
		w := call(method, path, string(body))
		var got dto.UserItemDataDto
		decode := json.NewDecoder(w.Body)
		decode.DisallowUnknownFields()
		if e = decode.Decode(&got); e != nil || w.Code != 200 || !reflect.DeepEqual(got, sample.Response) {
			t.Fatal(w.Code, e, got)
		}
	}
	if p.seen == nil || p.seen.Favorite == nil || *p.seen.Favorite || p.seen.PositionTicks == nil || *p.seen.PositionTicks != 1200000000 || p.seen.LastPlayedAt == nil {
		t.Fatal(p.seen)
	}
	for _, method := range []string{"POST", "DELETE"} {
		for _, route := range []string{"PlayedItems", "FavoriteItems"} {
			w := call(method, "/mediabrowser/Users/"+strings.Repeat("a", 32)+"/"+route+"/100035", "")
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
		}
	}
	for _, body := range []string{`{"PlaybackPositionTicks":-1}`, `{"PlayCount":1.5}`, `{"Played":[]}`, `{"LastPlayedDate":"not-a-date"}`, `{"Played":true,"pLaYeD":false}`} {
		before := p.calls
		w := call("POST", path, body)
		if w.Code != 400 || p.calls != before {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := call("POST", path+"?DatePlayed=2026-10-16T00:00:00Z", `{"LastPlayedDate":"2026-10-17T00:00:00Z"}`); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := call("POST", "/Users/"+strings.Repeat("b", 32)+"/PlayedItems/100035", ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	p.err = nativelibrary.ErrPersonalBatchTooLarge
	if w := call("POST", path, `{"Played":true}`); w.Code != 400 || !strings.Contains(w.Body.String(), "5000") {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestPersonalQueryFlagsAndCopies(t *testing.T) {
	v, e := ParseValues("IsPlayed=false&IsFavorite=true&IsResumable=true&SortBy=DatePlayed,PlayCount&SortOrder=Descending,Ascending")
	if e != nil {
		t.Fatal(e)
	}
	q, e := ParseQuery(v)
	if e != nil {
		t.Fatal(e)
	}
	domain := q.ItemQuery()
	*domain.IsResumable = false
	if !*q.IsResumable {
		t.Fatal("pointer alias")
	}
	for _, raw := range []string{"Filters=IsResumable&IsResumable=false", "IsResumable=invalid"} {
		v, e = ParseValues(raw)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = ParseQuery(v); e == nil {
			t.Fatal(raw)
		}
	}
}
