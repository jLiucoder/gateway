package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
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

type CounterTimestampPair struct {
	Counter   int
	TimeStamp time.Time
}

type RateLimiter struct {
	clients map[string]CounterTimestampPair
	mu      sync.Mutex
}

const threshold = 5

func (rl *RateLimiter) startCleanup() {
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

func (rateLimiter *RateLimiter) rateLimiting(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rateLimiter.mu.Lock()
		defer rateLimiter.mu.Unlock()

		clientIP := r.RemoteAddr
		pair, exists := rateLimiter.clients[clientIP]

		if !exists || time.Since(pair.TimeStamp) > 60*time.Second {
			pair = CounterTimestampPair{Counter: 1, TimeStamp: time.Now()}
		} else {
			pair.Counter++
		}
		rateLimiter.clients[clientIP] = pair

		if pair.Counter > threshold {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

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

			for _, key := range apikeys {
				if key == authHeaderKey {
					next.ServeHTTP(w, r)
					return
				}
			}

			http.Error(w, "not authorized", http.StatusUnauthorized)

		})
	}
}
