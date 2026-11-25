package httpapi

import (
	"crypto/sha256"
	"io"
	"media-tool/internal/platform/auth"
	"net/http"
	"sync"
	"time"
)

const jobEventInterval = 3 * time.Second
const jobEventWriteTimeout = 2 * time.Second
const maxJobStreamsPerSession = 4
const maxJobStreams = 64

type jobStreamBudget struct {
	mu        sync.Mutex
	active    int
	bySession map[[sha256.Size]byte]int
}

var jobStreams jobStreamBudget

func (b *jobStreamBudget) acquire(session [sha256.Size]byte) func() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active >= maxJobStreams || b.bySession[session] >= maxJobStreamsPerSession {
		return nil
	}
	if b.bySession == nil {
		b.bySession = make(map[[sha256.Size]byte]int)
	}
	b.active++
	b.bySession[session]++
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.active--
		b.bySession[session]--
		if b.bySession[session] == 0 {
			delete(b.bySession, session)
		}
	}
}

func (a *API) events(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodHead {
		jobEventHeaders(w)
		w.WriteHeader(http.StatusOK)
	} else {
		a.streamJobEvents(w, r)
	}
}

func (a *API) streamJobEvents(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil {
		fail(w, 401, "unauthorized", "请使用管理员账号密码登录")
		return
	}
	release := jobStreams.acquire(sha256.Sum256([]byte(cookie.Value)))
	if release == nil {
		w.Header().Set("Retry-After", "3")
		fail(w, 429, "job_stream_limit", "任务订阅并发已达上限，请读取任务列表")
		return
	}
	defer release()
	controller := http.NewResponseController(w)
	if err = controller.SetWriteDeadline(time.Time{}); err != nil {
		fail(w, 503, "job_stream_unavailable", "任务订阅暂不可用，请读取任务列表")
		return
	}
	jobEventHeaders(w)
	ticker := time.NewTicker(jobEventInterval)
	defer ticker.Stop()
	for {
		// There is no event journal. Recheck the cookie before each hint.
		if r.Context().Err() != nil || !a.authorized(r) || !writeJobHeartbeat(w, controller) {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func jobEventHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
}

func writeJobHeartbeat(w http.ResponseWriter, controller *http.ResponseController) bool {
	if err := controller.SetWriteDeadline(time.Now().Add(jobEventWriteTimeout)); err != nil {
		return false
	}
	if _, err := io.WriteString(w, "event: tick\ndata: {}\n\n"); err != nil {
		return false
	}
	if err := controller.Flush(); err != nil {
		return false
	}
	// The deadline covers only the write/flush, not the idle heartbeat gap.
	return controller.SetWriteDeadline(time.Time{}) == nil
}
