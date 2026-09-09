#!/usr/bin/env bash
# Run from any directory: IMAGE_TAG=v1.2.3 bash deploy/deploy.sh
# Compose reads the deployment directory's .env. Log in to GHCR beforehand.
set -euo pipefail
cd "$(dirname "$0")/.."

COMPOSE=(docker compose -f compose.prod.yaml)
"${COMPOSE[@]}" config --quiet
"${COMPOSE[@]}" pull

# The outer timeout also bounds migration/dependency startup. --wait checks
# every service with a healthcheck and requires cron to be running.
if ! timeout 600 "${COMPOSE[@]}" up --wait --wait-timeout 300 --remove-orphans; then
  echo "ERROR: deployment failed or timed out." >&2
  "${COMPOSE[@]}" ps -a >&2 || true
  "${COMPOSE[@]}" logs --tail=50 >&2 || true
  exit 1
fi

echo "Deployment complete."
