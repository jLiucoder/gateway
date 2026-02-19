package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func startServer(config Config) {
	addr := fmt.Sprintf(":%d", config.Server.Port)

	router := Router{config.Routes}
	rl := &RateLimiter{clients: make(map[string]CounterTimestampPair)}
	http.Handle("/", rl.rateLimiting(logger(requestId(timeout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received %s \n", r.Method)

		routeFound, err := router.findRoute(r.URL.Path)

		if err != nil {
			log.Println("error finding route: ", err)
			http.Error(w, "can not find route", http.StatusNotFound)
			return
		}

		targetLink, err := url.Parse(routeFound.Target)

		if err != nil {
			fmt.Fprintln(w, "Error happend when parsing URL", err)
			log.Println("Error happend when parsing URL", err)
			return
		}
		proxy := httputil.NewSingleHostReverseProxy(targetLink)
		proxy.ServeHTTP(w, r)
		log.Printf("Forwarded %s request to target %s\n", r.Method, targetLink)
	}))))))

	log.Println("Server started listening on port", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
