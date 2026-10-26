package embyapi

import (
	"context"
	"media-tool/internal/modules/nativelibrary"
	"net/http/httptest"
	"testing"
)

func TestT16BuiltinRootAndProtocolAliasesRemainAvailable(t *testing.T) {
	value := settingsFixture()
	value.PublicURL = "http://127.0.0.1:25018"
	server := nativeServer(t, func(context.Context) (nativelibrary.ServerSettings, error) { return value, nil }, "", nil)
	for _, prefix := range []string{"", "/emby", "/mediabrowser"} {
		w := httptest.NewRecorder()
		server.ServeHTTP(w, httptest.NewRequest("GET", prefix+"/System/Info/Public", nil))
		if w.Code != 200 {
			t.Fatal(prefix, w.Code, w.Body.String())
		}
	}
}
