# API Gateway

A lightweight API gateway and intelligent LLM router built in Go. Routes HTTP traffic to backend services with load balancing, and provides a smart `/smart/completion` endpoint that classifies queries by complexity and routes them to the appropriate LLM provider/model.

## Features

### Reverse Proxy
- Path-based routing to backend services
- Round-robin load balancing across multiple targets
- Background health checks every 10 seconds with automatic unhealthy target removal
- Hot reload — edit `config.yaml` and routes update without restart

### Smart LLM Routing
- Unified `/smart/completion` endpoint that accepts OpenAI-style chat requests
- Classifies queries into complexity tiers (simple / medium / complex) using a lightweight LLM
- Routes to different providers and models based on tier
- Supports **Anthropic** (native SDK) and **OpenAI-compatible** APIs (OpenAI, Kimi/Moonshot, DeepSeek, Groq, etc.)
- Streaming via Server-Sent Events (SSE)
- Extended thinking support for Anthropic models
- Tool use / function calling

### Semantic Cache
- Caches LLM responses using Redis VecSets + MiniLM-L6-v2 embeddings
- Semantically similar queries return cached results instantly
- Similarity threshold: 0.97 (configurable)
- Cache TTL: 1 hour
- Non-streaming requests only
- Async cache writes to avoid blocking responses
- `X-Cache: HIT/MISS` response header

### Middleware
- **API key auth** — SHA256-hashed keys via `Authorization` header
- **Rate limiting** — per-IP, 30 req/60s window, in-memory or Redis-backed
- **Request ID** — unique `X-Request-ID` on every request
- **Timeout** — 200-second deadline on upstream requests
- **Logging** — method, path, status, duration
- **Prometheus metrics** — `gateway_requests_total` and `gateway_request_duration_seconds` at `/metrics`

## Project Structure

```
.
├── main.go           # Entry point, loads .env and config
├── server.go         # HTTP server, endpoints, middleware chain, hot reload
├── llm.go            # LLM types, provider interface, classifier, provider factory
├── anthropic.go      # Anthropic provider (thinking, tools, streaming)
├── openaicompat.go   # OpenAI-compatible provider (OpenAI, Kimi, etc.)
├── cache.go          # Semantic cache with Redis VecSets + hugot embeddings
├── middleware.go      # Logger, auth, rate limiter, request ID, timeout
├── router.go         # Thread-safe route matching
├── loadbalance.go    # Round-robin load balancer with health checks
├── clients.go        # Redis client setup, rate limiter factory
├── metrics.go        # Prometheus metric definitions
├── config.go         # YAML config parsing
├── util.go           # SHA256 hashing utility
├── config.yaml       # Server, routes, and LLM provider configuration
├── dockerfile        # Multi-stage Docker build
└── .env              # Secrets (not committed)
```

## Getting Started

### Prerequisites

- Go 1.25+
- Redis 8.0+ (optional — for rate limiting and semantic cache with VecSets)

### Installation

```bash
git clone https://github.com/jLiucoder/gateway.git
cd gateway
go mod download
```

### Environment Variables

Create a `.env` file:

```env
API_KEYS=hashed-key-1,hashed-key-2

# Redis (optional)
REDIS_REST_URL=localhost:6379
REDIS_USERNAME=default
REDIS_PASSWORD=your-password
REDIS_DB=0

# LLM provider keys
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
KIMI_API_KEY=sk-...
```

Use the `/encode` endpoint to hash raw API keys:

```bash
curl -X POST http://localhost:8080/encode \
  -H "Content-Type: application/json" \
  -d '{"key": "my-raw-api-key"}'
```

### Configuration

```yaml
server:
  port: 8080

routes:
  - path: /api/users
    target:
      - http://localhost:3001
      - http://localhost:3002

llm:
  classifier:
    provider: openai
    model: gpt-4.1-nano

  providers:
    openai:
      type: openai-compat
      base_url: https://api.openai.com/v1
      api_key_env: OPENAI_API_KEY
      models: [gpt-4.1-nano, gpt-5-mini]

    anthropic:
      type: anthropic
      api_key_env: ANTHROPIC_API_KEY
      models: [claude-sonnet-4-6]

    kimi:
      type: openai-compat
      base_url: https://api.moonshot.ai/v1
      api_key_env: KIMI_API_KEY
      models: [moonshot-v1-8k]

  tiers:
    simple:
      provider: openai
      model: gpt-5-mini
    medium:
      provider: openai
      model: gpt-5
    complex:
      provider: anthropic
      model: claude-sonnet-4-6
```

### Run

```bash
go run .
```

## Endpoints

| Endpoint | Description |
|---|---|
| `/*` | Reverse proxy — matches configured routes, load balances to targets |
| `/smart/completion` | LLM routing — classifies query, routes to appropriate provider/model |
| `/health` | Health check |
| `/encode` | Hash a raw API key for use in `API_KEYS` env var |
| `/metrics` | Prometheus metrics |

## Usage

### Proxy request

```bash
curl -H "Authorization: your-api-key" http://localhost:8080/api/users
```

### LLM completion

```bash
curl -X POST http://localhost:8080/smart/completion \
  -H "Authorization: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [{"role": "user", "content": "What is 2+2?"}]
  }'
```

### Streaming

```bash
curl -X POST http://localhost:8080/smart/completion \
  -H "Authorization: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [{"role": "user", "content": "Write a poem"}],
    "stream": true
  }'
```

## How It Works

```
Client Request
  │
  ├─ Proxy routes (/*):
  │    Middleware → Route match → Round-robin target → Reverse proxy
  │
  └─ LLM route (/smart/completion):
       Middleware → Semantic cache lookup
         ├─ HIT  → Return cached response
         └─ MISS → Classify complexity → Select tier → Call LLM provider
                    → Cache response (async) → Return response
```

## Dependencies

- `github.com/anthropics/anthropic-sdk-go` — Anthropic API
- `github.com/openai/openai-go` — OpenAI API
- `github.com/redis/go-redis/v9` — Redis (rate limiting + semantic cache)
- `github.com/knights-analytics/hugot` — ONNX embeddings for semantic cache
- `github.com/prometheus/client_golang` — Prometheus metrics
- `github.com/fsnotify/fsnotify` — Config hot reload
- `gopkg.in/yaml.v3` — YAML parsing
- `github.com/joho/godotenv` — .env loading

## Design Notes

- No web frameworks — built on Go's `net/http` and `httputil.ReverseProxy`
- Thread-safe routing and load balancing with `sync.RWMutex`
- LLM providers implement a common `LLMProvider` interface — easy to add new ones
- Semantic cache gracefully degrades — if Redis or hugot init fails, the server runs without caching
- MiniLM model (~80MB) auto-downloads on first startup to `./models/`
