package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const TIMEOUTDURATION = 5
const RLThreshold = 10

func startServer(config Config) {

	addr := fmt.Sprintf(":%d", config.Server.Port)

	routes := buildRoutes(config)

	router := NewRouter(routes)

	ctx, cancel := context.WithCancel(context.Background())
	StartHealthCheck(ctx, routes)

	watchConfig("config.yaml", &config, router, &cancel)

	handler := proxyHandler(router)
	//create new strategy of ratelimiter, we dont care which RL we really use, it all depends on the env var
	rateLimiter := NewRateLimiter(RLThreshold)

	//proxy forwarding
	http.Handle("/", chain(handler,
		logger,
		apiKeyAuth(config.ApiKeys),
		rateLimiting(rateLimiter),
		requestId,
		timeout(TIMEOUTDURATION*time.Second),
	))

	//health check
	http.Handle("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, "healthy")
	}))

	//metrics
	http.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr: addr,
	}

	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	log.Println("Server started listening on port", addr)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("shutdown error:", err)
	}
	log.Println("server shut down cleanly")
}

func chain(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

func proxyHandler(router *Router) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		log.Printf("Received %s \n", r.Method)
		routeFound, err := router.findRoute(r.URL.Path)

		if err != nil {
			log.Println("error finding route: ", err)
			http.Error(w, "can not find route", http.StatusNotFound)
			return
		}
		//return the targetLink from load balancer
		tempLink, err := routeFound.lb.strategy.NextTarget()
		if err != nil {
			log.Println("error finding next target: ", err)
			http.Error(w, "error finding next target", http.StatusBadGateway)
			return
		}
		targetLink, err := url.Parse(tempLink)

		if err != nil {
			log.Println("error happened when parsing URL: ", err)
			http.Error(w, "error happened when parsing URL", http.StatusBadGateway)
			return
		}
		proxy := httputil.NewSingleHostReverseProxy(targetLink)
		proxy.ServeHTTP(w, r)
		log.Printf("Forwarded %s request to target %s\n", r.Method, targetLink)
	})
}

func buildRoutes(config Config) []Route {
	routes := make([]Route, len(config.Routes))

	for i, rc := range config.Routes {
		targets := make([]*Target, len(rc.Target))
		for j, target := range rc.Target {
			targets[j] = &Target{URL: target, healthy: true}
		}

		routes[i] = Route{
			Path: rc.Path,
			lb:   &LoadBalancer{strategy: &RoundRobin{targets: targets}},
		}
	}
	slices.SortFunc(routes, func(i, j Route) int {
		return len(j.Path) - len(i.Path)
	})
	return routes
}

func watchConfig(path string, config *Config, router *Router, cancelHealthCheck *context.CancelFunc) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal("Failed to create watcher:", err)
	}
	if err := watcher.Add(path); err != nil {
		log.Fatal("Failed to add path to watcher:", err)
	}

	go func() {
		defer watcher.Close()
		for event := range watcher.Events {
			if event.Op&fsnotify.Write == fsnotify.Write {
				newConfig, err := loadConfig("config.yaml", config.ApiKeys)
				if err != nil {
					log.Println("error reloading config:", err)
					continue
				}
				*config = newConfig

				routes := buildRoutes(*config)
				router.updateRoutes(routes)

				(*cancelHealthCheck)()  // stop old goroutine
                ctx, newCancel := context.WithCancel(context.Background())
                *cancelHealthCheck = newCancel
                StartHealthCheck(ctx, routes)
			}
		}
	}()

}
