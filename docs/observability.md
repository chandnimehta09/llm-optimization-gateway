# Observability Guide

The LLM Optimization Gateway provides comprehensive observability through structured logging, metrics, and distributed tracing.

## Logging

### Log Format

Logs are output in JSON format by default for machine parsing:

```json
{
  "timestamp": "2024-01-15T10:30:00.123Z",
  "level": "info",
  "message": "request completed",
  "request_id": "req-abc123",
  "method": "POST",
  "path": "/v1/chat/completions",
  "status": 200,
  "duration_ms": 1250,
  "cache_status": "MISS",
  "provider": "openai",
  "model": "gpt-4",
  "prompt_tokens": 150,
  "completion_tokens": 50
}
```

### Log Levels

| Level | Description |
|-------|-------------|
| `debug` | Detailed diagnostic information including cache keys and routing decisions |
| `info` | Request lifecycle events, cache hits/misses, provider selections |
| `warn` | Degraded behavior: fallbacks, retries, circuit breaker state changes |
| `error` | Request failures, provider errors, cache backend failures |

Configure with `LOG_LEVEL` environment variable.

### Key Log Fields

| Field | Description |
|-------|-------------|
| `request_id` | Unique request identifier for correlation |
| `cache_status` | `HIT`, `MISS`, or `BYPASS` |
| `provider` | Selected provider name |
| `model` | Resolved model name |
| `duration_ms` | Total request duration in milliseconds |
| `prompt_tokens` | Input token count |
| `completion_tokens` | Output token count |
| `fallback_count` | Number of fallback attempts |
| `dedup_coalesced` | Whether request was deduplicated |

## Metrics

Metrics are exposed in Prometheus format on the metrics port (default: 9090).

### Request Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gateway_requests_total` | counter | `method`, `path`, `status` | Total requests |
| `gateway_request_duration_seconds` | histogram | `method`, `path`, `status` | Request duration |
| `gateway_request_size_bytes` | histogram | `method`, `path` | Request body size |
| `gateway_response_size_bytes` | histogram | `method`, `path` | Response body size |

### Cache Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gateway_cache_operations_total` | counter | `operation`, `status` | Cache operations (get/set/delete) |
| `gateway_cache_hit_rate` | gauge | | Current cache hit ratio |
| `gateway_cache_entries` | gauge | | Current cache entry count |
| `gateway_cache_size_bytes` | gauge | | Approximate cache memory usage |
| `gateway_cache_latency_seconds` | histogram | `operation` | Cache operation latency |

### Provider Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gateway_provider_requests_total` | counter | `provider`, `status` | Provider requests |
| `gateway_provider_latency_seconds` | histogram | `provider` | Provider response latency |
| `gateway_provider_errors_total` | counter | `provider`, `error_type` | Provider errors |
| `gateway_fallback_total` | counter | `from`, `to` | Fallback events |

### Deduplication Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gateway_dedup_total` | counter | `status` | Dedup operations (coalesced/unique) |
| `gateway_dedup_pending` | gauge | | Currently pending dedup requests |
| `gateway_dedup_wait_seconds` | histogram | | Wait time for coalesced requests |

### Reliability Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gateway_circuit_breaker_state` | gauge | `provider` | Circuit breaker state (0=closed, 1=open, 2=half-open) |
| `gateway_retry_total` | counter | `provider`, `attempt` | Retry attempts |
| `gateway_timeout_total` | counter | `provider` | Request timeouts |

## Distributed Tracing

The gateway supports OpenTelemetry for distributed tracing.

### Trace Structure

```
gateway.request
├── gateway.normalize          # Request normalization
├── gateway.cache.lookup       # Cache lookup
├── gateway.dedup.check        # Deduplication check
├── gateway.policy.evaluate    # Policy engine evaluation
├── gateway.route              # Provider routing
│   ├── provider.openai.send   # Provider request
│   └── provider.gemini.send   # Fallback provider (if needed)
└── gateway.cache.store        # Cache population (on miss)
```

### Span Attributes

| Attribute | Description |
|-----------|-------------|
| `gateway.request_id` | Request identifier |
| `gateway.model` | Resolved model name |
| `gateway.provider` | Selected provider |
| `gateway.cache_status` | Cache hit/miss/bypass |
| `gateway.tokens.prompt` | Input token count |
| `gateway.tokens.completion` | Output token count |
| `gateway.optimization.cache_hit` | Whether cache was hit |
| `gateway.optimization.dedup` | Whether request was deduplicated |

### Configuration

```bash
OTEL_EXPORTER_ENDPOINT=http://jaeger:4318
OTEL_SERVICE_NAME=llm-optimization-gateway
OTEL_SAMPLE_RATE=0.1  # Sample 10% of traces
```

## Prometheus Scrape Configuration

```yaml
scrape_configs:
  - job_name: 'llm-gateway'
    scrape_interval: 15s
    static_configs:
      - targets: ['gateway:9090']
```

## Grafana Dashboard

Key panels for an LLM Gateway dashboard:

1. **Request Rate** — `rate(gateway_requests_total[5m])`
2. **Cache Hit Rate** — `gateway_cache_hit_rate`
3. **P99 Latency** — `histogram_quantile(0.99, gateway_request_duration_seconds_bucket)`
4. **Provider Error Rate** — `rate(gateway_provider_errors_total[5m])`
5. **Token Savings** — `gateway_tokens_saved_total`
6. **Circuit Breaker State** — `gateway_circuit_breaker_state`
7. **Dedup Rate** — `rate(gateway_dedup_total{status="coalesced"}[5m])`

## Alerting Rules

```yaml
groups:
  - name: llm-gateway
    rules:
      - alert: HighErrorRate
        expr: rate(gateway_provider_errors_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High provider error rate"

      - alert: CacheDown
        expr: gateway_cache_operations_total{operation="get",status="error"} > 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Cache backend errors detected"

      - alert: AllProvidersDown
        expr: sum(gateway_circuit_breaker_state == 1) == count(gateway_circuit_breaker_state)
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "All provider circuit breakers are open"
```
