# LLM Optimization Gateway

A lightweight HTTP gateway for routing and optimizing LLM API requests.

## Prerequisites

- Go 1.21+
- Docker and Docker Compose (optional, for containerized builds)

## Quick Start

### Run locally

```bash
go run ./cmd/gateway
```

The server starts on port 8080 by default. Set the `PORT` environment variable to override:

```bash
PORT=9090 go run ./cmd/gateway
```

### Build and run

```bash
make build
./bin/gateway
```

### Run tests

```bash
make test
```

### Docker

Build the Docker image:

```bash
make docker-build
```

Start the service with Docker Compose:

```bash
make docker-up
```

## Endpoints

| Method | Path      | Description          |
|--------|-----------|----------------------|
| GET    | `/health` | Health check         |
| GET    | `/ready`  | Readiness check      |

### Health Check

```bash
curl http://localhost:8080/health
# {"status":"healthy"}
```

### Readiness Check

```bash
curl http://localhost:8080/ready
# {"ready":true}
```

## Project Structure

```
cmd/gateway/         Entry point
internal/gateway/    Server and handler implementation
internal/api/        API models and validation
```
