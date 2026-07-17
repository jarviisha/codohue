#!/usr/bin/env bash
# Deploys the Codohue stack with docker compose.
#
# Expects to live one level below the directory holding docker-compose.prod.yml
# and the production .env file (CODOHUE_DATABASE_URL, CODOHUE_ADMIN_API_KEY).
# The caller must already be logged in to ghcr.io.
#
# Usage: IMAGE_TAG=v1.2.3 ./deploy/deploy.sh   (IMAGE_TAG defaults to latest)
set -euo pipefail

cd "$(dirname "$0")/.."

export IMAGE_TAG="${IMAGE_TAG:-latest}"
COMPOSE=(docker compose -f docker-compose.prod.yml)

echo "Deploying image tag: ${IMAGE_TAG}"
"${COMPOSE[@]}" pull

# depends_on conditions make `up -d` block until migrate completes and
# healthchecks pass; a crash-looping service would hang it forever
# (restart: on-failure retries indefinitely). Bound the wait and dump
# diagnostics instead of dying silently to the ssh-action timeout.
if ! timeout 600 "${COMPOSE[@]}" up -d --remove-orphans; then
  echo "ERROR: compose up did not converge within 10 minutes. Status:" >&2
  "${COMPOSE[@]}" ps -a >&2
  echo "--- migrate logs ---" >&2
  "${COMPOSE[@]}" logs --tail=30 migrate >&2
  echo "--- api logs ---" >&2
  "${COMPOSE[@]}" logs --tail=30 api >&2
  exit 1
fi

echo "Waiting for API to become healthy..."
for _ in $(seq 1 30); do
  api_container="$("${COMPOSE[@]}" ps -q api)"
  status="$(docker inspect -f '{{.State.Health.Status}}' "${api_container}" 2>/dev/null || echo starting)"
  if [ "${status}" = "healthy" ]; then
    echo "API is healthy. Deploy complete."
    docker image prune -f >/dev/null
    exit 0
  fi
  sleep 2
done

echo "ERROR: API did not become healthy in time. Recent logs:" >&2
"${COMPOSE[@]}" logs --tail=50 api >&2
exit 1
