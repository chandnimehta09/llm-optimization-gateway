# API Documentation

The LLM Optimization Gateway exposes an OpenAI-compatible HTTP API for chat completions with transparent optimization.

## Base URL

```
http://localhost:8080
```

## Authentication

The gateway forwards authentication headers to upstream providers. Include your provider API key:

```
Authorization: Bearer <your-api-key>
```

## Endpoints

### POST /v1/chat/completions

Create a chat completion. This is the primary endpoint that supports both OpenAI and Gemini request formats.

**Request Headers:**

| Header | Required | Description |
|--------|----------|-------------|
| `Content-Type` | Yes | Must be `application/json` |
| `Authorization` | Yes | Bearer token for the provider |
| `X-Provider-Hint` | No | Force routing to a specific provider |
| `X-Cache-Control` | No | Cache control: `no-cache`, `no-store` |

**OpenAI Format Request Body:**

```json
{
  "model": "gpt-4",
  "messages": [
    {
      "role": "system",
      "content": "You are a helpful assistant."
    },
    {
      "role": "user",
      "content": "Explain quantum computing."
    }
  ],
  "temperature": 0.7,
  "max_tokens": 500,
  "top_p": 1.0,
  "stream": false,
  "stop": ["\n\n"],
  "user": "user-123"
}
```

**Gemini Format Request Body:**

```json
{
  "contents": [
    {
      "role": "user",
      "parts": [
        {"text": "Explain quantum computing."}
      ]
    }
  ],
  "generation_config": {
    "temperature": 0.7,
    "max_output_tokens": 500
  },
  "model": "gemini-pro"
}
```

**Request Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `model` | string | Yes | Model identifier (e.g., `gpt-4`, `gemini-pro`) |
| `messages` | array | Yes | Conversation messages (OpenAI format) |
| `contents` | array | Yes | Conversation contents (Gemini format) |
| `temperature` | float | No | Sampling temperature (0.0–2.0), default: 1.0 |
| `max_tokens` | int | No | Maximum tokens to generate |
| `top_p` | float | No | Nucleus sampling (0.0–1.0), default: 1.0 |
| `stream` | bool | No | Enable streaming (not yet supported) |
| `stop` | array | No | Stop sequences |
| `tools` | array | No | Tool/function definitions |
| `tool_choice` | string | No | Tool selection mode |

**Response (200 OK):**

```json
{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1700000000,
  "model": "gpt-4",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Quantum computing uses quantum mechanical phenomena..."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 25,
    "completion_tokens": 150,
    "total_tokens": 175
  }
}
```

**Response Headers:**

| Header | Description |
|--------|-------------|
| `X-Request-ID` | Unique request identifier for tracing |
| `X-Cache-Status` | Cache status: `HIT`, `MISS`, or `BYPASS` |
| `X-Cache-Key` | Truncated cache key (when cached) |
| `X-Cache-Age` | Cache entry age in seconds (when HIT) |
| `Content-Type` | Always `application/json` |

### GET /health

Health check endpoint.

**Response (200 OK):**

```json
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime": "2h30m15s",
  "checks": {
    "redis": "ok",
    "providers": "ok"
  }
}
```

### GET /metrics

Prometheus-compatible metrics endpoint (on metrics port).

## Error Responses

All errors follow the OpenAI error format:

```json
{
  "error": {
    "message": "Description of what went wrong",
    "type": "invalid_request_error",
    "param": "model",
    "code": "model_not_found"
  }
}
```

**Error Types:**

| HTTP Status | Type | Description |
|------------|------|-------------|
| 400 | `invalid_request_error` | Malformed request or invalid parameters |
| 401 | `authentication_error` | Invalid or missing API key |
| 404 | `not_found` | Endpoint not found |
| 405 | `invalid_request_error` | Method not allowed |
| 429 | `rate_limit_error` | Rate limit exceeded |
| 500 | `server_error` | Internal gateway error |
| 502 | `provider_error` | Upstream provider error |
| 503 | `service_unavailable` | All providers unavailable |

## Rate Limits

Rate limits depend on the upstream provider's limits. The gateway adds the following headers when rate-limited:

| Header | Description |
|--------|-------------|
| `Retry-After` | Seconds to wait before retrying |
| `X-RateLimit-Remaining` | Remaining requests in window |

## Request Flow

```
Client → Gateway API → Normalization → Cache Check → Dedup Check →
  Policy Engine → Provider Routing → Fallback (if needed) → Response
```

1. **Normalization**: Incoming requests are normalized to a canonical format
2. **Cache Check**: Exact-match cache lookup (returns immediately on HIT)
3. **Dedup Check**: Identical in-flight requests are coalesced
4. **Policy Engine**: Evaluates routing, model selection, and optimization rules
5. **Provider Routing**: Sends request to selected provider with fallback
6. **Response**: Returns provider response, populating cache on success
