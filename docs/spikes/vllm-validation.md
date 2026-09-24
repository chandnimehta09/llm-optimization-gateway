# Spike: vLLM Prefix Caching Validation

## Summary

This document validates vLLM's prefix caching capabilities and defines how the
LLM optimization gateway integrates with vLLM to exploit prefix cache hits for
reduced latency and cost.

## vLLM Prefix Caching Overview

vLLM implements automatic prefix caching (APC) that stores the KV cache of
existing queries so that a new query sharing the same prefix with an existing
one can reuse the cached KV data directly. This avoids redundant computation of
shared prefix tokens.

### Enabling Prefix Caching

Prefix caching is enabled via the `--enable-prefix-caching` flag when starting
the vLLM server:

```bash
vllm serve <model> --enable-prefix-caching
```

No application-level API changes are required — the server transparently reuses
cached KV blocks when the prompt shares a prefix with a previously-seen prompt.

### API Compatibility

vLLM exposes an **OpenAI-compatible** API at `/v1/chat/completions` and
`/v1/completions`. Requests follow the standard OpenAI format:

```json
{
  "model": "meta-llama/Llama-3-8b-instruct",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Explain prefix caching."}
  ],
  "temperature": 0.7,
  "max_tokens": 256
}
```

The gateway already normalizes requests into this format via the
`request.NormalizedRequest` type, so no additional transformation is needed.

### Cache Hit Telemetry

vLLM reports prefix cache metrics through several channels:

| Metric                                  | Source             | Description                         |
|-----------------------------------------|--------------------|-------------------------------------|
| `vllm:num_prefix_cache_hit_blocks`      | Prometheus         | Total KV blocks served from cache   |
| `vllm:num_prefix_cache_miss_blocks`     | Prometheus         | Total KV blocks computed fresh      |
| `vllm:gpu_prefix_cache_hit_rate`        | Prometheus         | Ratio of cached to total blocks     |
| `usage.prompt_tokens_details.cached_tokens` | Response JSON  | Per-request cached token count      |
| `usage.prompt_tokens`                   | Response JSON      | Total prompt tokens processed       |

The per-request `cached_tokens` field in the response `usage` object is the
primary signal for the gateway to track cache effectiveness per request.

### Prefix Token Metrics

From the response `usage` object:

```json
{
  "usage": {
    "prompt_tokens": 150,
    "completion_tokens": 42,
    "total_tokens": 192,
    "prompt_tokens_details": {
      "cached_tokens": 128
    }
  }
}
```

- **prompt_tokens**: total tokens in the prompt (including cached).
- **cached_tokens**: tokens served from the prefix cache (avoids recomputation).
- **cache_hit_rate**: `cached_tokens / prompt_tokens` — computed by the gateway.

### What Makes a Good Prefix

Prefix caching operates on contiguous token blocks. The cache matches from the
start of the token sequence. To maximize hits:

1. **System prompts** — identical across requests; always the first tokens.
2. **Tool definitions** — usually identical for a session or application.
3. **Common context** — shared few-shot examples or instructions.
4. **Conversation history** — earlier turns are a prefix of later turns.

The gateway constructs a *stable prefix* by concatenating these components in a
deterministic order and keeping them at the front of the message list.

### Eligibility Rules

A request is eligible for prefix caching when:

- The vLLM backend has `--enable-prefix-caching` enabled.
- The request contains a non-empty system prompt or tool definitions.
- The gateway configuration allows prefix caching for the model.

Requests that lack a stable prefix (e.g. a single user message with no system
prompt) are still sent normally — they simply will not benefit from caching.

## Integration Design

### Architecture

```
 ┌─────────────┐     ┌────────────────┐     ┌──────────────┐
 │ Normalized   │────▶│ PrefixBackend  │────▶│ vLLM Server  │
 │ Request      │     │ (this package) │     │ (prefix APC) │
 └─────────────┘     └────────────────┘     └──────────────┘
                            │
                     ┌──────┴──────┐
                     │ Telemetry   │
                     │ - prefix_tokens
                     │ - cached_tokens
                     │ - hit_rate
                     │ - latency_savings
                     └─────────────┘
```

### Components

| Component       | File                  | Responsibility                       |
|-----------------|----------------------|--------------------------------------|
| Config          | `config.go`          | vLLM connection and feature flags    |
| Client          | `client.go`          | HTTP transport to vLLM               |
| Backend         | `backend.go`         | Prefix construction + cache logic    |
| Telemetry       | `telemetry.go`       | Metrics and tracing for prefix cache |

### Policy Integration

The policy engine already has a `UsePrefixCache` field on `PolicyDecision` and
a `RuleTargetPrefixCache` rule target. The prefix backend checks
`decision.UsePrefixCache` before enabling prefix-aware request construction.

## Validation Results

| Capability               | Status    | Notes                                  |
|--------------------------|-----------|----------------------------------------|
| OpenAI-compatible API    | ✅ Verified | Standard `/v1/chat/completions`       |
| Prefix caching toggle    | ✅ Verified | `--enable-prefix-caching` flag        |
| Per-request cached_tokens| ✅ Verified | In `usage.prompt_tokens_details`      |
| Prometheus metrics       | ✅ Verified | `vllm:gpu_prefix_cache_hit_rate`      |
| Stable prefix matching   | ✅ Verified | System prompt + tools always cached   |
| No API changes needed    | ✅ Verified | Cache is server-side, transparent     |

## Risks and Mitigations

| Risk                                    | Mitigation                                       |
|-----------------------------------------|--------------------------------------------------|
| vLLM version without APC               | Feature-detect via health check; degrade to passthrough |
| Block size mismatch loses cache benefit | Document recommended block sizes in config       |
| Network overhead exceeds savings        | Track latency_savings; disable if negative       |
| Response format changes                 | Parse usage fields defensively                   |

## Conclusion

vLLM's prefix caching is transparent and requires no changes to the request
format. The gateway adds value by:

1. Constructing stable prefixes (system prompt + tools + common context first).
2. Tracking per-request cache hit metrics from the `usage` response.
3. Exposing aggregate hit-rate and latency-savings telemetry.
4. Integrating with the policy engine to control when prefix caching is used.
