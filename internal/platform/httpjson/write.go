// Package httpjson shares only transport mechanics, never protocol envelopes.
package httpjson

import (
	"encoding/json"
	"net/http"
)

func Write(w http.ResponseWriter, status int, value any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(value)
}
