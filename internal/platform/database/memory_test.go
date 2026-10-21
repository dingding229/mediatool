package database

import (
	"context"
	"testing"
	"time"
)

func TestPoolMemoryBoundsKeepMaintenanceCapacity(t *testing.T) {
	db, err := Open(context.Background(), "postgres://synthetic:synthetic@127.0.0.1:1/media_tool_test")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := db.Config()
	if c.MaxConns != 10 || c.MinConns != 0 || c.MaxConnIdleTime > 2*time.Minute || c.ConnConfig.StatementCacheCapacity != 64 || c.ConnConfig.DescriptionCacheCapacity != 64 {
		t.Fatal("pool memory/capacity contract", c.MaxConns, c.MaxConnIdleTime, c.ConnConfig.StatementCacheCapacity)
	}
	small, err := Open(context.Background(), "postgres://synthetic:synthetic@127.0.0.1:1/media_tool_test?statement_cache_capacity=0&description_cache_capacity=8&pool_max_conn_idle_time=30s")
	if err != nil {
		t.Fatal(err)
	}
	defer small.Close()
	c = small.Config()
	if c.ConnConfig.StatementCacheCapacity != 0 || c.ConnConfig.DescriptionCacheCapacity != 8 || c.MaxConnIdleTime != 30*time.Second {
		t.Fatal("smaller explicit memory limits overwritten")
	}
}
