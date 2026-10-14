package httpapi

import (
	"context"
	"net/http"
	"time"
)

func (a *API) libraryOrganizeSelection(w http.ResponseWriter, r *http.Request) {
	e, err := a.files()
	if err != nil {
		fileError(w, err)
		return
	}
	var in struct {
		Candidates []string `json:"candidateIds"`
		Assets     []string `json:"assetIds"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	out, err := e.LibrarySelection(ctx, in.Candidates, in.Assets)
	if err != nil {
		fileError(w, err)
		return
	}
	write(w, 200, out)
}
