package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

type LoadBalancingStrategy interface {
	NextTarget() (string, error)
	CheckHealth()
}

type LoadBalancer struct {
	strategy LoadBalancingStrategy
}

// RoundRobin strategy section
type RoundRobin struct {
	targets []*Target
	current int
	mu      sync.Mutex
}

type Target struct {
	URL     string
	healthy bool
	mu      sync.Mutex
}

func (t *Target) setHealthy(h bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.healthy = h
}

func (t *Target) isHealthy() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.healthy
}

func (rr *RoundRobin) NextTarget() (string, error) {
	rr.mu.Lock()
	defer rr.mu.Unlock()

	for i := 0; i < len(rr.targets); i++ {

		target := rr.targets[rr.current]
		rr.current = (rr.current + 1) % len(rr.targets)
		if target.isHealthy() {
			return target.URL, nil
		}
	}
	return "", fmt.Errorf("no healthy targets available")
}

func (rr *RoundRobin) CheckHealth() {

	for _, target := range rr.targets {
		resp, err := http.Get(target.URL)

		if err != nil {
			log.Printf("%s is not healthy\n", target.URL)
			target.setHealthy(false)
			continue
		}
		//we need to close the connection manually in go
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Printf("%s is not healthy\n", target.URL)
			target.setHealthy(false)
		} else {
			log.Printf("%s is healthy\n", target.URL)
			target.setHealthy(true)
		}
	}
}

func StartHealthCheck(ctx context.Context, routes []Route) {

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
            select {
            case <-ticker.C:
                for _, route := range routes {
                    route.lb.strategy.CheckHealth()
                }
            case <-ctx.Done():
                return
            }
        }
	}()

}
