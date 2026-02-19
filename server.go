package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const TIMEOUTDURATION = 5

func startServer(config Config) {
	addr := fmt.Sprintf(":%d", config.Server.Port)

	router := Router{config.Routes}

	handler := proxyHandler(router)
	rateLimiter := &RateLimiter{clients: make(map[string]CounterTimestampPair)}

	//proxy forwarding
	http.Handle("/", chain(handler,
		logger,
		apiKeyAuth(config.ApiKeys),
		rateLimiter.rateLimiting,
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

	log.Println("Server started listening on port", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func chain(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

func proxyHandler(router Router) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		
		log.Printf("Received %s \n", r.Method)
		routeFound, err := router.findRoute(r.URL.Path)

		if err != nil {
			log.Println("error finding route: ", err)
			http.Error(w, "can not find route", http.StatusNotFound)
			return
		}
		//return the targetLink from load balancer
		targetLink, err := url.Parse(routeFound.lb.strategy.NextTarget())

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
