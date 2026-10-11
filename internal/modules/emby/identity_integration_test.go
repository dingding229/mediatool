package emby

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisIdentityBindingsPersistAcrossGatewaysAndExpire(t *testing.T) {
	raw := os.Getenv("TEST_REDIS_URL")
	if raw == "" {
		t.Skip("isolated Redis required")
	}
	options, err := redis.ParseURL(raw)
	if err != nil {
		t.Fatal(err)
	}
	redisClient := redis.NewClient(options)
	defer redisClient.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	endpoint, _ := url.Parse("https://emby.fixture/" + t.Name() + time.Now().Format("150405.000000000"))
	key := IdentityKey(endpoint, "fixture-session")
	defer redisClient.Del(context.Background(), key)
	s := &RedisIdentityStore{Redis: redisClient}
	if err := s.Remember(ctx, key, "user-1"); err != nil {
		t.Fatal(err)
	}
	other := &RedisIdentityStore{Redis: redisClient}
	if id, err := other.Lookup(ctx, key); err != nil || id != "user-1" {
		t.Fatal("binding lost", err)
	}
	if ttl := redisClient.TTL(ctx, key).Val(); ttl <= 0 || ttl > identityTTL {
		t.Fatal("missing/bad identity expiry", ttl)
	}
	if err := other.Forget(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lookup(ctx, key); err == nil {
		t.Fatal("logout binding persisted")
	}
}
