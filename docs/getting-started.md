# Getting Started

This guide walks you through setting up and running the LLM Optimization Gateway for the first time.

## Prerequisites

- **Go 1.26+** installed ([download](https://go.dev/dl/))
- **Docker** and **Docker Compose** (for production deployment)
- **Redis 7+** (for caching — optional for local development)

## Quick Start

### 1. Clone the Repository

```bash
git clone https://github.com/llm-optimization-gateway.git
cd llm-optimization-gateway
```

### 2. Install Dependencies

```bash
go mod download
```

### 3. Run the Gateway

**Local development (no Redis):**

```bash
go run cmd/gateway/main.go
```

**With Docker Compose (recommended):**

```bash
docker compose -f docker-compose.prod.yml up -d
```

The gateway starts on port `8080` by default.

### 4. Make Your First Request

Send a chat completion request using curl:

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "What is the capital of France?"}
    ],
    "temperature": 0.7,
    "max_tokens": 100
  }'
```

You should receive a response like:

```json
{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "model": "gpt-4",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "The capital of France is Paris."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 20,
    "completion_tokens": 8,
    "total_tokens": 28
  }
}
```

### 5. Check Gateway Health

```bash
curl http://localhost:8080/health
```

## Project Structure

```
llm-optimization-gateway/
├── cmd/                    # Application entry points
│   ├── gateway/            # Main gateway server
│   └── benchmark/          # Benchmark runner
├── internal/               # Internal packages
│   ├── api/                # HTTP API layer
│   ├── cache/              # Cache abstraction (Redis, in-memory)
│   ├── context/            # Request context identity & hashing
│   ├── dedup/              # Content deduplication
│   ├── gateway/            # Gateway middleware pipeline
│   ├── policy/             # Policy engine for optimization decisions
│   ├── prefix/             # Prefix caching with vLLM
│   ├── reliability/        # Circuit breaker, retry, timeout
│   ├── request/            # Request normalization
│   └── routing/            # Provider routing & fallback
├── test/                   # Test suites
│   └── e2e/                # End-to-end tests
├── docs/                   # Documentation
└── scripts/                # Operational scripts
```

## Configuration

The gateway is configured through environment variables. See the [Configuration Reference](configuration.md) for all options.

Key environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `GATEWAY_PORT` | `8080` | HTTP server port |
| `METRICS_PORT` | `9090` | Metrics/health port |
| `REDIS_URL` | `""` | Redis connection URL |
| `CACHE_ENABLED` | `true` | Enable response caching |
| `CACHE_TTL` | `10m` | Default cache TTL |
| `DEDUP_ENABLED` | `true` | Enable request deduplication |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |

## Next Steps

- [Configuration Reference](configuration.md) — all configuration options
- [API Documentation](api.md) — endpoint reference
- [Optimizations Guide](optimizations.md) — caching, dedup, prefix caching
- [Architecture Overview](architecture.md) — system design
- [Operations Guide](operations.md) — running in production
