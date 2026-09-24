#!/bin/sh
# Docker entrypoint script for LLM Optimization Gateway.
# Performs pre-flight checks and starts the gateway binary.

set -e

# Print startup banner
echo "========================================"
echo " LLM Optimization Gateway"
echo " Starting up..."
echo "========================================"

# Log configuration (non-sensitive values only)
echo "Configuration:"
echo "  GATEWAY_PORT=${GATEWAY_PORT:-8080}"
echo "  METRICS_PORT=${METRICS_PORT:-9090}"
echo "  CACHE_ENABLED=${CACHE_ENABLED:-true}"
echo "  DEDUP_ENABLED=${DEDUP_ENABLED:-true}"
echo "  LOG_LEVEL=${LOG_LEVEL:-info}"
echo "  REDIS_URL=${REDIS_URL:+[configured]}"

# Validate required configuration
if [ -z "${GATEWAY_PORT}" ]; then
  export GATEWAY_PORT=8080
fi

if [ -z "${METRICS_PORT}" ]; then
  export METRICS_PORT=9090
fi

# Wait for dependencies if configured
if [ -n "${REDIS_URL}" ]; then
  echo "Checking Redis connectivity..."
  # Extract host and port from Redis URL
  REDIS_HOST=$(echo "${REDIS_URL}" | sed -E 's|redis://([^:]+):([0-9]+).*|\1|')
  REDIS_PORT=$(echo "${REDIS_URL}" | sed -E 's|redis://([^:]+):([0-9]+).*|\2|')

  if [ -n "${REDIS_HOST}" ] && [ -n "${REDIS_PORT}" ]; then
    RETRIES=0
    MAX_RETRIES=30
    while [ ${RETRIES} -lt ${MAX_RETRIES} ]; do
      if nc -z "${REDIS_HOST}" "${REDIS_PORT}" 2>/dev/null; then
        echo "Redis is reachable at ${REDIS_HOST}:${REDIS_PORT}"
        break
      fi
      RETRIES=$((RETRIES + 1))
      echo "Waiting for Redis (attempt ${RETRIES}/${MAX_RETRIES})..."
      sleep 1
    done

    if [ ${RETRIES} -ge ${MAX_RETRIES} ]; then
      echo "WARNING: Redis not reachable after ${MAX_RETRIES} attempts. Starting without cache."
    fi
  fi
fi

echo "Starting gateway..."

# Execute the gateway binary
exec /gateway "$@"
