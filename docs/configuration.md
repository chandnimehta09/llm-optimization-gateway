# Configuration Reference

The LLM Optimization Gateway is configured through environment variables and optional configuration files.

## Environment Variables

### Server

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `GATEWAY_PORT` | int | `8080` | Port for the HTTP API server |
| `METRICS_PORT` | int | `9090` | Port for metrics and health endpoints |
| `GATEWAY_HOST` | string | `0.0.0.0` | Bind address for the server |
| `READ_TIMEOUT` | duration | `30s` | HTTP read timeout |
| `WRITE_TIMEOUT` | duration | `60s` | HTTP write timeout |
| `IDLE_TIMEOUT` | duration | `120s` | HTTP idle connection timeout |
| `SHUTDOWN_TIMEOUT` | duration | `30s` | Graceful shutdown timeout |
| `LOG_LEVEL` | string | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | string | `json` | Log format: `json`, `text` |

### Cache

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CACHE_ENABLED` | bool | `true` | Enable response caching |
| `CACHE_TTL` | duration | `10m` | Default time-to-live for cache entries |
| `CACHE_KEY_PREFIX` | string | `llm-gw` | Prefix for all cache keys |
| `CACHE_BYPASS_STREAMING` | bool | `true` | Skip caching for streaming requests |
| `CACHE_MAX_ENTRY_SIZE` | int | `1048576` | Maximum cached entry size in bytes (1MB) |

### Redis

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `REDIS_URL` | string | `""` | Redis connection URL (e.g., `redis://host:6379`) |
| `REDIS_PASSWORD` | string | `""` | Redis password |
| `REDIS_DB` | int | `0` | Redis database number |
| `REDIS_MAX_RETRIES` | int | `3` | Maximum connection retries |
| `REDIS_DIAL_TIMEOUT` | duration | `5s` | Connection dial timeout |
| `REDIS_READ_TIMEOUT` | duration | `3s` | Read operation timeout |
| `REDIS_WRITE_TIMEOUT` | duration | `3s` | Write operation timeout |
| `REDIS_POOL_SIZE` | int | `10` | Connection pool size |
| `REDIS_MIN_IDLE_CONNS` | int | `2` | Minimum idle connections |

### Deduplication

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `DEDUP_ENABLED` | bool | `true` | Enable request deduplication |
| `DEDUP_WINDOW` | duration | `5s` | Time window for deduplication |
| `DEDUP_MAX_PENDING` | int | `1000` | Maximum pending deduplicated requests |

### Routing

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `ROUTING_MAX_ATTEMPTS` | int | `3` | Maximum provider attempts before failure |
| `ROUTING_TIMEOUT` | duration | `30s` | Per-provider request timeout |
| `ROUTING_FALLBACK_ENABLED` | bool | `true` | Enable provider fallback |

### Reliability

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CIRCUIT_BREAKER_THRESHOLD` | int | `5` | Failures before circuit opens |
| `CIRCUIT_BREAKER_TIMEOUT` | duration | `30s` | Time before half-open transition |
| `CIRCUIT_BREAKER_HALF_OPEN_MAX` | int | `3` | Max requests in half-open state |
| `RETRY_MAX_ATTEMPTS` | int | `3` | Maximum retry attempts |
| `RETRY_BASE_DELAY` | duration | `100ms` | Base delay between retries |
| `RETRY_MAX_DELAY` | duration | `5s` | Maximum retry delay |
| `REQUEST_TIMEOUT` | duration | `30s` | Default per-request timeout |

### Prefix Caching (vLLM)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `PREFIX_CACHE_ENABLED` | bool | `false` | Enable vLLM prefix caching |
| `PREFIX_CACHE_MIN_LENGTH` | int | `100` | Minimum prefix length in tokens |
| `PREFIX_CACHE_VLLM_URL` | string | `""` | vLLM server URL |

### Observability

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `OTEL_EXPORTER_ENDPOINT` | string | `""` | OpenTelemetry exporter endpoint |
| `OTEL_SERVICE_NAME` | string | `llm-optimization-gateway` | Service name for telemetry |
| `OTEL_SAMPLE_RATE` | float | `1.0` | Trace sampling rate (0.0–1.0) |

## Duration Format

Duration values use Go's duration format: `100ms`, `5s`, `10m`, `1h`.

## Example Configuration

```bash
# Server
export GATEWAY_PORT=8080
export METRICS_PORT=9090
export LOG_LEVEL=info

# Cache + Redis
export CACHE_ENABLED=true
export CACHE_TTL=10m
export REDIS_URL=redis://localhost:6379

# Deduplication
export DEDUP_ENABLED=true
export DEDUP_WINDOW=5s

# Routing
export ROUTING_MAX_ATTEMPTS=3
export ROUTING_FALLBACK_ENABLED=true

# Reliability
export CIRCUIT_BREAKER_THRESHOLD=5
export RETRY_MAX_ATTEMPTS=3
```

## Docker Environment

When running with Docker, pass variables in your `docker-compose.yml`:

```yaml
services:
  gateway:
    image: llm-optimization-gateway:latest
    environment:
      - GATEWAY_PORT=8080
      - REDIS_URL=redis://redis:6379
      - CACHE_ENABLED=true
      - LOG_LEVEL=info
```

Or use an environment file:

```bash
docker run --env-file .env llm-optimization-gateway:latest
```
