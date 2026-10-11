package linkcache

import (
	"media-tool/internal/modules/storage"
	"testing"
	"time"
)

func TestProgramCacheBoundIsNotProviderExpiry(t *testing.T) {
	link := storage.DownloadLink{CacheUntil: time.Now().Add(45 * time.Second)}
	if ttl := cacheTTL(link); ttl <= 0 || ttl > 40*time.Second {
		t.Fatal("program bound not applied", ttl)
	}
	if !link.ExpiresAt.IsZero() {
		t.Fatal("provider expiry was invented")
	}
	link.ExpiresAt = time.Now().Add(10 * time.Second)
	if cacheTTL(link) > 0 {
		t.Fatal("program TTL extended expiring provider URL")
	}
	if cacheTTL(storage.DownloadLink{}) != 0 {
		t.Fatal("unknown lifetime cached")
	}
}
