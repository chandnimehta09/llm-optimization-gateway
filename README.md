# LLM Optimization Gateway

A Go-based LLM gateway that exposes an OpenAI-compatible REST API and supports Google Gemini, OpenAI, and vLLM as backend providers, with intelligent caching and cost optimization strategies.

## Overview

The gateway acts as a translation layer between your application and multiple LLM providers:

- **Single API Format**: Applications use OpenAI SDK/format to communicate with the gateway
- **Multiple Backend Support**: Gateway translates requests to Google Gemini, OpenAI, or vLLM APIs
- **Optimization Layer**: Adds caching, routing, deduplication, and telemetry between client and provider

## Features

### Core Optimization Strategies

1. **Exact Response Caching**
   - SHA256-based deterministic cache keys
   - In-memory cache with LRU eviction
   - Configurable TTL via `GATEWAY_CACHE_DEFAULT_TTL` environment variable (default: 1 hour)
   - Cache headers (`X-Cache-Status`, `X-Cache-Key`) for observability

2. **Provider Routing with Fallback Chains**
   - Multi-provider support (Google Gemini, OpenAI, vLLM)
   - Automatic failover on provider errors
   - Load balancing across providers

3. **Context Identity System**
   - Deterministic request hashing
   - Request deduplication for identical queries
   - Prefix caching support for vLLM backends

4. **Telemetry & Observability**
   - Prometheus metrics endpoint
   - Token usage tracking
   - Cost estimation and savings reporting
   - Request latency monitoring

5. **Reliability Features**
   - Circuit breaker pattern for provider failures
   - Automatic retries with exponential backoff
   - Graceful degradation

### API Design

- **OpenAI-Compatible Endpoint**: Clients use OpenAI SDK/format to send requests
- **Backend Provider Translation**: Gateway translates OpenAI format to Gemini/OpenAI/vLLM APIs
- **Transparent Provider Switching**: Change backends without modifying client code

## Quick Start

### Run with Gemini Provider

```bash
export GATEWAY_PROVIDERS_0_API_KEY="your-gemini-api-key"
export GATEWAY_PROVIDERS_0_NAME="google"
export GATEWAY_PROVIDERS_0_BASE_URL="https://generativelanguage.googleapis.com"

go run ./cmd/gateway
```

### Run with Docker

```bash
docker build -t llm-gateway .
docker run -p 8080:8080 \
  -e GATEWAY_PROVIDERS_0_API_KEY="your-api-key" \
  -e GATEWAY_PROVIDERS_0_NAME="google" \
  llm-gateway
```

### Make a Request

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3.8-flash",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Gateway server port | `8080` |
| `GATEWAY_PROVIDERS_0_API_KEY` | Provider API key | Required |
| `GATEWAY_PROVIDERS_0_NAME` | Provider name (google, openai) | Required |
| `GATEWAY_PROVIDERS_0_BASE_URL` | Provider API URL | Provider default |
| `GATEWAY_CACHE_ENABLED` | Enable caching | `true` |
| `GATEWAY_TELEMETRY_ENABLED` | Enable telemetry | `true` |
| `GATEWAY_TELEMETRY_METRICS_PORT` | Prometheus metrics port | `9090` |

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| GET | `/ready` | Readiness check |
| POST | `/v1/chat/completions` | Chat completion (OpenAI-compatible) |
| GET | `/metrics` | Prometheus metrics |

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Client Request                          │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                   API Router (HTTP)                          │
│              OpenAI-compatible endpoint                       │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                   Cache Middleware                           │
│   • SHA256 cache key generation                              │
│   • In-memory LRU cache lookup                               │
│   • X-Cache-Status header injection                          │
└──────────────┬──────────────────────────────┬───────────────┘
               │ HIT                           │ MISS
               ▼                               ▼
        ┌──────────┐              ┌──────────────────────────┐
        │ Response │              │   Provider Router        │
        └──────────┘              │   • Fallback chain       │
                                  │   • Circuit breaker      │
                                  └────────────┬─────────────┘
                                               │
                                               ▼
                                  ┌──────────────────────────┐
                                  │   HTTP Provider          │
                                  │   • OpenAI format        │
                                  │   • Gemini format        │
                                  └────────────┬─────────────┘
                                               │
                                               ▼
                                  ┌──────────────────────────┐
                                  │   Cache Write            │
                                  │   Store response         │
                                  └──────────────────────────┘
```

## Project Structure

```
cmd/gateway/              Entry point
internal/api/             API models, validation, handlers
internal/cache/           Cache interface and implementations
  ├── memory/             In-memory LRU cache
  └── redis/              Redis cache backend
internal/provider/        Provider abstraction layer
  ├── http/               HTTP provider (OpenAI, Gemini)
  └── mock/               Mock provider for testing
internal/gateway/         Server, middleware, telemetry
internal/policy/          Routing policies, fallback chains
internal/prefix/          Prefix caching for vLLM
```

## Testing

Run the test suite:

```bash
make test
```

Run integration tests:

```bash
cd testing
bash run_tests_curl.sh
```

## Prerequisites

- Go 1.21+
- Docker (optional, for containerized builds)
- Redis (optional, for distributed caching)

## License

MIT
