package dto_test

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/embyapi/dto"
)

func TestEmptyArraysAndOptionalZeroAreDifferentFromMissing(t *testing.T) {
	for _, value := range []any{dto.Array[string](nil), dto.Array[dto.MediaStream](nil), dto.QueryResult[dto.BaseItemDto]{}, dto.PlaybackInfoResponse{}, dto.UserPolicy{}, dto.MediaSourceInfo{}} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte("null")) {
			t.Fatalf("nil array escaped: %s", raw)
		}
	}
	value := dto.UserItemDataDto{}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("PlaybackPositionTicks")) {
		t.Fatal("missing position invented a zero")
	}
	value.PlaybackPositionTicks = ptr(int64(0))
	value.PlayCount = ptr(0)
	value.UnplayedItemCount = ptr(0)
	raw, err = json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"PlaybackPositionTicks", "PlayCount", "UnplayedItemCount"} {
		if !bytes.Contains(raw, []byte(`"`+key+`":0`)) {
			t.Fatalf("valid zero omitted: %s", raw)
		}
	}
	item, err := dto.NewBaseItemDto("100001", "Synthetic Special", dto.Episode)
	if err != nil {
		t.Fatal(err)
	}
	item.IndexNumber = ptr(0)
	item.IndexNumberEnd = ptr(0)
	item.ParentIndexNumber = ptr(0)
	item.ChildCount = ptr(0)
	item.RecursiveItemCount = ptr(0)
	raw, err = json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"IndexNumber", "IndexNumberEnd", "ParentIndexNumber", "ChildCount", "RecursiveItemCount"} {
		if !bytes.Contains(raw, []byte(`"`+key+`":0`)) {
			t.Fatalf("valid zero omitted: %s", raw)
		}
	}
}
func TestBaseItemTypesMediaTypeAndFolderFlags(t *testing.T) {
	for _, tc := range []struct {
		kind   dto.ItemType
		folder bool
	}{{dto.CollectionFolder, true}, {dto.Movie, false}, {dto.Series, true}, {dto.Season, true}, {dto.Episode, false}} {
		t.Run(string(tc.kind), func(t *testing.T) {
			value, err := dto.NewBaseItemDto("100001", "Synthetic", tc.kind)
			if err != nil {
				t.Fatal(err)
			}
			if value.Type != tc.kind || value.MediaType != "Video" || value.LocationType != "FileSystem" || value.IsFolder != tc.folder {
				t.Fatalf("bad defaults: %+v", value)
			}
			if _, err := json.Marshal(value); err != nil {
				t.Fatal(err)
			}
			value.IsFolder = !tc.folder
			if _, err := json.Marshal(value); err == nil {
				t.Fatal("inconsistent folder flag serialized")
			}
		})
	}
	if _, err := dto.NewBaseItemDto("100001", "Synthetic", dto.ItemType("Audio")); err == nil {
		t.Fatal("unsupported kind accepted")
	}
	for _, change := range []func(*dto.BaseItemDto){func(b *dto.BaseItemDto) { b.Type = "Music" }, func(b *dto.BaseItemDto) { b.MediaType = "Audio" }, func(b *dto.BaseItemDto) { b.LocationType = "Remote" }} {
		value, err := dto.NewBaseItemDto("100001", "Synthetic", dto.Movie)
		if err != nil {
			t.Fatal(err)
		}
		change(&value)
		if _, err := json.Marshal(value); err == nil {
			t.Fatal("invalid base invariant serialized")
		}
	}
	var item dto.BaseItemDto
	if err := json.Unmarshal([]byte(`{"Id":"100001","Type":"Invalid","MediaType":"Video","LocationType":"FileSystem"}`), &item); err == nil {
		t.Fatal("invalid type decoded")
	}
}
func TestMediaSourceCapabilitiesCannotBeAssignedOrEnabled(t *testing.T) {
	typ := reflect.TypeOf(dto.MediaSourceInfo{})
	for _, field := range []string{"Protocol", "IsRemote", "SupportsDirectPlay", "SupportsDirectStream", "SupportsTranscoding", "RequiresOpening", "RequiresClosing", "TranscodingUrl"} {
		if _, ok := typ.FieldByName(field); ok {
			t.Fatal("writable capability field exists", field)
		}
	}
	source, err := dto.NewMediaSourceInfo("")
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []dto.MediaSourceInfo{{}, source} {
		if value.Protocol() != "Http" || !value.IsRemote() || !value.SupportsDirectPlay() || !value.SupportsDirectStream() || value.SupportsTranscoding() || value.RequiresOpening() || value.RequiresClosing() {
			t.Fatal("capability not fixed")
		}
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"SupportsTranscoding", "RequiresOpening", "RequiresClosing"} {
			if string(fields[key]) != "false" {
				t.Fatalf("%s not false: %s", key, raw)
			}
		}
		if _, ok := fields["TranscodingUrl"]; ok {
			t.Fatal("transcoding URL output")
		}
	}
	for _, payload := range []string{`{"SupportsTranscoding":true}`, `{"supportstranscoding":true}`, `{"Protocol":"File"}`, `{"IsRemote":false}`, `{"SupportsDirectPlay":false}`, `{"SupportsDirectStream":false}`, `{"RequiresOpening":true}`, `{"RequiresClosing":true}`, `{"TranscodingUrl":""}`, `{"TranscodingUrl":"https://synthetic.invalid/transcode"}`, `{"SupportsTranscoding":false,"supportstranscoding":true}`, `{"SupportsTranscoding":null}`} {
		var decoded dto.MediaSourceInfo
		if err := json.Unmarshal([]byte(payload), &decoded); err == nil {
			t.Fatalf("mutable capability decoded: %s", payload)
		}
		if decoded.SupportsTranscoding() {
			t.Fatal("failed decode enabled transcoding")
		}
	}
}
func TestNoCloudResolverOrDirectoryDataCanEnterMediaPaths(t *testing.T) {
	invalid := []string{"/Synthetic.mkv", "folder/Synthetic.mkv", `folder\Synthetic.mkv`, "../Synthetic.mkv", "Movie..mkv", "C:Movie.mkv", "https://synthetic.invalid/file.mkv", "/115/private/film.mkv", "/root/private/film.mkv", "/data/strm/film.strm", "https://synthetic.invalid/api/v1/cloudStorage/strm302?apiKey=SYNTHETIC-GLOBAL&pickCode=SYNTHETIC-PICK&path=/115/film.mkv", "pickCode=SYNTHETIC.mkv", "api_key=SYNTHETIC.mkv", "%2F115%2Ffilm.mkv", "no-extension", ".mkv", "file.", "file.m kv", "movie\n.mkv", strings.Repeat("a", 513) + ".mkv"}
	for _, path := range invalid {
		t.Run(path[:min(len(path), 40)], func(t *testing.T) {
			if _, err := dto.NewMediaSourceInfo(path); err == nil {
				t.Fatalf("unsafe filename accepted: %q", path)
			}
			item, err := dto.NewBaseItemDto("100001", "Synthetic", dto.Movie)
			if err != nil {
				t.Fatal(err)
			}
			item.Path = path
			if raw, err := json.Marshal(item); err == nil {
				t.Fatalf("unsafe base path output: %s", raw)
			}
			source := dto.MediaSourceInfo{Path: path}
			if raw, err := json.Marshal(source); err == nil {
				t.Fatalf("unsafe source path output: %s", raw)
			}
		})
	}
	for _, path := range []string{"", "Synthetic Film.mkv", "Synthetic - S00E00.mp4", "合成影片 (2026).m2ts", "115 Synthetic.mkv"} {
		item, err := dto.NewBaseItemDto("100001", "Synthetic", dto.Movie)
		if err != nil {
			t.Fatal(err)
		}
		item.Path = path
		if _, err := json.Marshal(item); err != nil {
			t.Fatalf("safe filename rejected: %s: %v", path, err)
		}
	}
	for _, payload := range []string{`{"Path":"/115/private.mkv"}`, `{"Name":"pickCode=SYNTHETIC-PICK"}`, `{"Name":"apiKey=SYNTHETIC-GLOBAL"}`} {
		var source dto.MediaSourceInfo
		if err := json.Unmarshal([]byte(payload), &source); err == nil {
			t.Fatalf("sensitive metadata accepted: %s", payload)
		}
	}
	for _, name := range []string{"/custom-cloud/Synthetic.mkv", `\\nas\share\Synthetic.mkv`, "https://synthetic.invalid/private?token=synthetic", "file:/private/Synthetic.mkv", "smb://nas/private/Synthetic.mkv"} {
		source := dto.MediaSourceInfo{Name: name}
		if _, err := json.Marshal(source); err == nil {
			t.Fatal("private location used as source name", name)
		}
	}
	base, err := dto.NewBaseItemDto("100001", "Synthetic", dto.Movie)
	if err != nil {
		t.Fatal(err)
	}
	base.ProviderIds = map[string]string{"pickCode": "SYNTHETIC-PICK"}
	if _, err := json.Marshal(base); err == nil {
		t.Fatal("arbitrary provider metadata exported")
	}
	base.ProviderIds = nil
	base.Overview = "Source: /115/private/film.mkv"
	if _, err := json.Marshal(base); err == nil {
		t.Fatal("private mount leaked through overview")
	}
	base.Overview = ""
	base.MediaSources = dto.Array[dto.MediaSourceInfo]{{Path: "/115/film.mkv"}}
	if _, err := json.Marshal(base); err == nil {
		t.Fatal("nested source bypassed validation")
	}
}
func TestDatesUseExactlySevenUTCDecimals(t *testing.T) {
	value := dto.NewUTCDateTime(time.Date(2026, 10, 16, 8, 9, 10, 123456789, time.FixedZone("synthetic-offset", 8*3600)))
	raw, err := json.Marshal(value)
	if err != nil || string(raw) != `"2026-10-16T00:09:10.1234567Z"` {
		t.Fatal(string(raw), err)
	}
	var decoded dto.UTCDateTime
	if err := json.Unmarshal(raw, &decoded); err != nil || !decoded.Time().Equal(value.Time()) {
		t.Fatal("datetime round trip", err)
	}
	if err := json.Unmarshal([]byte(`"2026-10-16T08:09:10.1234567+08:00"`), &decoded); err != nil {
		t.Fatal(err)
	}
	again, err := json.Marshal(decoded)
	if err != nil || !bytes.Equal(raw, again) {
		t.Fatal("offset not normalized", string(again), err)
	}
	for _, text := range []string{`null`, `123`, `"not-a-date"`, `"2026-10-16"`, `"2026-10-16T00:00:00.123456789Z"`, `"0001-01-01T00:00:00.0000000+01:00"`, `"9999-12-31T23:59:59.0000000-01:00"`} {
		if err := json.Unmarshal([]byte(text), &decoded); err == nil {
			t.Fatalf("invalid datetime accepted %s", text)
		}
	}
	outside := dto.NewUTCDateTime(time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC))
	if _, err := json.Marshal(outside); err == nil {
		t.Fatal("invalid RFC3339 year serialized")
	}
}
func TestKnownSensitivePrefixesDoNotAppearInGoldenMediaObjects(t *testing.T) {
	for _, tc := range fixtures(t) {
		if tc.name != "base_item" && tc.name != "media_source" {
			continue
		}
		raw, err := json.Marshal(tc.full)
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(raw))
		for _, prefix := range []string{"/115/", "/root/", "/data/", "strm302", "pickcode", "apikey", "synthetic-global"} {
			if strings.Contains(lower, prefix) {
				t.Fatalf("%s fixture contains forbidden prefix %s", tc.name, prefix)
			}
		}
	}
	// Compile-time size assertions are supplemented with >2^53 round trips.
	value := dto.UserItemDataDto{PlaybackPositionTicks: ptr(int64(math.MaxInt64))}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var restored dto.UserItemDataDto
	if err := json.Unmarshal(raw, &restored); err != nil || restored.PlaybackPositionTicks == nil || *restored.PlaybackPositionTicks != math.MaxInt64 {
		t.Fatal("int64 precision lost", err)
	}
}

func TestMediaSourcePathAcceptsOnlyNativeAbsolutePlaybackURL(t *testing.T) {
	const valid = "https://media.synthetic.invalid:25018/Videos/100005/stream.mkv?Static=true&MediaSourceId=100005&api_key=synthetic-user-token"
	for _, path := range []string{"", valid, strings.Replace(valid, "https:", "http:", 1)} {
		source, err := dto.NewMediaSourceInfo(path)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		var restored dto.MediaSourceInfo
		if err := json.Unmarshal(raw, &restored); err != nil || restored.Path != path {
			t.Fatal("source path round trip", err)
		}
		item, err := dto.NewBaseItemDto("100005", "Synthetic", dto.Episode)
		if err != nil {
			t.Fatal(err)
		}
		item.MediaSources = dto.Array[dto.MediaSourceInfo]{source}
		if _, err := json.Marshal(item); err != nil {
			t.Fatal("nested valid stream rejected", err)
		}
		if path != "" {
			item.Path = path
			if _, err := json.Marshal(item); err == nil {
				t.Fatal("URL accepted in BaseItem.Path")
			}
		}
	}
	invalid := []string{
		"Synthetic.mkv", "/Videos/100005/stream.mkv", "//media.synthetic.invalid/Videos/100005/stream.mkv",
		"https://media.synthetic.invalid/api/v1/cloudStorage/strm302?apiKey=synthetic-global&pickCode=synthetic-pick&path=/115/private.mkv",
		strings.Replace(valid, "https:", "file:", 1), strings.Replace(valid, "media.synthetic.invalid", "user:password@media.synthetic.invalid", 1),
		strings.Replace(valid, "media.synthetic.invalid", "cdnfhnfile.115cdn.net", 1), valid + "#private", valid + "&pickcode=synthetic", valid + "&api_key=other",
		strings.Replace(valid, "/Videos/", "/Videos/../115/", 1), strings.Replace(valid, "/Videos/", "/Videos/%252f115/", 1),
		strings.Replace(valid, "synthetic-user-token", "pickcode", 1), strings.Replace(valid, "Static=true", "Static=false", 1),
		strings.Replace(valid, "MediaSourceId=100005", "MediaSourceId=", 1), strings.Replace(valid, "api_key=synthetic-user-token", "api_key=", 1),
		strings.Replace(valid, "media.synthetic.invalid:25018", "", 1), valid + "%ZZ", strings.Repeat("x", 4097),
		strings.Replace(valid, "/Videos/", "/Other/", 1), strings.Replace(valid, "/Videos/", "/Videos//", 1),
		strings.Replace(valid, "media.synthetic.invalid", "proapi.115.com", 1),
		strings.Replace(valid, "synthetic-user-token", "bad token", 1), strings.Replace(valid, "synthetic-user-token", "bad%2Ftoken", 1),
		strings.Replace(valid, "synthetic-user-token", "token;path=private", 1), strings.Replace(valid, "synthetic-user-token", strings.Repeat("x", 513), 1),
		valid + "\n", strings.Replace(valid, "/Videos/", "/Videos/\\private", 1),
	}
	for _, path := range invalid {
		if _, err := dto.NewMediaSourceInfo(path); err == nil {
			t.Errorf("unsafe URL accepted: %q", path)
		}
		if _, err := json.Marshal(dto.MediaSourceInfo{Path: path}); err == nil {
			t.Errorf("unsafe URL serialized: %q", path)
		}
		raw, err := json.Marshal(map[string]string{"Path": path})
		if err != nil {
			t.Fatal(err)
		}
		var source dto.MediaSourceInfo
		if err := json.Unmarshal(raw, &source); err == nil {
			t.Errorf("unsafe URL decoded: %q", path)
		}
	}
}
