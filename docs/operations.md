# Operations Guide

This guide covers running the LLM Optimization Gateway in production.

## Deployment

### Docker (Recommended)

Use the production Docker image with the hardened Dockerfile:

```bash
docker build -f Dockerfile.prod -t llm-optimization-gateway:latest .
```

Run with Docker Compose:

```bash
docker compose -f docker-compose.prod.yml up -d
```

### Binary

Build and run the Go binary directly:

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o gateway ./cmd/gateway/
./gateway
```

## Production Configuration

### Resource Requirements

| Component | CPU | Memory | Storage |
|-----------|-----|--------|---------|
| Gateway | 0.5–2 cores | 256MB–1GB | None (stateless) |
| Redis | 0.5–1 core | 512MB–4GB | 1–10GB |

### Scaling

The gateway is stateless and horizontally scalable. Run multiple instances behind a load balancer:

```yaml
services:
  gateway:
    image: llm-optimization-gateway:latest
    deploy:
      replicas: 3
      resources:
        limits:
          cpus: "1.0"
          memory: 512M
```

### Load Balancer Configuration

- Use health check endpoint: `GET /health`
- Enable sticky sessions if using in-memory dedup (not needed with Redis)
- Set connection timeouts to match gateway timeouts

## Health Checks

### Endpoint

```bash
curl http://localhost:8080/health
```

### Docker Health Check

The production Docker image includes a built-in health check script:

```dockerfile
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
  CMD ["/healthcheck.sh"]
```

### Kubernetes Probes

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 30

readinessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
```

## Graceful Shutdown

The gateway handles `SIGTERM` and `SIGINT` signals for graceful shutdown:

1. Stop accepting new connections
2. Drain in-flight requests (up to `SHUTDOWN_TIMEOUT`)
3. Close cache connections
4. Exit with code 0

Set `SHUTDOWN_TIMEOUT` to match your load balancer's drain timeout.

## Redis Operations

### Connection

```bash
REDIS_URL=redis://redis-host:6379
REDIS_PASSWORD=your-password
REDIS_DB=0
```

### High Availability

For production, use Redis Sentinel or Redis Cluster:

```bash
REDIS_URL=redis-sentinel://sentinel1:26379,sentinel2:26379/mymaster
```

### Cache Maintenance

Monitor cache size and eviction:

```bash
redis-cli INFO memory
redis-cli INFO keyspace
```

Flush the gateway cache:

```bash
redis-cli KEYS "llm-gw:*" | xargs redis-cli DEL
```

## Security

### Non-root Container

The production Docker image runs as a non-root user (UID 10001):

```dockerfile
USER appuser:appuser
```

### Read-only Filesystem

The container filesystem is read-only. Only `/tmp` is writable:

```yaml
read_only: true
tmpfs:
  - /tmp:size=64M
```

### Network Security

- Restrict gateway access to your internal network
- Use TLS termination at the load balancer
- Rotate provider API keys regularly
- Do not expose the metrics port publicly

### Security Headers

The gateway sets the following headers:
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`

## Backup and Recovery

The gateway is stateless. Only Redis data needs backup:

```bash
# Create Redis backup
redis-cli BGSAVE

# Restore from backup
cp dump.rdb /var/lib/redis/dump.rdb
redis-cli DEBUG RELOAD
```

Cache loss is non-critical — it causes temporary increased latency as the cache warms up.

## Upgrades

### Rolling Update

```bash
# Build new image
docker build -f Dockerfile.prod -t llm-optimization-gateway:v2.0.0 .

# Rolling update
docker compose -f docker-compose.prod.yml up -d --no-deps gateway
```

### Zero-downtime

1. Deploy new version alongside old version
2. Shift traffic gradually using load balancer
3. Monitor error rates and latency
4. Remove old version once stable

## Monitoring Checklist

- [ ] Health check endpoint responding
- [ ] Cache hit rate within expected range (>20%)
- [ ] Provider error rate below threshold (<5%)
- [ ] No circuit breakers in open state
- [ ] Request latency P99 within SLA
- [ ] Redis connection pool utilization below 80%
- [ ] Memory usage within limits
- [ ] Disk space (for Redis persistence)
