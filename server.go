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

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received %s \n", r.Method)

		routes := config.Routes

		for _, route := range routes {
			if route.Path == r.URL.Path {

				targetLink, err := url.Parse(route.Target)

				if err != nil {
					fmt.Fprintln(w, "Error happend when parsing URL", err)
					log.Println("Error happend when parsing URL", err)
				}
				proxy := httputil.NewSingleHostReverseProxy(targetLink)
				proxy.ServeHTTP(w, r)
				log.Printf("Forwarded %s request to target %s\n", r.Method, targetLink)
			}

		}
	})

	log.Println("Server started listening on port", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
