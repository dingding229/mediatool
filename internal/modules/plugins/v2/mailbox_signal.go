package v2

import (
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Hints only, never authority: SQL owns the queue and all generation/grant fences.
// Register only live pumps; deleting a lane on exit bounds memory by worker count.
// The existing poll remains necessary for other processes and transactional jobs.
type mailboxKey struct {
	db       *pgxpool.Pool
	instance string
}
type mailboxLane struct {
	wake    chan struct{}
	mu      sync.Mutex
	changed chan struct{}
}

var mailboxLanes sync.Map

func (s Store) mailbox(id string) *mailboxLane {
	v, ok := mailboxLanes.Load(mailboxKey{s.DB, id})
	if !ok {
		return nil
	}
	return v.(*mailboxLane)
}
func (l *mailboxLane) queued() {
	if l == nil {
		return
	}
	select {
	case l.wake <- struct{}{}:
	default:
	}
}
func (l *mailboxLane) observed() <-chan struct{} {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.changed
}
func (l *mailboxLane) completed() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	close(l.changed)
	l.changed = make(chan struct{})
}
