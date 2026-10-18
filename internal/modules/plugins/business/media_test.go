package business

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"media-tool/internal/modules/filemanager"
	"media-tool/internal/modules/jobs"
	"strings"
	"testing"
)

func mediaCall(t *testing.T, h AcquisitionHost, method string, in any, revision ...int64) (any, error) {
	t.Helper()
	raw, e := json.Marshal(in)
	if e != nil {
		t.Fatal(e)
	}
	spec, ok := b.Lookup(method)
	if !ok {
		t.Fatal(method)
	}
	call := contracts.HostCall{Method: method, Args: raw, OperationKey: jobs.UUID()}
	if len(revision) > 0 {
		call.ExpectedRevision = &revision[0]
	}
	return h.Call(context.Background(), call, contracts.Permission{Scope: spec.Scope}, func() error { return nil })
}
func mediaCode(t *testing.T, err error, code contracts.Code) {
	t.Helper()
	var rpc *contracts.Error
	if !errors.As(err, &rpc) || rpc.Code != code {
		t.Fatalf("want %d got %v", code, err)
	}
}
func TestMediaPureMethodsAndBounds(t *testing.T) {
	h := AcquisitionHost{}
	for _, v := range []struct {
		method string
		in     any
	}{
		{"metadata.parse", b.Text{Text: "Owned.2026.1080p.mkv"}},
		{"metadata.attributes.parse", b.Text{Text: "Owned.1080p.BluRay.H265"}},
		{"metadata.attributes.validate", b.Attribute{Key: "resolution", Value: "1080p"}},
		{"metadata.episodes", b.Text{Text: "Owned.S01E01-E03.mkv"}},
		{"metadata.normalize", b.EpisodeCoverage{Season: 1, Episodes: []int{3, 1, 2}}},
		{"metadata.name.render", b.RenderName{Template: "{title} ({year})", Hint: b.NameHint{Title: "Owned", Year: "2026"}, Original: "Owned.mkv"}},
		{"classification.get", b.Empty{}},
		{"classification.match", b.Classify{Media: b.Media{MediaType: "movie", Title: "Owned"}}},
		{"classification.render", b.Classify{Media: b.Media{MediaType: "movie", Title: "Owned", Year: "2026"}, Source: "Owned.mkv", MovieTemplate: "{title}", Extension: "mkv"}},
	} {
		t.Run(v.method, func(t *testing.T) {
			if _, e := mediaCall(t, h, v.method, v.in); e != nil {
				t.Fatal(e)
			}
		})
	}
	for _, raw := range []string{`{"text":"` + strings.Repeat("中", 201) + `"}`, `{"items":[` + strings.Repeat(`{},`, 200) + `{}]}`, `{"text":"a\u0000b"}`} {
		if mediaBounds([]byte(raw)) {
			t.Fatal("unbounded", raw)
		}
	}
	for _, p := range []string{"../other", "a/../b", "/absolute", "a//b", "a\\b", "./b"} {
		if _, e := relativeCloudRoot("/115/Media", p); e == nil {
			t.Fatal("escaped", p)
		}
		if _, e := fileInput(b.FileInput{RootID: jobs.UUID(), Operation: "move", Source: p}); e == nil {
			t.Fatal("escaped input", p)
		}
	}
	if _, e := fileInput(b.FileInput{RootID: jobs.UUID(), Operation: "copy", Source: "Owned.mkv"}); e == nil {
		t.Fatal("unsupported native copy")
	}
}
func TestEveryMediaRPCUnavailableBackendExplicitUnsupported(t *testing.T) {
	h := AcquisitionHost{}
	for _, m := range b.Methods() {
		if !(strings.HasPrefix(m.Name, "library.") || strings.HasPrefix(m.Name, "strm.") || strings.HasPrefix(m.Name, "fs.media.") || strings.HasPrefix(m.Name, "storage.")) {
			continue
		}
		t.Run(m.Name, func(t *testing.T) { _, e := mediaCall(t, h, m.Name, b.Empty{}); mediaCode(t, e, contracts.Unsupported) })
	}
}
func TestSafeFilePlansNeverExposeRootOrRawSTRM(t *testing.T) {
	r := filemanager.Result{ID: jobs.UUID(), State: "preview", Plan: filemanager.Plan{Root: "/115/SECRET", OriginFingerprint: "PRIVATE", Items: []filemanager.Item{{Source: filemanager.File{Path: "Owned.mkv", Name: "Owned.mkv"}, Target: "Movies/Owned.mkv"}}, Steps: []filemanager.Step{{Target: "Movies/Owned.mkv"}}}}
	out, e := filePlan(r)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "SECRET") || strings.Contains(string(raw), "PRIVATE") || len(out.Targets) != 1 {
		t.Fatal(string(raw))
	}
	if _, e = filePlan(filemanager.Result{Plan: filemanager.Plan{Items: make([]filemanager.Item, 201)}}); e == nil {
		t.Fatal("unbounded plan")
	}
}
