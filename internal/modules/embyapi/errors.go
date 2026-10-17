package embyapi

import (
	"encoding/json"
	"errors"
	"media-tool/internal/modules/nativeplayback"
	"net/http"
)

// WriteError accepts only a status, never an internal error or user-supplied
// message. All payloads are fixed; callers must handle a failed response write.
func WriteError(w http.ResponseWriter, status int) error {
	code := map[int]string{400: "BadRequest", 413: "PayloadTooLarge", 401: "Unauthorized", 403: "Forbidden", 404: "NotFound", 405: "MethodNotAllowed", 429: "TooManyRequests", 409: "Conflict", 500: "InternalServerError", 501: "NotImplemented", 503: "ServiceUnavailable"}[status]
	if code == "" {
		status, code = 500, "InternalServerError"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(struct {
		ResponseStatus struct{ ErrorCode, Message string }
	}{ResponseStatus: struct{ ErrorCode, Message string }{code, http.StatusText(status)}})
}

// WriteDomainError never serializes a domain error's text. Browsing is opaque;
// only explicit nativeplayback errors get fixed, readable denial reasons.
func WriteDomainError(w http.ResponseWriter, err error) error {
	if err == nil {
		return nil
	}
	message := ""
	switch {
	case errors.Is(err, nativeplayback.ErrParentalDenied):
		message = "分级限制，不允许播放此内容"
	case errors.Is(err, nativeplayback.ErrStreamLimit):
		message = "超过同时播放数限制，请先停止其他播放"
	case errors.Is(err, nativeplayback.ErrForbidden):
		message = "权限不足，无法播放此媒体库"
	}
	if message == "" {
		return WriteError(w, DomainErrorStatus(err))
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusForbidden)
	return json.NewEncoder(w).Encode(struct {
		ResponseStatus struct{ ErrorCode, Message string }
	}{struct{ ErrorCode, Message string }{"Forbidden", message}})
}
