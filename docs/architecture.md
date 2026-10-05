# Architecture Overview

The LLM Optimization Gateway is a transparent proxy that sits between clients and LLM providers, optimizing requests for cost and latency.

## System Architecture

```
                    ┌─────────────────────────────────────────────────┐
                    │              LLM Optimization Gateway            │
                    │                                                  │
  ┌──────────┐     │  ┌──────────┐  ┌─────────┐  ┌───────────────┐  │  ┌──────────┐
  │  Client   │────►│  │  API     │─►│ Request │─►│ Cache         │  │  │  OpenAI  │
  │  (HTTP)   │◄────│  │  Layer   │  │ Normal. │  │ Middleware    │  │  │  API     │
  │           │     │  └──────────┘  └─────────┘  └───────┬───────┘  │  └──────────┘
  └──────────┘     │                                      │          │
                    │                              ┌──────▼───────┐  │  ┌──────────┐
                    │                              │ Dedup        │  │  │  Gemini  │
                    │                              │ Middleware   │  │  │  API     │
                    │                              └──────┬───────┘  │  └──────────┘
                    │                                      │          │
                    │  ┌──────────┐  ┌─────────┐  ┌──────▼───────┐  │  ┌──────────┐
                    │  │ Redis    │  │ Policy  │◄─│ Routing      │──│─►│  vLLM    │
                    │  │ Cache    │  │ Engine  │  │ Engine       │  │  │  Server  │
                    │  └──────────┘  └─────────┘  └──────────────┘  │  └──────────┘
                    │                                                  │
                    └─────────────────────────────────────────────────┘
```

## Component Overview

### API Layer (`internal/api/`)

The HTTP interface that accepts incoming requests. It provides:
- OpenAI-compatible endpoints (`/v1/chat/completions`)
- Request validation and error formatting
- Health check and metrics endpoints

The API layer uses standard `net/http` handlers with no external framework dependencies.

### Request Normalization (`internal/request/`)

Transforms incoming requests from various formats (OpenAI, Gemini) into a canonical internal representation (`NormalizedRequest`). This ensures consistent processing regardless of input format.

**Key design decisions:**
- Provider-agnostic internal format
- Lossless conversion — original request preserved for debugging
- Change tracking — all normalization transformations are recorded

### Context Identity (`internal/context/`)

Computes a deterministic identity hash for request content. This identity is the foundation for caching and deduplication.

**Components:**
- **Canonicalization** — deterministic serialization of request content
- **Hashing** — SHA-256 based identity computation
- **Cache Key Builder** — constructs namespaced cache keys
- **Change Detection** — detects meaningful changes between requests
- **Invalidation** — rules for when cached entries should be invalidated

### Cache Layer (`internal/cache/`)

A multi-backend cache abstraction with failure isolation.

**Architecture:**
```
Cache Interface
    ├── Redis Cache (primary)
    ├── In-Memory Cache (fallback)
    └── Circuit Breaker (fault isolation)
```

**Features:**
- Backend-agnostic interface
- Circuit breaker for Redis failure isolation
- Health monitoring with automatic recovery
- Configurable TTL with per-entry overrides
- Serialization with compression

### Deduplication (`internal/dedup/`)

Coalesces concurrent identical requests. Only one request reaches the provider; all others wait and receive the same response.

**Algorithm:**
1. Compute dedup key from normalized request
2. Check if an identical request is in-flight
3. If yes, register as waiter and block
4. If no, register as leader and proceed
5. On completion, broadcast result to all waiters

### Policy Engine (`internal/policy/`)

Makes deterministic optimization decisions based on request context, cache state, provider health, and configuration rules.

**Decision outputs:**
- Whether to use exact cache
- Whether to use prefix cache
- Whether to apply content deduplication
- Which model and provider to select
- Confidence score and reasoning

**Rule categories:**
- Cache rules (TTL, bypass conditions)
- Routing rules (model-to-provider mapping, cost optimization)
- Model selection rules (capability matching, load balancing)

### Routing Engine (`internal/routing/`)

Routes requests to providers with fallback support.

**Components:**
- **Registry** — stores providers with capabilities and health status
- **Engine** — orchestrates provider selection and execution
- **Fallback Chain** — ordered list of providers to try on failure
- **Health Checker** — monitors provider availability
- **Executor** — handles individual provider requests

**Fallback flow:**
```
Primary Provider → [FAIL] → Fallback Provider 1 → [FAIL] → Fallback Provider 2 → [FAIL] → Error
```

### Reliability (`internal/reliability/`)

Cross-cutting reliability patterns:

- **Circuit Breaker** — prevents cascading failures by short-circuiting calls to unhealthy providers
- **Retry** — configurable retry with exponential backoff and jitter
- **Timeout** — per-request and per-provider timeout enforcement
- **Error Classification** — categorizes errors as retryable, non-retryable, or fatal

### Prefix Caching (`internal/prefix/`)

Integrates with vLLM's prefix caching to reuse KV-cache entries for shared prompt prefixes.

**How it works:**
1. Identify cacheable prefix in the conversation
2. Send prefix to vLLM with cache hint
3. vLLM reuses KV-cache, reducing computation

### Benchmark (`internal/benchmark/`)

Performance benchmarking framework for measuring optimization effectiveness.

**Components:**
- Workload generators (static prefix, repository context, random)
- Benchmark harness with statistical analysis
- Comparison engine for A/B testing configurations
- Report generation with charts and exports

## Request Lifecycle

```
1. HTTP Request arrives at API Layer
   │
2. ├── Validate request format and required fields
   │
3. ├── Normalize to canonical NormalizedRequest
   │   ├── Detect input format (OpenAI/Gemini)
   │   ├── Convert to internal message format
   │   ├── Resolve model aliases
   │   └── Apply defaults
   │
4. ├── Compute context identity hash
   │
5. ├── Cache Middleware: lookup by identity hash
   │   ├── HIT → Return cached response (fast path)
   │   └── MISS → Continue to provider
   │
6. ├── Dedup Middleware: check for in-flight duplicate
   │   ├── DUPLICATE → Wait for leader's result
   │   └── UNIQUE → Continue as leader
   │
7. ├── Policy Engine: evaluate optimization rules
   │   ├── Select model
   │   ├── Select provider
   │   └── Determine optimization flags
   │
8. ├── Routing Engine: execute with fallback
   │   ├── Try primary provider
   │   ├── On failure: try fallback providers
   │   └── Record metrics for all attempts
   │
9. ├── Cache Middleware: store response on miss
   │
10.└── Return response to client
```

## Design Principles

1. **Transparency** — Clients interact with an OpenAI-compatible API. Optimizations are invisible.
2. **Graceful Degradation** — Every optimization layer can fail independently. Cache down? Skip it. Provider down? Fall back.
3. **Determinism** — Same inputs produce same routing decisions. Policy decisions include audit trails.
4. **Observability** — Every layer emits structured logs, metrics, and traces.
5. **Stateless Gateway** — All state lives in Redis. The gateway binary is horizontally scalable.
6. **Fail-safe Defaults** — If configuration is missing, the gateway defaults to safe values.

## Package Dependencies

```
api → request → context → cache → dedup → policy → routing → reliability
                                                        ↓
                                                     prefix
```

No circular dependencies. Each package depends only on packages to its left or below.

## Technology Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.26 |
| HTTP | stdlib `net/http` |
| Cache | Redis 7 via `go-redis` |
| Telemetry | OpenTelemetry SDK |
| Testing | stdlib `testing` + `miniredis` |
| Containers | Docker with distroless base |
