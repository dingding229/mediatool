package embyapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"
)

const BrowseRequestBudget = 6 * time.Second
const maxBrowseResponse = 32 << 20

// Buffer bounded JSON until all stages finish, so a deadline after a handler
// starts encoding cannot leave a partial 200 instead of the fixed 503 response.
type browseBuffer struct {
	header   http.Header
	body     bytes.Buffer
	status   int
	overflow bool
}

func (b *browseBuffer) Header() http.Header { return b.header }
func (b *browseBuffer) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}
func (b *browseBuffer) Write(data []byte) (int, error) {
	if b.status == 0 {
		b.status = 200
	}
	if b.body.Len()+len(data) > maxBrowseResponse {
		b.overflow = true
		return 0, io.ErrShortWrite
	}
	return b.body.Write(data)
}
func (s *Server) browseBudget(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	s.requestBudget(w, r, BrowseRequestBudget, next)
}
func (s *Server) requestBudget(w http.ResponseWriter, r *http.Request, budget time.Duration, next http.HandlerFunc) {
	ctx, cancel := context.WithTimeout(r.Context(), budget)
	defer cancel()
	buffer := &browseBuffer{header: make(http.Header)}
	next(buffer, r.WithContext(ctx))
	if ctx.Err() != nil || buffer.overflow {
		s.router.fail(w, 503)
		return
	}
	for key, values := range buffer.header {
		w.Header()[key] = values
	}
	status := buffer.status
	if status == 0 {
		status = 200
	}
	w.WriteHeader(status)
	if _, err := w.Write(buffer.body.Bytes()); err != nil {
		s.options.Logger.Debug("Native browse response interrupted", "module", "native_media")
	}
}
