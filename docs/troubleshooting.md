# Troubleshooting Guide

Common issues and their solutions when operating the LLM Optimization Gateway.

## Startup Issues

### Gateway fails to start

**Symptom:** Container exits immediately or logs show initialization errors.

**Check:**
```bash
docker logs <container-id>
```

**Common causes:**

1. **Port already in use**
   ```
   Error: listen tcp :8080: bind: address already in use
   ```
   Solution: Change `GATEWAY_PORT` or stop the conflicting process.

2. **Redis connection refused**
   ```
   Error: dial tcp redis:6379: connect: connection refused
   ```
   Solution: Verify Redis is running and `REDIS_URL` is correct. The gateway will start without Redis but with caching degraded.

3. **Permission denied**
   ```
   Error: permission denied
   ```
   Solution: Check file permissions. The production container runs as non-root (UID 10001).

### Health check failing

**Symptom:** Docker reports container as unhealthy.

**Check:**
```bash
docker inspect --format='{{json .State.Health}}' <container-id>
curl http://localhost:8080/health
```

**Solution:** Increase `start_period` in the health check configuration to give the gateway time to initialize.

## Request Issues

### 400 Bad Request

**Symptom:** Requests return 400 with an error message.

**Common causes:**

1. **Missing model field**
   ```json
   {"error": {"message": "model is required", "type": "invalid_request_error"}}
   ```
   Solution: Include the `model` field in your request.

2. **Empty messages array**
   ```json
   {"error": {"message": "messages must not be empty", "type": "invalid_request_error"}}
   ```
   Solution: Include at least one message in the request.

3. **Invalid JSON**
   ```json
   {"error": {"message": "invalid JSON in request body", "type": "invalid_request_error"}}
   ```
   Solution: Validate your JSON payload. Check for trailing commas or missing quotes.

### 429 Rate Limited

**Symptom:** Requests return 429 with a `Retry-After` header.

**Solution:**
- Implement exponential backoff in your client
- Check provider rate limits and adjust request rate
- Enable caching to reduce provider calls
- Enable deduplication to coalesce identical requests

### 502 Bad Gateway

**Symptom:** Provider returned an error that the gateway couldn't handle.

**Check:**
```bash
# Check provider health
curl http://localhost:9090/metrics | grep circuit_breaker
```

**Solution:**
- Verify provider API keys are valid
- Check provider status pages for outages
- Review circuit breaker state — it may need time to recover

### 503 Service Unavailable

**Symptom:** All providers are unavailable.

**Check:**
```bash
# Check all circuit breakers
curl http://localhost:9090/metrics | grep "circuit_breaker_state"
```

**Solution:**
- Wait for circuit breakers to transition to half-open
- Manually verify at least one provider is responding
- Check network connectivity to provider endpoints

## Cache Issues

### Low cache hit rate

**Symptom:** `gateway_cache_hit_rate` is below expected levels (<10%).

**Possible causes:**

1. **High request diversity** — Each request is unique, so caching provides limited benefit.
   - Solution: This is normal for diverse workloads.

2. **TTL too short** — Entries expire before being reused.
   - Solution: Increase `CACHE_TTL`.

3. **Temperature variation** — Different temperatures produce different cache keys.
   - Solution: Standardize temperature values where possible.

4. **Redis down** — Cache operations are failing silently.
   - Check: `curl http://localhost:9090/metrics | grep cache_operations`

### Cache causing stale responses

**Symptom:** Users receive outdated information.

**Solution:**
- Reduce `CACHE_TTL` for time-sensitive queries
- Use `X-Cache-Control: no-cache` header for specific requests
- Flush the cache: `redis-cli KEYS "llm-gw:*" | xargs redis-cli DEL`

### Redis connection errors

**Symptom:** Logs show Redis connection errors but the gateway is still responding.

The gateway gracefully degrades when Redis is unavailable — requests are served without caching.

**Check:**
```bash
redis-cli ping
redis-cli INFO clients
```

**Solutions:**
- Verify Redis is running and reachable
- Check Redis memory usage: `redis-cli INFO memory`
- Check connection pool: `REDIS_POOL_SIZE` may need increasing
- Verify network/firewall rules between gateway and Redis

## Routing Issues

### Requests going to wrong provider

**Symptom:** Requests intended for one provider are routed to another.

**Check logs:**
```bash
docker logs <container-id> | grep "routing"
```

**Solution:**
- Use `X-Provider-Hint` header to force routing
- Check policy configuration for model-to-provider mappings
- Verify provider health — unhealthy providers are skipped

### Fallback not working

**Symptom:** Requests fail even though fallback providers are available.

**Check:**
- `ROUTING_FALLBACK_ENABLED` must be `true`
- Fallback providers must be registered and healthy
- `ROUTING_MAX_ATTEMPTS` must be > 1
- The error type must be fallback-eligible (4xx client errors typically do not trigger fallback)

## Performance Issues

### High latency

**Symptom:** P99 latency is higher than expected.

**Diagnose:**
```bash
# Check provider latency
curl http://localhost:9090/metrics | grep provider_latency

# Check cache latency
curl http://localhost:9090/metrics | grep cache_latency
```

**Solutions:**
1. Enable caching (if not enabled)
2. Enable prefix caching for long system prompts
3. Check Redis latency: `redis-cli --latency`
4. Verify network latency to providers
5. Increase `ROUTING_TIMEOUT` if providers are slow

### High memory usage

**Symptom:** Container approaching memory limits.

**Check:**
```bash
docker stats <container-id>
```

**Solutions:**
- Reduce `DEDUP_MAX_PENDING` to limit in-flight request buffering
- Reduce `REDIS_POOL_SIZE`
- Increase container memory limits
- Check for goroutine leaks: enable pprof in debug mode

### Connection pool exhaustion

**Symptom:** Requests timing out with connection errors.

**Check:**
```bash
# Redis connections
redis-cli INFO clients
```

**Solutions:**
- Increase `REDIS_POOL_SIZE`
- Reduce `REDIS_READ_TIMEOUT` to fail fast
- Add connection retry with backoff

## Docker Issues

### Container runs out of disk space

**Symptom:** Write errors in logs.

The production container uses a read-only filesystem with only `/tmp` writable.

**Solution:**
- Increase tmpfs size: `tmpfs: /tmp:size=128M`
- Check if logs are being written to disk (they should go to stdout)

### Container restarts repeatedly

**Symptom:** Container restart count increasing.

**Check:**
```bash
docker inspect --format='{{json .RestartCount}}' <container-id>
docker logs --tail 50 <container-id>
```

**Solution:** Check the logs for the root cause. Common issues are listed above.

## Getting Help

If you encounter an issue not covered here:

1. Check the logs with `LOG_LEVEL=debug`
2. Check metrics on the metrics port
3. Review the [Architecture](architecture.md) doc to understand the request flow
4. Open an issue with logs, metrics, and request/response examples
