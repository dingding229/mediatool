package cache

import (
	"github.com/redis/go-redis/v9"
	"time"
)

func Open(rawURL string) (*redis.Client, error) {
	if rawURL == "" {
		return nil, nil
	}
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, err
	}
	options.DialTimeout = 2 * time.Second
	options.ReadTimeout = time.Second
	options.WriteTimeout = time.Second
	options.MaxRetries = 0
	options.ContextTimeoutEnabled = true
	options.PoolSize = 10
	return redis.NewClient(options), nil
}
