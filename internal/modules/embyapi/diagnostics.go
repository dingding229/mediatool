package embyapi

import (
	"net/http"
	"regexp"
	"time"
)

var diagnosticOpaque = regexp.MustCompile(`[a-zA-Z0-9_-]{32,}`)
var diagnosticCredential = regexp.MustCompile(`(?i)(api[_-]?key|token|filekey|pick_?code)[\s=:]+[^\s,;&/]+`)

func diagnosticText(value string) string {
	if len(value) > 4096 {
		value = value[:4096]
	}
	return diagnosticOpaque.ReplaceAllString(diagnosticCredential.ReplaceAllString(value, "[redacted]"), "[redacted]")
}
func (r *Router) logUnknown(req *http.Request) {
	// Bound anonymous diagnostic amplification. The small lock never spans I/O.
	r.logMu.Lock()
	window := time.Now().Unix() / 60
	if window != r.logWindow {
		r.logWindow = window
		r.logCount = 0
	}
	emit := r.logCount < 60
	if emit {
		r.logCount++
	}
	r.logMu.Unlock()
	if emit {
		r.logger.Info("native Emby endpoint not found", "path", diagnosticText(req.URL.Path), "userAgent", diagnosticText(req.UserAgent()))
	}
}
