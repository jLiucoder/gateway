package main

import (
	"crypto/tls"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

func setupRedisClient(RedisAddr string, RedisToken string) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:      RedisAddr,
		Password:  RedisToken,
		TLSConfig: &tls.Config{},
	})
	return rdb
}

func NewRateLimiter(threshold int) RateLimitStrategy {
	redisAddr := os.Getenv("REDIS_REST_URL")
	redisToken := os.Getenv("REDIS_REST_TOKEN")

	if redisAddr != "" && redisToken != "" {
		rdb := setupRedisClient(redisAddr, redisToken)
		return NewRedisRateLimiter(rdb, threshold, 60*time.Second)
	}
	inMemRateLimiter := NewInMemoryRateLimiter(threshold)
	inMemRateLimiter.startCleanup()
	return inMemRateLimiter
}
