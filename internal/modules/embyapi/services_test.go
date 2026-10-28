package embyapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativeplayback"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestServicesUseOnlyDatabaseFreeContracts(t *testing.T) {
	services := NewServices()
	if services.Libraries == nil || services.Items == nil || services.UserData == nil || services.Playback == nil || services.Authenticator == nil || services.Permissions == nil {
		t.Fatal("incomplete service set")
	}
	original := true
	query := Query{StartIndex: 3, Limit: 20, SortBy: []string{"Name"}, SortOrder: []string{"Descending"}, Fields: []string{"Overview"}, IDs: []string{"100001"}, IncludeItemTypes: []string{"Movie"}, ExcludeItemTypes: []string{"Episode"}, ParentID: "100000", Recursive: &original}
	converted := query.ItemQuery()
	if converted.StartIndex != query.StartIndex || converted.Limit != query.Limit || converted.ParentID != query.ParentID || !reflect.DeepEqual(converted.SortBy, query.SortBy) || !reflect.DeepEqual(converted.IDs, query.IDs) || converted.Recursive == query.Recursive || *converted.Recursive != true {
		t.Fatalf("query conversion=%+v", converted)
	}
	converted.SortBy[0] = "Changed"
	if query.SortBy[0] == "Changed" {
		t.Fatal("query slices were not cloned")
	}
	if got := DomainErrorStatus(nativeplayback.ErrUADenied); got != 403 {
		t.Fatal(got)
	}
	if got := DomainErrorStatus(contracts.ErrUnauthenticated); got != 401 {
		t.Fatal(got)
	}
	if got := DomainErrorStatus(nativelibrary.ErrNotFound); got != 404 {
		t.Fatal(got)
	}
	if got := DomainErrorStatus(errors.New("internal synthetic")); got != 500 {
		t.Fatal(got)
	}
	if got := DomainErrorStatus(nil); got != 200 {
		t.Fatal(got)
	}
}
func TestEmptyServicesFailClosedThroughContracts(t *testing.T) {
	ctx := context.Background()
	services := NewServices()
	if _, _, err := services.Items.Query(ctx, contracts.User{}, nativelibrary.ItemQuery{}); !errors.Is(err, nativelibrary.ErrNotImplemented) {
		t.Fatal(err)
	}
	if _, _, _, err := services.Authenticator.Authenticate(ctx, "synthetic", contracts.DeviceInfo{}); !errors.Is(err, contracts.ErrNotImplemented) {
		t.Fatal(err)
	}
	if _, err := services.Playback.Resolve(ctx, contracts.User{}, "100001", "source", "Synthetic/1", "203.0.113.7"); !errors.Is(err, nativeplayback.ErrNotImplemented) {
		t.Fatal(err)
	}
}

func TestItemQueryContractMatchesEveryParsedFieldAndOwnsValues(t *testing.T) {
	raw, err := os.ReadFile("testdata/contracts/item-query.json")
	if err != nil {
		t.Fatal(err)
	}
	var sample struct {
		RawQuery string
		Expected nativelibrary.ItemQuery
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&sample); err != nil {
		t.Fatal(err)
	}
	values, err := ParseValues(sample.RawQuery)
	if err != nil {
		t.Fatal(err)
	}
	query, err := ParseQuery(values)
	if err != nil {
		t.Fatal(err)
	}
	parsedType, domainType := reflect.TypeOf(query), reflect.TypeFor[nativelibrary.ItemQuery]()
	if parsedType.NumField() != domainType.NumField() {
		t.Fatal("query field counts diverged")
	}
	for i := 0; i < parsedType.NumField(); i++ {
		source := parsedType.Field(i)
		target, ok := domainType.FieldByName(source.Name)
		if !ok || source.Type != target.Type {
			t.Fatal("domain query missing/mismatched field", source.Name)
		}
	}
	mapped := query.ItemQuery()
	if !reflect.DeepEqual(mapped, sample.Expected) {
		t.Fatalf("query contract mismatch: %+v", mapped)
	}
	result, original := reflect.ValueOf(&mapped).Elem(), reflect.ValueOf(query)
	for i := 0; i < result.NumField(); i++ {
		field, old := result.Field(i), original.FieldByName(result.Type().Field(i).Name)
		switch field.Kind() {
		case reflect.Slice:
			before := old.Index(0).String()
			field.Index(0).SetString("changed")
			if old.Index(0).String() != before {
				t.Fatal("slice alias", result.Type().Field(i).Name)
			}
		case reflect.Pointer:
			if field.Pointer() == old.Pointer() {
				t.Fatal("pointer alias", result.Type().Field(i).Name)
			}
			before := old.Elem().Bool()
			field.Elem().SetBool(!before)
			if old.Elem().Bool() != before {
				t.Fatal("pointer mutation escaped")
			}
		}
	}
	empty := Query{}.ItemQuery()
	if !reflect.DeepEqual(empty, nativelibrary.ItemQuery{}) {
		t.Fatal("missing filters changed meaning")
	}
}
func TestAllDomainErrorsMapToFixedEmbyResponses(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{ErrInvalid, 400}, {contracts.ErrUnauthenticated, 401},
		{nativeplayback.ErrForbidden, 403}, {nativeplayback.ErrUADenied, 403}, {contracts.ErrForbidden, 404},
		{nativeplayback.ErrStreamLimit, 403}, {contracts.ErrStreamLimit, 404},
		{nativeplayback.ErrNotFound, 404}, {nativelibrary.ErrNotFound, 404},
		{nativeplayback.ErrUpstream, 503}, {contracts.ErrUnavailable, 503},
		{nativeplayback.ErrNotImplemented, 501}, {nativelibrary.ErrNotImplemented, 501}, {contracts.ErrNotImplemented, 501},
		{errors.New("synthetic unknown"), 500},
	} {
		err := fmt.Errorf("synthetic-private-value %w", tc.err)
		status := DomainErrorStatus(err)
		if status != tc.status {
			t.Fatalf("%v mapped to %d not %d", tc.err, status, tc.status)
		}
		recorder := httptest.NewRecorder()
		if err := WriteError(recorder, status); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != tc.status || strings.Contains(recorder.Body.String(), "synthetic-private-value") {
			t.Fatal("error details exposed", recorder.Body.String())
		}
	}
}
