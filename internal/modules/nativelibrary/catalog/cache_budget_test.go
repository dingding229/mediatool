package catalog

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"os"
	"strings"
	"testing"
)

func TestBrowseRedisBudgetNeverEvictsAuthorityKeys(t *testing.T) {
	raw := os.Getenv("TEST_REDIS_URL")
	if raw == "" {
		t.Skip("isolated Redis DB15")
	}
	opt, err := redis.ParseURL(raw)
	if err != nil || opt.DB != 15 {
		t.Fatal("Redis DB15 only")
	}
	client := redis.NewClient(opt)
	defer client.Close()
	ctx := context.Background()
	index, sizes := "native:browse:budget:v2:index", "native:browse:budget:v2:sizes"
	if keys, err := client.ZRange(ctx, index, 0, -1).Result(); err == nil {
		for _, k := range keys {
			if strings.HasPrefix(k, "native:browse:v2:") {
				client.Del(ctx, k)
			}
		}
	}
	client.Del(ctx, index, sizes)
	defer func() {
		keys, _ := client.ZRange(ctx, index, 0, -1).Result()
		for _, k := range keys {
			if strings.HasPrefix(k, "native:browse:v2:") {
				client.Del(ctx, k)
			}
		}
		client.Del(ctx, index, sizes, "b5:authority:lease")
	}()
	client.Set(ctx, "b5:authority:lease", "must-remain", 0)
	client.ZAdd(ctx, index, redis.Z{Score: 0, Member: "b5:authority:lease"})
	client.HSet(ctx, sizes, "b5:authority:lease", 1)
	r := &CachedRepository{redis: client}
	payload := []byte(strings.Repeat("x", 2<<20))
	for i := 0; i < 12; i++ {
		if err = r.store(ctx, fmt.Sprintf("native:browse:v2:%064x", i), payload); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := client.ZRange(ctx, index, 0, -1).Result()
	if err != nil {
		t.Fatal(err)
	}
	total := int64(0)
	for _, key := range keys {
		n, err := client.StrLen(ctx, key).Result()
		if err != nil {
			t.Fatal(err)
		}
		total += n
	}
	if total > BrowseCacheTotalBytes || len(keys) > BrowseCacheEntries {
		t.Fatal("cache bytes unbounded", total, len(keys))
	}
	if v, _ := client.Get(ctx, "b5:authority:lease").Result(); v != "must-remain" {
		t.Fatal("authority key evicted")
	}
	for i := 20; i < 100; i++ {
		if err = r.store(ctx, fmt.Sprintf("native:browse:v2:%064x", i), []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	if count, err := client.ZCard(ctx, index).Result(); err != nil || count > BrowseCacheEntries {
		t.Fatal("entry cap", count, err)
	}
}
