package httpapi

import (
	"encoding/json"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/modules/externalapi"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/platform/auth"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestExternalKeyListWireContractNeverContainsCredentialMaterial(t *testing.T) {
	ctx, s := pluginHTTPFixture(t)
	manager := &integrations.Manager{Repository: integrations.Repository{DB: s.DB, Vault: s.Vault}}
	c := cfg()
	signer, _ := externalapi.NewSigner(strings.Repeat("a", 64))
	c.SecretKey = strings.Repeat("a", 64)
	token := legacyAdminCookie(c.SessionKey, time.Now().Add(time.Hour))
	if _, e := (accounts.Repository{DB: s.DB}).Register(ctx, "external_fixture", "synthetic-password", token); e != nil {
		t.Fatal(e)
	}
	api := &API{Config: c, Auth: auth.Auth{Key: c.SessionKey}, Deps: Dependencies{DB: s.DB, Integrations: manager, ExternalSigner: signer}}
	adminRequest := func(method, path, body string) *http.Request {
		r := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		return r
	}
	created := httptest.NewRecorder()
	api.createAPIKey(created, adminRequest("POST", "/api/v1/external-api/keys", `{"name":"Owned fixture","scopes":["status.read"],"confirm":true}`))
	if created.Code != 201 {
		t.Fatal("creation status", created.Code, created.Body.String())
	}
	var receipt struct {
		Data externalapi.Key `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &receipt); err != nil || strings.Count(receipt.Data.Token, ".") != 2 {
		t.Fatal("missing one-time credential", err)
	}
	key := receipt.Data
	// A list must not depend on recovering or decrypting an existing credential.
	manager.Repository.Vault = nil
	checkList := func(revoked bool) {
		t.Helper()
		w := httptest.NewRecorder()
		api.listAPIKeys(w, httptest.NewRequest("GET", "/api/v1/external-api/keys", nil).WithContext(ctx))
		if w.Code != 200 {
			t.Fatal("list status", w.Code, w.Body.String())
		}
		for _, secret := range []string{key.Token} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("credential material leaked through HTTP list")
			}
		}
		var out struct {
			Data []map[string]json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || len(out.Data) != 1 {
			t.Fatal("list envelope", err)
		}
		fields := out.Data[0]
		for _, field := range []string{"token", "tokenDigest", "token_digest", "hash", "token_ciphertext", "ciphertext"} {
			if _, ok := fields[field]; ok {
				t.Fatal("secret field serialized", field)
			}
		}
		if string(fields["id"]) != `"`+key.ID+`"` || string(fields["scopes"]) != `["status.read"]` {
			t.Fatal("metadata contract changed")
		}
		if (string(fields["revokedAt"]) != "null") != revoked {
			t.Fatal("revocation metadata mismatch")
		}
		assertExternalSummarySchema(t, fields)
	}
	checkList(false)
	if _, _, err := (externalapi.Repository{DB: s.DB, Signer: signer}).Admit(ctx, key.Token, "status.read", "GET", "/api/external/v1/status"); err != nil {
		t.Fatal("creation token unusable", err)
	}
	if err := (externalapi.Repository{DB: s.DB}).Revoke(ctx, key.ID); err != nil {
		t.Fatal(err)
	}
	checkList(true)
}

func assertExternalSummarySchema(t *testing.T, fields map[string]json.RawMessage) {
	t.Helper()
	raw, err := os.ReadFile("../../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	components := doc["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	get := doc["paths"].(map[string]any)["/api/v1/external-api/keys"].(map[string]any)["get"].(map[string]any)
	responseRef := get["responses"].(map[string]any)["200"].(map[string]any)["$ref"].(string)
	response := components["responses"].(map[string]any)[strings.TrimPrefix(responseRef, "#/components/responses/")].(map[string]any)
	envelope := response["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	data := envelope["properties"].(map[string]any)["data"].(map[string]any)
	ref := data["items"].(map[string]any)["$ref"]
	if ref != "#/components/schemas/modules_externalapi_KeySummary" {
		t.Fatal("list still documents creation receipt", ref)
	}
	summary := schemas["modules_externalapi_KeySummary"].(map[string]any)
	props := summary["properties"].(map[string]any)
	if summary["additionalProperties"] != false || len(props) != len(fields) {
		t.Fatal("list schema must be closed and match wire fields")
	}
	for field := range fields {
		if _, ok := props[field]; !ok {
			t.Fatal("undocumented list field", field)
		}
	}
	for _, value := range summary["required"].([]any) {
		field := value.(string)
		if _, ok := fields[field]; !ok {
			t.Fatal("required field missing", field)
		}
	}
	creation := schemas["modules_externalapi_Key"].(map[string]any)
	if _, ok := creation["properties"].(map[string]any)["token"]; !ok {
		t.Fatal("creation schema lost one-time token")
	}
}
