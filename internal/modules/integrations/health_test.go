package integrations

import (
	"context"
	"errors"
	"media-tool/internal/adapters/open115"
	"strings"
	"testing"
	"time"
)

func TestHealthDelaysAndSafeReasons(t *testing.T) {
	for _, kind := range []string{"open115", "clouddrive2", "qbittorrent"} {
		previous := time.Duration(0)
		for failures := range 10 {
			delay := healthDelay(kind, failures, nil)
			if failures == 0 {
				min, max := time.Minute, 75*time.Second
				if kind == "open115" {
					min, max = time.Hour, 70*time.Minute
				}
				if delay < min || delay > max {
					t.Fatal(kind, delay)
				}
				continue
			}
			if delay < previous {
				t.Fatal("backoff fell", kind, delay)
			}
			previous = delay - 10*time.Minute
			cap := 31 * time.Minute
			if kind == "open115" {
				cap = 370 * time.Minute
			}
			if delay > cap {
				t.Fatal(kind, delay)
			}
		}
	}
	if delay := healthDelay("open115", 1, &open115.APIError{HTTPStatus: 429, RetryAt: time.Now().Add(9 * time.Hour)}); delay < 9*time.Hour-time.Second {
		t.Fatal(delay)
	}
	for _, err := range []error{context.DeadlineExceeded, open115.ErrAuthorization, open115.ErrRateLimited, &open115.APIError{Code: 40140117}, errors.New("cookie=PRIVATE https://user:password@host/?token=PRIVATE")} {
		if strings.Contains(healthMessage(err), "PRIVATE") || strings.Contains(healthMessage(err), "http") {
			t.Fatal("leaked error")
		}
	}
	if safeHealthMessage("cookie=PRIVATE") != healthMessage(errors.New("private")) {
		t.Fatal("legacy unsafe message")
	}
	if !strings.Contains(healthMessage(&open115.APIError{Code: 40140120}), "重新扫码授权") {
		t.Fatal("missing reauthorization guidance")
	}
}
