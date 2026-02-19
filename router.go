package main

import (
	"fmt"
	"log"
)

type Router struct {
	Routes []RouteConfig
}

func (router Router) findRoute(path string) (RouteConfig, error) {

	routes := router.Routes

	for _, route := range routes {
		if route.Path == path {
			log.Println("Found route based on path ", route.Path)
			return route, nil
		}

	}
	return RouteConfig{}, fmt.Errorf("route not found")
}
