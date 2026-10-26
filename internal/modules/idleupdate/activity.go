package idleupdate

import (
	"sync"
	"time"
)

var activityHealth struct {
	sync.Mutex
	failedAt time.Time
}

func ActivityWriteFailed() {
	activityHealth.Lock()
	activityHealth.failedAt = time.Now()
	activityHealth.Unlock()
}
func ActivityWriteSucceeded() {} // The failure quarantine lasts a complete configured quiet interval.
func activityReliable(now time.Time, minutes int) bool {
	activityHealth.Lock()
	defer activityHealth.Unlock()
	return activityHealth.failedAt.IsZero() || now.Sub(activityHealth.failedAt) >= time.Duration(minutes)*time.Minute
}
