// Package cache is a thin JSON-over-Redis helper used to memoize read-heavy,
// rarely-changing data (food search, barcode lookups, daily summaries).
//
// Reads are best-effort: a Redis hiccup degrades to a cache miss rather than a
// request failure, so the API keeps serving from Postgres.
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache wraps a redis client with typed JSON helpers.
type Cache struct {
	rdb *redis.Client
	log *slog.Logger
}

// New parses a redis:// URL, connects, and verifies the server is reachable.
func New(ctx context.Context, redisURL string, log *slog.Logger) (*Cache, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	rdb := redis.NewClient(opt)

	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return &Cache{rdb: rdb, log: log}, nil
}

// Get unmarshals the value at key into dst. Returns true only on a clean hit.
func (c *Cache) Get(ctx context.Context, key string, dst any) bool {
	if c == nil {
		return false
	}
	b, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if err != redis.Nil {
			c.log.Warn("cache get failed", "key", key, "err", err)
		}
		return false
	}
	if err := json.Unmarshal(b, dst); err != nil {
		c.log.Warn("cache unmarshal failed", "key", key, "err", err)
		return false
	}
	return true
}

// Set stores v as JSON under key with a TTL. Failures are logged, not returned.
func (c *Cache) Set(ctx context.Context, key string, v any, ttl time.Duration) {
	if c == nil {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		c.log.Warn("cache marshal failed", "key", key, "err", err)
		return
	}
	if err := c.rdb.Set(ctx, key, b, ttl).Err(); err != nil {
		c.log.Warn("cache set failed", "key", key, "err", err)
	}
}

// Delete removes keys (best effort).
func (c *Cache) Delete(ctx context.Context, keys ...string) {
	if c == nil || len(keys) == 0 {
		return
	}
	if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
		c.log.Warn("cache delete failed", "keys", keys, "err", err)
	}
}

// Ping checks Redis connectivity (used by the readiness probe).
func (c *Cache) Ping(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("cache not initialised")
	}
	return c.rdb.Ping(ctx).Err()
}

// Close releases the underlying client.
func (c *Cache) Close() error {
	if c == nil {
		return nil
	}
	return c.rdb.Close()
}
