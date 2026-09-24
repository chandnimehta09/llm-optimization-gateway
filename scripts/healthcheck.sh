#!/bin/sh
# Health check script for LLM Optimization Gateway.
# Used by Docker HEALTHCHECK to determine container health.

set -e

# Configuration
GATEWAY_PORT="${GATEWAY_PORT:-8080}"
HEALTH_URL="http://localhost:${GATEWAY_PORT}/health"
TIMEOUT=3

# Perform health check
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" --max-time ${TIMEOUT} "${HEALTH_URL}" 2>/dev/null || echo "000")

if [ "${HTTP_CODE}" = "200" ]; then
  exit 0
fi

# If /health is not available, try a simple TCP check on the gateway port
if nc -z localhost "${GATEWAY_PORT}" 2>/dev/null; then
  exit 0
fi

echo "Health check failed: HTTP ${HTTP_CODE}"
exit 1
