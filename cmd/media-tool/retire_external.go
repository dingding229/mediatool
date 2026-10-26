package main

import (
	"context"
	"github.com/redis/go-redis/v9"
)

// Only the obsolete external proxy identity cache is removed. No shared token,
// playback link, plugin or 115 namespace is included in this scan.
func retireExternalIdentityCache(ctx context.Context, r *redis.Client) error {
	var cursor uint64
	for {
		keys, next, err := r.Scan(ctx, cursor, "mediatool:emby:identity:v1:*", 128).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err = r.Unlink(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		if next == 0 {
			return nil
		}
		cursor = next
	}
}
