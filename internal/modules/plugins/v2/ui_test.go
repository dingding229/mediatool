package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"media-tool/internal/modules/releasename"
	"strings"
	"testing"
)

func TestPluginUISchemaBounds(t *testing.T) {
	m := contracts.Manifest{Extensions: contracts.Extensions{Dashboards: []string{"a"}}}
	for _, x := range []struct {
		kind, raw string
		valid     bool
	}{{"form", `{"content":[{"component":"Unknown","model":"safe","when":{"model":"enabled","equals":true}}],"defaults":{}}`, true}, {"meta", `[{"key":"a","title":"A","cols":6,"refresh":30,"border":true}]`, true}, {"meta", `[{"key":"unlisted","cols":6,"refresh":30}]`, false}, {"meta", `[{"key":"a","cols":99,"refresh":0}]`, false}, {"page", `{"content":[{"component":"Text","model":"__proto__"}]}`, false}, {"form", `{"content":[],"defaults":[]}`, false}, {"page", `{"content":[{"component":"Text","model":"nested.value"}]}`, false}} {
		if e := validateUI(m, x.kind, json.RawMessage(x.raw)); (e == nil) != x.valid {
			t.Fatal(x, e)
		}
	}
	n := contracts.Component{Component: "Text"}
	for range 14 {
		n = contracts.Component{Component: "VCol", Children: []contracts.Component{n}}
	}
	raw, _ := json.Marshal(contracts.Page{Content: []contracts.Component{n}})
	if validateUI(m, "page", raw) == nil {
		t.Fatal("deep tree")
	}
	raw, _ = json.Marshal(contracts.Page{Content: []contracts.Component{{Component: "Text", Props: map[string]json.RawMessage{"text": json.RawMessage(`"` + strings.Repeat("x", 8192) + `"`)}}}})
	if validateUI(m, "page", raw) == nil {
		t.Fatal("oversize prop")
	}
}
func TestNameRecognitionBuiltinOnUnavailableAndRecursion(t *testing.T) {
	s := Store{}
	for _, ctx := range []context.Context{context.Background(), context.WithValue(context.Background(), hookContext{}, true)} {
		got := s.RecognizeName(ctx, "/private/Owned.2026.S01E02.mkv")
		want := releasename.Parse("Owned.2026.S01E02.mkv")
		if got.Title != want.Title || got.Episode != want.Episode {
			t.Fatal(got, want)
		}
	}
}
