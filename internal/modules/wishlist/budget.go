package wishlist

import (
	"context"
	"golang.org/x/net/html"
	"io"
	"strings"
	"sync"
	"time"
)

var publicSlots = make(chan struct{}, 2)
var publicRate struct {
	sync.Mutex
	next time.Time
}

func admit(ctx context.Context) (func(), bool) {
	select {
	case publicSlots <- struct{}{}:
	case <-ctx.Done():
		return nil, false
	}
	publicRate.Lock()
	now := time.Now()
	at := maxTime(now, publicRate.next)
	publicRate.next = at.Add(250 * time.Millisecond)
	publicRate.Unlock()
	timer := time.NewTimer(time.Until(at))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		<-publicSlots
		return nil, false
	case <-timer.C:
		return func() { <-publicSlots }, true
	}
}
func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// Reject pathological public markup before building a DOM: byte bound alone
// does not bound node count, selector recursion depth or parse work well enough.
func htmlBudget(raw []byte) bool {
	z := html.NewTokenizer(strings.NewReader(string(raw)))
	depth, count := 0, 0
	for {
		t := z.Next()
		if t == html.ErrorToken {
			return z.Err() == io.EOF
		}
		count++
		if count > 10000 {
			return false
		}
		if t == html.StartTagToken {
			name, _ := z.TagName()
			switch string(name) {
			case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
			default:
				depth++
			}
		}
		if t == html.EndTagToken && depth > 0 {
			depth--
		}
		if depth > 128 {
			return false
		}
	}
}
