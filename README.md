# API Gateway

A lightweight HTTP reverse proxy and API gateway built in Go.

## Features

- **Reverse Proxy**: Routes incoming requests to backend services based on configured paths
- **YAML Configuration**: Define routes and server settings in a simple YAML config file
- **Hot Reload**: Watches `config.yaml` for changes and reloads routes without restarting the server
- **API Key Authentication**: Secure endpoints with API key validation via `Authorization` header
- **Request Logging**: Logs all incoming requests, response times, and proxy forwarding operations
- **Rate Limiting**: Per-client rate limiting (10 requests per 60-second window), supports both in-memory and Redis-backed strategies
- **Request ID Tracking**: Adds unique X-Request-ID headers to all requests for tracing
- **Request Timeout**: 5-second timeout on upstream requests to prevent hanging connections
- **Load Balancing**: Round-robin distribution across multiple backend targets per route
- **Health Checks**: Background health checking of backend targets every 10 seconds, with automatic unhealthy target removal from rotation
- **Prometheus Metrics**: Exposes request count and duration metrics at `/metrics`
- **Graceful Shutdown**: Handles SIGINT/SIGTERM with a 10-second shutdown deadline
- **Error Handling**: Graceful handling of invalid URLs, routing errors, and timeout scenarios

## Project Structure

```
.
├── main.go          # Entry point, loads .env and config, starts server
├── server.go        # HTTP server, reverse proxy, middleware chain, hot reload
├── middleware.go     # Logger, rate limiter, API key auth, request ID, timeout
├── router.go        # Thread-safe route matching with RWMutex
├── loadbalance.go   # Round-robin load balancer with health checking
├── clients.go       # Redis client setup and rate limiter factory
├── metrics.go       # Prometheus metrics definitions
├── config.go        # YAML configuration parsing
├── config.yaml      # Server and route configuration
├── .env             # API keys and Redis address (not committed)
└── dockerfile       # Container build instructions
```

## Getting Started

### Prerequisites

- Go 1.25.0 or higher
- Redis (optional, for distributed rate limiting)

### Installation

Clone the repository and navigate to the project directory:

```bash
git clone <repository-url>
cd api-gateway
```

Install dependencies:

```bash
go mod download
```

### Environment Variables

Create a `.env` file in the project root:

```env
API_KEYS=your-api-key-1,your-api-key-2
REDIS_ADDR=localhost:6379  # optional, omit to use in-memory rate limiting
```

### Configuration

Edit `config.yaml` to define your server and routes. Each route supports multiple targets for load balancing:

```yaml
server:
  port: 8080

routes:
  - path: /api/users
    target:
      - http://localhost:3001
      - http://localhost:3002
  - path: /api/orders
    target:
      - http://localhost:4002
```

Changes to `config.yaml` are picked up automatically without restarting the server.

### Running the Server

```bash
go run .
```

The server will start listening on the configured port and forward requests to the specified target services based on the path matching.

## How It Works

1. **Load Configuration**: Reads `.env` for secrets and `config.yaml` for server/route definitions
2. **Start HTTP Server**: Listens on the configured port with graceful shutdown support
3. **Apply Middleware Chain**: Each request passes through (in order):
   - **Timeout**: Sets a 5-second deadline for upstream responses
   - **Request ID**: Adds unique X-Request-ID header for request tracing
   - **Rate Limiter**: Enforces 10 requests per 60-second window per client IP
   - **API Key Auth**: Validates the `Authorization` header against configured API keys
   - **Logger**: Logs request method, path, response status, and duration
4. **Route Requests**: Matches the URL path against configured routes
5. **Load Balance**: Selects a healthy backend target using round-robin
6. **Reverse Proxy**: Forwards matching requests to the selected backend
7. **Health Checks**: Background goroutine pings all targets every 10 seconds and removes unhealthy ones from rotation

## Example Usage

```bash
curl -H "Authorization: your-api-key-1" http://localhost:8080/api/users
```

The gateway will authenticate the request, apply rate limiting, select a healthy backend via round-robin, and forward the request. The response will include an `X-Request-ID` header for tracing.

## Dependencies

- `gopkg.in/yaml.v3` - YAML configuration parsing
- `github.com/joho/godotenv` - .env file loading
- `github.com/fsnotify/fsnotify` - File system watching for hot reload
- `github.com/redis/go-redis/v9` - Redis client for distributed rate limiting
- `github.com/prometheus/client_golang` - Prometheus metrics

## Development

- No external web frameworks — built on Go's standard `net/http` package
- Uses `httputil.ReverseProxy` for efficient request forwarding
- Thread-safe routing and load balancing with `sync.RWMutex`
