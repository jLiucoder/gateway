package main

import (
	"fmt"
	"strings"
	"sync"
)

type Router struct {
	Routes []Route
	mu     sync.RWMutex
}

func NewRouter(routes []Route) *Router {
	return &Router{
		Routes: routes,
	}
}

type Route struct {
	Path string
	lb   *LoadBalancer
}

func (router *Router) findRoute(path string) (Route, error) {
	router.mu.RLock()
	defer router.mu.RUnlock()

	for _, route := range router.Routes {
		if strings.HasPrefix(path, route.Path) {
			return route, nil
		}
	}
	return Route{}, fmt.Errorf("route not found: %s", path)
}

func (r *Router) updateRoutes(routes []Route) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Routes = routes
}
