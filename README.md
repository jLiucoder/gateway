# API Gateway

A lightweight HTTP reverse proxy and API gateway built in Go.

## Features

- **Reverse Proxy**: Routes incoming requests to backend services based on configured paths
- **YAML Configuration**: Define routes and server settings in a simple YAML config file
- **Request Logging**: Logs all incoming requests and proxy forwarding operations
- **Error Handling**: Graceful handling of invalid URLs and routing errors

## Project Structure

```
.
├── main.go        # Entry point, loads config and starts server
├── server.go      # HTTP server and reverse proxy logic
├── config.go      # Configuration parsing (YAML)
├── go.mod         # Go module definition
├── go.sum         # Go dependencies checksums
├── config.yaml    # Server and route configuration
└── README.md      # This file
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
    target: http://users-service:3001
  - path: /api/products
    target: http://products-service:3002
```

### Running the Server

```bash
go run .
```

The server will start listening on the configured port and forward requests to the specified target services based on the path matching.

## How It Works

1. **Load Configuration**: Reads `config.yaml` for server settings and route definitions
2. **Start HTTP Server**: Listens on the configured port
3. **Route Requests**: For each incoming request, matches the URL path against configured routes
4. **Reverse Proxy**: Forwards matching requests to the target backend service
5. **Log Activity**: Logs all requests and proxy operations for debugging

## Example Usage

If configured with:

```yaml
server:
  port: 8080
routes:
  - path: /home/jiayuliu
    target: localhost:8080
```

A request to `http://localhost:8080/home/jiayuliu` will be forwarded to `localhost:8080`.

## Dependencies

- `gopkg.in/yaml.v3` - YAML configuration parsing

## Development

- No external web frameworks used — leverages Go's standard `net/http` package
- Uses `httputil.ReverseProxy` for efficient request forwarding

## Future Enhancements

- Load balancing across multiple backend targets
- Request/response middleware
- Rate limiting
- Authentication/authorization
- Metrics and monitoring

README.md is written by Github Copilot
