package updateidle

import (
	"testing"
	"time"
)

func TestAdmissionAndAutomaticExpiry(t *testing.T) {
	var g Gate
	release, ok := g.Enter()
	if !ok {
		t.Fatal("initial")
	}
	if _, ok = g.CloseFor(time.Minute); ok {
		t.Fatal("inflight allowed")
	}
	release()
	release()
	open, ok := g.CloseFor(10 * time.Millisecond)
	if !ok {
		t.Fatal("close")
	}
	if _, ok = g.Enter(); ok {
		t.Fatal("gate failed")
	}
	time.Sleep(20 * time.Millisecond)
	done, ok := g.Enter()
	if !ok {
		t.Fatal("TTL did not release")
	}
	done()
	open()
	if g.active != 0 {
		t.Fatal(g.active)
	}
	var restarted Gate
	done, ok = restarted.Enter()
	if !ok {
		t.Fatal("restart retained gate")
	}
	done()
}
