package main

import (
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

func setupRedisClient(RedisAddr string) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: RedisAddr,
	})
	return rdb
}

func NewRateLimiter(threshold int) RateLimitStrategy {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr != "" {
		rdb := setupRedisClient(redisAddr)
		return NewRedisRateLimiter(rdb, threshold, 60*time.Second)
	}
	inMemRateLimiter := NewInMemoryRateLimiter(threshold)
	inMemRateLimiter.startCleanup()
	return inMemRateLimiter
}
