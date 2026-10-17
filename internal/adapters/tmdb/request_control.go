package tmdb

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type ResponseError struct {
	Status     int
	RetryAfter time.Duration
}

func (e *ResponseError) Error() string { return "TMDB upstream request rejected" }
func (e *ResponseError) Unwrap() error { return ErrRequest }

var requestSlots = make(chan struct{}, 2)
var requestRate struct {
	sync.Mutex
	next time.Time
}

func admitRequest(ctx context.Context) (func(), error) {
	select {
	case requestSlots <- struct{}{}:
	case <-ctx.Done():
		return nil, ErrRequest
	}
	release := func() { <-requestSlots }
	requestRate.Lock()
	wait := time.Until(requestRate.next)
	requestRate.next = time.Now().Add(max(wait, 0) + 100*time.Millisecond)
	requestRate.Unlock()
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			release()
			return nil, ErrRequest
		}
	}
	return release, nil
}
func retryDelay(header string) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds > 0 {
		return min(time.Duration(seconds), 3600) * time.Second
	}
	if date, err := http.ParseTime(header); err == nil {
		return min(max(time.Until(date), 0), time.Hour)
	}
	return 0
}

// ExponentialBackoff is used by persistent jobs, never a sleeping HTTP retry.
func ExponentialBackoff(attempt int, err error) time.Duration {
	delay := time.Duration(1<<min(max(attempt, 1), 10)) * time.Minute
	var response *ResponseError
	if errors.As(err, &response) && response.Status == 429 {
		delay = max(delay, response.RetryAfter)
	}
	return min(delay, 24*time.Hour)
}

func pauseRequests(delay time.Duration) {
	requestRate.Lock()
	defer requestRate.Unlock()
	requestRate.next = maxTime(requestRate.next, time.Now().Add(delay))
}
func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
