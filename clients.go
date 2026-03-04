package main

import (
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

func setupRedisClient(redisAddr string, redisUsername string, redisPassword string, redisDB int) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Username: redisUsername,
		Password: redisPassword,
		DB:       redisDB,
	})
	return rdb
}

func NewRateLimiter(threshold int) RateLimitStrategy {
	redisAddr := os.Getenv("REDIS_REST_URL")
	redisUsername := os.Getenv("REDIS_USERNAME")
	redisPassword := os.Getenv("REDIS_PASSWORD")
	redisDB := os.Getenv("REDIS_DB")

	if redisAddr != "" && redisPassword != "" {
		redisDB, _ := strconv.Atoi(redisDB)
		rdb := setupRedisClient(redisAddr, redisUsername, redisPassword, redisDB)
		return NewRedisRateLimiter(rdb, threshold, 60*time.Second)
	}
	inMemRateLimiter := NewInMemoryRateLimiter(threshold)
	inMemRateLimiter.startCleanup()
	return inMemRateLimiter
}
