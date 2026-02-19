package main

import (
	"sync"
)

type LoadBalancingStrategy interface {
	NextTarget() string
}

type LoadBalancer struct{
	strategy LoadBalancingStrategy
}

//RoundRobin strategy section
type RoundRobin struct{
	targets []string
	current int
	mu sync.Mutex
}

func (rr *RoundRobin) NextTarget() string {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	target := rr.targets[rr.current]

	//update current to be the next one
	rr.current = (rr.current + 1) % len(rr.targets)

	return target
}