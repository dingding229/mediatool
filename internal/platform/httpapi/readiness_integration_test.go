package httpapi

import (
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/strm"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkflowReadinessDoesNotProbeOrEnableCredentials(t *testing.T) {
	ctx, s := pluginHTTPFixture(t)
	id := "11111111-0000-4000-8000-000000000071"
	cipher, e := s.Vault.Seal(id, map[string]string{"accessToken": "private-token", "refreshToken": "private-refresh"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(ctx, `INSERT INTO integrations(id,kind,name,credentials_ciphertext,enabled)VALUES($1,'open115','Synthetic',$2,false)`, id, cipher); e != nil {
		t.Fatal(e)
	}
	api := API{Deps: Dependencies{DB: s.DB, Integrations: &integrations.Manager{Repository: integrations.Repository{DB: s.DB, Vault: s.Vault}}}, Builder: strm.Builder{BaseURL: "http://127.0.0.1", APIKey: "private-strm-key", AllowedRoots: []string{"/Synthetic"}}}
	w := httptest.NewRecorder()
	api.workflowReadiness(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"nextAction":"enable_connection"`) || !strings.Contains(w.Body.String(), `"nextAction":"verify_player_302"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, secret := range []string{"private-token", "private-refresh", "private-strm-key"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("readiness leaked credentials")
		}
	}
	var enabled bool
	if e = s.DB.QueryRow(ctx, `SELECT enabled FROM integrations WHERE id=$1`, id).Scan(&enabled); e != nil || enabled {
		t.Fatal("readiness enabled connection", e)
	}
}
