package httpapi

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPipelineReadDistinguishesNotFoundFromFailure(t *testing.T) {
	for _, q := range []struct {
		e    error
		code int
	}{{nil, 200}, {pgx.ErrNoRows, 404}, {errors.New("private DSN and credential must not leak"), 503}} {
		w := httptest.NewRecorder()
		failed := pipelineReadError(w, q.e)
		if failed != (q.e != nil) || w.Code != q.code || strings.Contains(w.Body.String(), "credential") {
			t.Fatal(w.Code, w.Body.String())
		}
		if q.code == 503 && (!strings.Contains(w.Body.String(), `"reason":"storage_unavailable"`) || !strings.Contains(w.Body.String(), `"nextAction":"refresh_status"`)) {
			t.Fatal(w.Body.String())
		}
	}
}
