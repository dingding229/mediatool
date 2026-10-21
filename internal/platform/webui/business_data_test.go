package webui

import (
	"bytes"
	"io/fs"
	"net/http/httptest"
	"testing"
)

func TestIndexIsStaticAndDoesNotInjectBusinessData(t *testing.T) {
	t.Setenv("SECRET_KEY", "never-inject-this-private-value")
	raw, err := fs.ReadFile(assets, "assets/index.html")
	if err != nil {
		t.Skip("frontend not built")
	}
	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, httptest.NewRequest("GET", "/library", nil))
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), raw) {
		t.Fatal("index was not served verbatim")
	}
	if bytes.Contains(w.Body.Bytes(), []byte("never-inject-this-private-value")) {
		t.Fatal("business data injected")
	}
}
