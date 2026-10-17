package dto_test

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"media-tool/internal/modules/embyapi/dto"
)

func TestReportsAcceptNumbersAndStringNumbersWithoutDiscardingZero(t *testing.T) {
	for _, makeTarget := range []func() any{func() any { return new(dto.PlayRequest) }, func() any { return new(dto.PlaybackProgressInfo) }, func() any { return new(dto.PlaybackStopInfo) }} {
		for _, value := range []string{"0", `"0"`, strconv.FormatInt(math.MaxInt64, 10), `"` + strconv.FormatInt(math.MaxInt64, 10) + `"`} {
			target := makeTarget()
			raw := []byte(`{"iTeMiD":"100005","pOsItIoNtIcKs":` + value + `,"Unrecognized":{"Nested":[1,null,true]}}`)
			if err := json.Unmarshal(raw, target); err != nil {
				t.Fatalf("valid report rejected: %s: %v", raw, err)
			}
			position := reflect.ValueOf(target).Elem().FieldByName("PositionTicks")
			if position.IsNil() {
				t.Fatal("explicit zero became missing")
			}
			if strings.Contains(value, "922337") && position.Elem().Int() != math.MaxInt64 {
				t.Fatal("large ticks lost precision")
			}
			output, err := json.Marshal(target)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(output, []byte("Unrecognized")) || bytes.Contains(output, []byte("Nested")) {
				t.Fatal("unknown report fields retained")
			}
		}
	}
	var progress dto.PlaybackProgressInfo
	if err := json.Unmarshal([]byte(`{"PositionTicks":"0007","RunTimeTicks":"10000000","AudioStreamIndex":"0","SubtitleStreamIndex":"-1","VolumeLevel":"0","IsPaused":true}`), &progress); err != nil || progress.PositionTicks == nil || *progress.PositionTicks != 7 || progress.AudioStreamIndex == nil || *progress.AudioStreamIndex != 0 || progress.SubtitleStreamIndex == nil || *progress.SubtitleStreamIndex != -1 || !progress.IsPaused {
		t.Fatal("mixed numeric fields", progress, err)
	}
}
func TestReportsRejectInvalidNumericFormsAndDoNotPartiallyMutate(t *testing.T) {
	for _, payload := range []string{`null`, `[]`, `{"PositionTicks":1.5}`, `{"PositionTicks":"1.5"}`, `{"PositionTicks":1e3}`, `{"PositionTicks":true}`, `{"PositionTicks":""}`, `{"PositionTicks":"9223372036854775808"}`, `{"PositionTicks":"-9223372036854775809"}`, `{"PositionTicks":" 1 "}`, `{"PositionTicks":0,"positionticks":"1"}`, `{"ItemId":100005}`} {
		original := dto.PlaybackProgressInfo{ItemId: "original", PositionTicks: ptr(int64(17))}
		before := original
		if err := json.Unmarshal([]byte(payload), &original); err == nil {
			t.Fatalf("bad report accepted %s", payload)
		}
		if !reflect.DeepEqual(original, before) {
			t.Fatal("failed decode mutated receiver")
		}
	}
	for _, payload := range []string{`{"VolumeLevel":"2147483648"}`, `{"AudioStreamIndex":2147483648}`, `{"SubtitleStreamIndex":"not-an-int"}`, `{"IsPaused":"true"}`, `{"PositionTicks":"0",`, strings.Repeat(" ", 65537)} {
		var progress dto.PlaybackProgressInfo
		if err := json.Unmarshal([]byte(payload), &progress); err == nil {
			t.Fatalf("bad field accepted %.100q", payload)
		}
	}
	var progress dto.PlaybackProgressInfo
	if err := json.Unmarshal([]byte(`{"PositionTicks":null,"AudioStreamIndex":null,"Unknown":123}`), &progress); err != nil || progress.PositionTicks != nil || progress.AudioStreamIndex != nil {
		t.Fatal("optional null not accepted", err)
	}
	if err := json.Unmarshal([]byte(`{"PositionTicks":"01","positionticks":1}`), &progress); err != nil || progress.PositionTicks == nil || *progress.PositionTicks != 1 {
		t.Fatal("identical normalized values not accepted", err)
	}
}
func FuzzReportDecodingPreservesInt64AndNeverEnablesTranscoding(f *testing.F) {
	for _, value := range []string{`{}`, `{"PositionTicks":"0"}`, `{"PositionTicks":9007199254740993}`, `{"Unknown":{"SupportsTranscoding":true}}`, `{"PositionTicks":null}`, `{"PositionTicks":"9223372036854775808"}`, `{"Path":"https://media.synthetic.invalid/Videos/100005/stream.mkv?Static=true&MediaSourceId=100005&api_key=synthetic-user-token"}`, `{"Path":"https://media.synthetic.invalid/api/v1/cloudStorage/strm302?apiKey=synthetic-global&pickCode=synthetic"}`} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		for _, target := range []any{new(dto.PlayRequest), new(dto.PlaybackProgressInfo), new(dto.PlaybackStopInfo)} {
			if err := json.Unmarshal([]byte(raw), target); err != nil {
				continue
			}
			encoded, err := json.Marshal(target)
			if err != nil {
				t.Fatal(err)
			}
			inspect(t, encoded)
		}
		var source dto.MediaSourceInfo
		if err := json.Unmarshal([]byte(raw), &source); err == nil {
			if source.SupportsTranscoding() {
				t.Fatal("transcoding enabled")
			}
			output, err := json.Marshal(source)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(output, []byte(`"SupportsTranscoding":true`)) {
				t.Fatal("transcoding output")
			}
		}
	})
}
