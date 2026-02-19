# API Gateway

A lightweight HTTP reverse proxy and API gateway built in Go.


## Features

- **Reverse Proxy**: Routes incoming requests to backend services based on configured paths
- **YAML Configuration**: Define routes and server settings in a simple YAML config file
- **API Key Authentication**: Secure endpoints with API key validation
- **Request Logging**: Logs all incoming requests, response times, and proxy forwarding operations
- **Rate Limiting**: Per-client rate limiting to prevent abuse (5 requests per 60-second window)
- **Request ID Tracking**: Adds unique X-Request-ID headers to all requests for tracing
- **Request Timeout**: 5-second timeout on upstream requests to prevent hanging connections
- **Load Balancing**: Distributes requests across multiple backend targets
- **Prometheus Metrics**: Exposes metrics at `/metrics` for monitoring
- **Error Handling**: Graceful handling of invalid URLs, routing errors, and timeout scenarios

## Project Structure

```
.
├── main.go         # Entry point, loads config and starts server
├── server.go       # HTTP server and reverse proxy logic
├── middleware.go   # Logger, rate limiter, request ID, and timeout middleware
├── router.go       # Route matching logic
├── config.go       # Configuration parsing (YAML)
├── config.yaml     # Server and route configuration
├── go.mod          # Go module definition
├── go.sum          # Go dependencies checksums
└── README.md       # This file
```

## Getting Started

### Prerequisites

- Go 1.25.0 or higher

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

### Configuration

Edit `config.yaml` to define your server and routes:

```yaml
server:
  port: 8080

routes:
  - path: /api/users
    target:
      - http://localhost:3001
  - path: /api/orders
    target:
      - http://localhost:3002
  - path: /api/users/pages
    target:
      - http://localhost:3003
```

Each route maps a request path to a backend service target.

### Running the Server

```bash
go run .
```

The server will start listening on the configured port and forward requests to the specified target services based on the path matching.

## How It Works

1. **Load Configuration**: Reads `config.yaml` for server settings and route definitions
2. **Start HTTP Server**: Listens on the configured port
3. **Apply Middleware Chain**: Each request passes through (in order):
   - **Logger**: Logs request method, path, and response time
   - **API Key Auth**: Validates requests against configured API keys
   - **Rate Limiter**: Enforces 5 requests per 60-second window per client IP
   - **Request ID**: Adds unique X-Request-ID header for request tracing
   - **Timeout**: Sets a 5-second deadline for upstream responses
4. **Route Requests**: Matches the URL path against configured routes
5. **Reverse Proxy**: Forwards matching requests to the target backend service
6. **Error Handling**: Returns appropriate HTTP error codes (401 for invalid API key, 404 for not found, 429 for rate limit, 504 for timeout)

## Example Usage

If configured with:

```yaml
server:
  port: 8080
routes:
  - path: /api/users
    target:
      - http://localhost:3001
```

A request to `http://localhost:8080/api/users` will be forwarded to `http://localhost:3001/api/users`.

Each request will:

1. Be validated with an API key (if configured)
2. Get a unique X-Request-ID header
3. Be logged with method and path
4. Be subject to rate limiting (max 5 requests per 60 seconds per client)
5. Have a 5-second timeout for the upstream response
6. Get response time logged

## Dependencies

- `gopkg.in/yaml.v3` - YAML configuration parsing

## Development

- No external web frameworks used — leverages Go's standard `net/http` package
- Uses `httputil.ReverseProxy` for efficient request forwarding


## Future Enhancements

- Path-based request routing patterns (wildcards, regex)
- Custom middleware support
- Request/response manipulation
- Authentication/authorization
- Circuit breaker pattern

README.md is written by Github Copilot
