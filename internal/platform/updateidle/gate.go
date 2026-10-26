// Package updateidle provides only the short, process-local automatic-update admission gate.
// It never changes maintenance mode, playback delivery or persistent worker configuration.
package updateidle

import (
	"sync"
	"time"
)

type Gate struct {
	mu         sync.Mutex
	active     int
	until      time.Time
	generation uint64
}

var Default Gate

func (g *Gate) Enter() (func(), bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if time.Now().Before(g.until) {
		return func() {}, false
	}
	g.active++
	var once sync.Once
	return func() { once.Do(func() { g.mu.Lock(); g.active--; g.mu.Unlock() }) }, true
}
func (g *Gate) CloseFor(d time.Duration) (func(), bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active != 0 || time.Now().Before(g.until) {
		return func() {}, false
	}
	if d <= 0 || d > 5*time.Minute {
		d = 5 * time.Minute
	}
	g.generation++
	generation := g.generation
	g.until = time.Now().Add(d)
	return func() {
		g.mu.Lock()
		if g.generation == generation {
			g.until = time.Time{}
		}
		g.mu.Unlock()
	}, true
}
