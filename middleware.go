package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"sync"
	"time"
)

func logger(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Handler Method : %s. Path : %s\n", r.Method, r.URL.Path)

		currTime := time.Now()
		next.ServeHTTP(w, r)
		timeElapsed := time.Since(currTime)

		log.Printf("request took %v to respond", timeElapsed)
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

func (rateLimiter *RateLimiter) rateLimiting(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rateLimiter.mu.Lock()
		defer rateLimiter.mu.Unlock()
		clientMap := rateLimiter.clients
		clientIP := r.RemoteAddr
		pair, exists := clientMap[clientIP]

		if !exists || time.Since(pair.TimeStamp) > 60*time.Second {
			pair = CounterTimestampPair{Counter: 0, TimeStamp: time.Now()}
		} else {
			pair.Counter++
		}
		clientMap[clientIP] = pair

		if pair.Counter > threshold {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func timeout(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)

		if ctx.Err() == context.DeadlineExceeded {
			http.Error(w, "upstream timeout", http.StatusGatewayTimeout)
		}
	})
}
