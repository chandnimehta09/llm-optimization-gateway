# Optimizations Guide

The LLM Optimization Gateway reduces cost and latency through several optimization strategies that work together transparently.

## Optimization Pipeline

```
Request → Normalization → Exact Cache → Deduplication → Prefix Cache → Provider
                              ↓                                           ↓
                         Cache HIT                                   Cache STORE
                        (fast path)                                (populate cache)
```

## 1. Request Normalization

All incoming requests are normalized to a canonical internal format before any optimization is applied. This ensures consistent cache keys and deduplication regardless of the input format.

**What gets normalized:**
- Message format (OpenAI ↔ Gemini conversion)
- Model name aliases (e.g., `gpt4` → `gpt-4`)
- Default parameter values (temperature, top_p)
- Whitespace and content trimming
- Role validation and normalization

**Configuration:**

Normalization is always active and cannot be disabled. It has no measurable performance impact.

## 2. Exact Cache

The exact cache stores responses keyed by a cryptographic hash of the normalized request. Identical requests return cached responses without calling the provider.

**How it works:**
1. Request is normalized and a context identity hash is computed
2. The hash incorporates: model, messages, temperature, max_tokens, tools
3. Cache lookup: if HIT, return cached response immediately
4. On MISS, request goes to provider; response is cached for future requests

**Cache key components:**
- Model name
- Message content and roles
- Sampling parameters (temperature, top_p)
- Tool definitions
- System prompt

**Effectiveness:**
- Best for: repeated queries, static system prompts, FAQ-style workloads
- Typical hit rate: 15-40% for interactive workloads, 60-90% for batch workloads
- Latency reduction: ~95% on cache hits (sub-millisecond vs. seconds)

**Configuration:**

| Variable | Default | Description |
|----------|---------|-------------|
| `CACHE_ENABLED` | `true` | Enable/disable exact cache |
| `CACHE_TTL` | `10m` | Time-to-live for entries |
| `CACHE_KEY_PREFIX` | `llm-gw` | Key prefix for namespacing |

## 3. Request Deduplication

Deduplication coalesces concurrent identical requests so that only one request is sent to the provider. All waiting callers receive the same response.

**How it works:**
1. When a request arrives, its dedup key is computed from the normalized request
2. If an identical request is already in-flight, the new request waits for it
3. When the first request completes, all waiting requests receive the response
4. If the first request fails, waiting requests may retry independently

**Effectiveness:**
- Best for: burst traffic, auto-complete, retry storms
- Typical dedup rate: 5-20% of concurrent requests
- Cost reduction: proportional to dedup rate

**Configuration:**

| Variable | Default | Description |
|----------|---------|-------------|
| `DEDUP_ENABLED` | `true` | Enable/disable deduplication |
| `DEDUP_WINDOW` | `5s` | Time window for coalescing |
| `DEDUP_MAX_PENDING` | `1000` | Max pending deduplicated requests |

## 4. Prefix Caching (vLLM)

Prefix caching reuses KV-cache entries from the inference engine (vLLM) for shared prompt prefixes. This reduces first-token latency for requests with common system prompts.

**How it works:**
1. System prompts and common conversation prefixes are identified
2. The prefix is sent to vLLM with a cache hint
3. vLLM reuses the KV-cache for the prefix, only computing new tokens
4. Subsequent requests with the same prefix skip recomputation

**Effectiveness:**
- Best for: long system prompts, multi-turn conversations, RAG contexts
- Typical latency reduction: 30-60% on first token
- Requires vLLM backend with prefix caching enabled

**Configuration:**

| Variable | Default | Description |
|----------|---------|-------------|
| `PREFIX_CACHE_ENABLED` | `false` | Enable prefix caching |
| `PREFIX_CACHE_MIN_LENGTH` | `100` | Min prefix length in tokens |
| `PREFIX_CACHE_VLLM_URL` | `""` | vLLM server URL |

## 5. Content Deduplication

Content deduplication removes duplicate text segments within a single request, reducing token count and provider costs.

**How it works:**
1. Message content is analyzed for repeated segments
2. Duplicate segments are removed, preserving first occurrence
3. A dedup map is maintained to track removals
4. Token count is reduced proportionally

**Effectiveness:**
- Best for: copy-pasted content, repeated context, log analysis
- Typical reduction: 5-30% of tokens for repetitive content

## Optimization Metrics

The gateway exposes metrics for each optimization layer:

| Metric | Description |
|--------|-------------|
| `gateway_cache_hits_total` | Total cache hits |
| `gateway_cache_misses_total` | Total cache misses |
| `gateway_cache_hit_rate` | Cache hit ratio |
| `gateway_dedup_coalesced_total` | Total deduplicated requests |
| `gateway_dedup_rate` | Deduplication ratio |
| `gateway_prefix_cache_hits_total` | Prefix cache reuse count |
| `gateway_tokens_saved_total` | Total tokens saved by optimizations |
| `gateway_cost_savings_usd` | Estimated cost savings in USD |

## Tuning Recommendations

### High-throughput batch workloads
```bash
CACHE_TTL=30m           # Longer TTL for stable workloads
DEDUP_WINDOW=10s        # Wider dedup window for burst absorption
DEDUP_MAX_PENDING=5000  # Higher pending limit
```

### Interactive/chat workloads
```bash
CACHE_TTL=5m            # Shorter TTL for fresher responses
DEDUP_WINDOW=2s         # Narrow dedup window
PREFIX_CACHE_ENABLED=true  # Enable prefix caching for system prompts
```

### Cost-optimized
```bash
CACHE_ENABLED=true
CACHE_TTL=1h            # Aggressive caching
DEDUP_ENABLED=true
PREFIX_CACHE_ENABLED=true
```
