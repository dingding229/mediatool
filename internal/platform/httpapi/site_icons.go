package httpapi

import (
	"crypto/sha256"
	"fmt"
	mediatoolsite "github.com/dingding229/mediatool-site"
	"net/http"
)

func (a *API) ptTemplateIcon(w http.ResponseWriter, r *http.Request) {
	raw, ok := mediatoolsite.Icon(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	tag := fmt.Sprintf(`"%x"`, sha256.Sum256(raw))
	w.Header().Set("ETag", tag)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Header.Get("If-None-Match") == tag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Write(raw)
}
