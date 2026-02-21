package main

import (
	"fmt"
)

type Router struct {
	Routes []Route
}

type Route struct {
	Path string
	lb *LoadBalancer
}

func (router Router) findRoute(path string) (Route, error) {

	for _, route := range router.Routes {
		if route.Path == path {
			return route, nil
		}
	}
	return Route{}, fmt.Errorf("route not found: %s", path)
}
