package embyapi

import (
	"sync"
	"time"
)

type imageWindow struct {
	start time.Time
	count int
}
type imageLimiter struct {
	mu      sync.Mutex
	windows map[string]imageWindow
	swept   time.Time
}

func (l *imageLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.windows == nil {
		l.windows = map[string]imageWindow{}
	}
	if now.Sub(l.swept) >= time.Minute {
		for key, w := range l.windows {
			if now.Sub(w.start) >= time.Minute {
				delete(l.windows, key)
			}
		}
		l.swept = now
	}
	w, exists := l.windows[ip]
	if !exists && len(l.windows) >= 10000 {
		return false
	}
	if now.Sub(w.start) >= time.Minute {
		w = imageWindow{start: now}
	}
	if w.count >= 600 {
		return false
	}
	w.count++
	l.windows[ip] = w
	return true
}
