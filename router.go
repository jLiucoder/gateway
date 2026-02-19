package main

import (
	"fmt"
)

type Router struct {
	Routes []RouteConfig
}

func (router Router) findRoute(path string) (RouteConfig, error) {

	for _, route := range router.Routes {
		if route.Path == path {
			return route, nil
		}

	}
	return RouteConfig{}, fmt.Errorf("route not found: %s", path)
}
