package httpapi

import (
	"encoding/json"
	v2 "media-tool/internal/modules/plugins/v2"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
)

func TestPluginReviewAdminDTOAndCatalogCounts(t *testing.T) {
	ctx, s := pluginHTTPFixture(t)
	defs, _ := v2.Catalog()
	d := defs[0]
	scopes := []string{}
	for _, p := range d.Manifest.Permissions {
		scopes = append(scopes, p.Scope)
	}
	in, e := s.Apply(ctx, "admin", v2.Request{Kind: v2.Install, OperationKey: "review-http-install", PluginID: d.Manifest.ID, Fingerprint: d.Fingerprint, Confirm: true, Scopes: scopes})
	if e != nil {
		t.Fatal(e)
	}
	cfg := cfg()
	cfg.SecretKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	cfg.PluginDataRoot = s.Root
	a := &API{Config: cfg, Deps: Dependencies{DB: s.DB}}
	r := httptest.NewRequest("GET", "/", nil)
	r.SetPathValue("id", in.ID)
	w := httptest.NewRecorder()
	a.pluginReviewPlan(w, r)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
	var envelope struct {
		Data v2.ReviewPlan `json:"data"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &envelope); e != nil || envelope.Data.ApprovedVersion != d.Manifest.Version || envelope.Data.Instance.Desired != v2.Disabled {
		t.Fatal(envelope, e)
	}
	w = httptest.NewRecorder()
	a.pluginV2Catalog(w, httptest.NewRequest("GET", "/?pageSize=200", nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var catalog struct {
		Data struct {
			Items []v2.Definition `json:"items"`
		} `json:"data"`
	}
	if json.Unmarshal(w.Body.Bytes(), &catalog) != nil || catalog.Data.Items[0].InstalledCount == nil || *catalog.Data.Items[0].InstalledCount != 1 {
		t.Fatal(w.Body.String())
	}
}

func TestPluginReviewSharedGoVueContract(t *testing.T) {
	raw, e := os.ReadFile("testdata/contracts/plugin-library-review.json")
	if e != nil {
		t.Fatal(e)
	}
	var plan v2.ReviewPlan
	if e = json.Unmarshal(raw, &plan); e != nil || !plan.NeedsReview || plan.ApprovedVersion != "1.0.0" || plan.Current.Author != "Fixture Maintainer" || plan.Added[0] != "settings.write" || plan.Changed[0] != "fs.data" {
		t.Fatal(plan, e)
	}
	encoded, _ := json.Marshal(plan)
	var a, b any
	json.Unmarshal(raw, &a)
	json.Unmarshal(encoded, &b)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("shared contract drift", string(encoded))
	}
}
