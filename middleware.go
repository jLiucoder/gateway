package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func logger(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Handler Method : %s. Path : %s\n", r.Method, r.URL.Path)

		currTime := time.Now()
		wrappedWr := &responseWriter{ResponseWriter: w, status: 200}

		next.ServeHTTP(wrappedWr, r)
		timeElapsed := time.Since(currTime)

		log.Printf("request took %v to respond", timeElapsed)

		requestCount.WithLabelValues(r.Method, r.URL.Path, fmt.Sprintf("%d", wrappedWr.status)).Inc()
		requestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(timeElapsed.Seconds())
	})
}

func requestId(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 16)
		rand.Read(b)
		id := hex.EncodeToString(b)

		r.Header.Set("X-Request-ID", id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})

}

func timeout(duration time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), duration)
			defer cancel()

			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

func apiKeyAuth(apikeys []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			authHeaderKey := r.Header.Get("Authorization")
			hashAuthHeaderKey := hashKey(authHeaderKey)
			for _, storedKey := range apikeys {
				if subtle.ConstantTimeCompare([]byte(hashAuthHeaderKey), []byte(storedKey)) != 0 {
					next.ServeHTTP(w, r)
					return
				}
			}

			http.Error(w, "not authorized", http.StatusUnauthorized)
		})
	}
}

// two rate limit strategies
type RateLimitStrategy interface {
	IsAllowed(clientIp string) (bool, error)
}

func rateLimiting(strategy RateLimitStrategy) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, err := strategy.IsAllowed(r.RemoteAddr)
			if err != nil || !allowed {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type CounterTimestampPair struct {
	Counter   int
	TimeStamp time.Time
}

type InMemoryRateLimiter struct {
	clients   map[string]CounterTimestampPair
	threshold int
	mu        sync.Mutex
}

func NewInMemoryRateLimiter(threshold int) *InMemoryRateLimiter {
	return &InMemoryRateLimiter{
		clients:   make(map[string]CounterTimestampPair),
		threshold: threshold,
	}
}

func (rl *InMemoryRateLimiter) IsAllowed(clientIp string) (bool, error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	pair, exists := rl.clients[clientIp]

	if !exists || time.Since(pair.TimeStamp) > 60*time.Second {
		pair = CounterTimestampPair{Counter: 1, TimeStamp: time.Now()}
	} else {
		pair.Counter++
	}
	rl.clients[clientIp] = pair

	if pair.Counter > rl.threshold {
		return false, fmt.Errorf("rate limit exceeded")
	}
	return true, nil
}

func (rl *InMemoryRateLimiter) startCleanup() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			rl.mu.Lock()
			for ip, pair := range rl.clients {
				if time.Since(pair.TimeStamp) > 60*time.Second {
					delete(rl.clients, ip)
				}
			}
			rl.mu.Unlock()
		}
	}()
}

type RedisRateLimiter struct {
	client    *redis.Client
	threshold int
	window    time.Duration
}

func (rl *RedisRateLimiter) IsAllowed(clientIp string) (bool, error) {
	now := time.Now()
	ctx := context.Background()
	windowStart := now.Add(-rl.window).UnixNano()

	script := redis.NewScript(`
		redis.call('ZADD', KEYS[1], ARGV[1], ARGV[1])
		redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[2])
		local count = redis.call('ZCARD', KEYS[1])
		redis.call('EXPIRE', KEYS[1], ARGV[3])
		return count
	`)

	count, err := script.Run(ctx, rl.client, []string{clientIp},
		now.UnixNano(), windowStart, int(rl.window.Seconds())).Int64()

	if err != nil {
		return false, err
	}

	if count > int64(rl.threshold) {
		return false, fmt.Errorf("rate limit exceeded")
	}

	return true, nil
}

func NewRedisRateLimiter(rdb *redis.Client, threshold int, window time.Duration) *RedisRateLimiter {
	return &RedisRateLimiter{
		client:    rdb,
		threshold: threshold,
		window:    window,
	}
}
