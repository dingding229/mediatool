package business

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"strings"
	"testing"
)

func TestSafeProjectionNeverReturnsCredentialsOrInternalHints(t *testing.T) {
	for _, input := range []string{"https://tracker.invalid/SECRET/announce?passkey=SECRET", "magnet:?xt=HASH&tr=https://SECRET", "password=SECRET", "token=SECRET", "pickCode=SECRET", "API-KEY=SECRET"} {
		if strings.Contains(safeText(input), "SECRET") {
			t.Fatal(input, safeText(input))
		}
	}
	h := AcquisitionHost{}
	var admit int
	ctx := context.Background()
	falseHR := false
	tests := []struct {
		method string
		in     any
	}{{"metadata.torrent.parse", b.TorrentInput{Data: []byte("d4:infod6:lengthi4e4:name4:test12:piece lengthi4e6:pieces20:12345678901234567890ee")}}, {"subscriptions.evaluateRules", b.Evaluation{Resource: b.Resource{Title: "Owned Film 2026", HR: &falseHR}, Media: b.Media{ID: 1, MediaType: "movie", Title: "Owned Film", Year: "2026"}, Rules: b.Rules{}}}, {"subscriptions.coverage", b.CoverageInput{Names: []string{"Owned.S01E01.mkv"}, Season: 1}}}
	for _, tt := range tests {
		raw, _ := json.Marshal(tt.in)
		v, e := h.Call(ctx, contracts.HostCall{Method: tt.method, Args: raw}, contracts.Permission{Scope: func() string { s, _ := b.Lookup(tt.method); return s.Scope }()}, func() error { admit++; return nil })
		if e != nil {
			t.Fatalf("%s: %v", tt.method, e)
		}
		encoded, _ := json.Marshal(v)
		if strings.Contains(string(encoded), "downloadHint") {
			t.Fatal(string(encoded))
		}
	}
	if admit != 0 {
		t.Fatal("pure functions attempted side effects")
	}
	for _, m := range []string{"downloads.recheck", "subscriptions.completion"} {
		_, e := h.Call(ctx, contracts.HostCall{Method: m}, contracts.Permission{}, nil)
		if e == nil {
			t.Fatal("fake completion/recheck")
		}
	}
	for _, r := range []b.Rules{{Include: make([]string, 201)}, {Keyword: strings.Repeat("中", 201)}} {
		if validateRules(r) == nil {
			t.Fatal("unbounded rules")
		}
	}
}
