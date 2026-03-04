package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const TIMEOUTDURATION = 200
const RLThreshold = 30

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

	//llm request handler
	providers := buildProviders(config.LLMConfig)
	classifierProvider := providers[config.LLMConfig.Classifier.Provider]

	//semantic cache
	var cache *SemanticCache
	redisAddr := os.Getenv("REDIS_REST_URL")
	if redisAddr != "" {
		redisDB, _ := strconv.Atoi(os.Getenv("REDIS_DB"))
		cacheRdb := setupRedisClient(redisAddr, os.Getenv("REDIS_USERNAME"), os.Getenv("REDIS_PASSWORD"), redisDB)
		c, err := NewSemanticCache(cacheRdb, defaultThreshold)
		if err != nil {
			log.Printf("[cache] failed to init semantic cache: %v (continuing without cache)", err)
		} else {
			cache = c
			log.Println("[cache] semantic cache initialized")
		}
	}

	http.Handle("/smart/completion", chain(llmHandler(providers, config.LLMConfig.Classifier.Model, classifierProvider, config.LLMConfig.Tiers, cache),
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

	//test encoding handler for apikeys
	http.Handle("/encode", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		var req struct {
			Key string `json:"key"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
		}

		hashed := hashKey(req.Key)

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, hashed)
	}))

	//metrics
	http.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr: addr,
	}
	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
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

func llmHandler(providers map[string]LLMProvider, classifierModel string, classifierProvider LLMProvider, tiers map[string]TierConfig, cache *SemanticCache) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req LLMRequest
		err := json.NewDecoder(r.Body).Decode(&req)

		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		// semantic cache lookup (non-streaming only)
		queryText := extractQueryText(req.Messages)
		if !req.Stream && cache != nil && queryText != "" {
			if cached, hit := cache.Lookup(queryText); hit {
				w.Header().Set("X-Cache", "HIT")
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(cached)
				return
			}
		}

		tier, err := classify(classifierModel, classifierProvider, req.Messages, r.Context())

		if err != nil {
			log.Println(err)
			w.Header().Set("X-Classification-Fallback", "medium")
		}

		chosenTierConfig, ok := tiers[tier]
		if !ok {
			log.Printf("unknown tier %q, falling back to medium", tier)
			chosenTierConfig = tiers["medium"]
		}
		log.Printf("[routing] tier=%s provider=%s model=%s", tier, chosenTierConfig.Provider, chosenTierConfig.Model)

		req.Model = chosenTierConfig.Model

		if req.Stream {
			log.Printf("[streaming]: True")

			w.Header().Set("Content-Type", "text/event-stream")

			resp, err := providers[chosenTierConfig.Provider].ChatStream(r.Context(), req)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			flusher := w.(http.Flusher)
			for event := range resp {
				marshalBytes, err := json.Marshal(event)
				if err != nil {
					return
				}

				fmt.Fprintf(w, "data: %s\n\n", marshalBytes)
				flusher.Flush()
			}
		} else {

			resp, err := providers[chosenTierConfig.Provider].Chat(r.Context(), req)

			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			//add usage logging
			log.Printf("[usage] input=%d output=%d total=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.TotalTokens)

			// store in cache async
			if cache != nil && queryText != "" {
				go cache.Store(queryText, resp)
			}

			w.Header().Set("X-Cache", "MISS")
			w.Header().Set("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(resp)
			if err != nil {
				return
			}
		}
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

				(*cancelHealthCheck)() // stop old goroutine
				ctx, newCancel := context.WithCancel(context.Background())
				*cancelHealthCheck = newCancel
				StartHealthCheck(ctx, routes)
			}
		}
	}()

}
