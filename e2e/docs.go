// Package e2e contains end-to-end tests for the Codohue API.
// These tests start a real API binary against live infrastructure
// (postgres, redis, qdrant) and exercise each endpoint over HTTP.
//
// Prerequisites before running:
//
//	make up-infra   # start postgres, redis, qdrant
//	make migrate-up # apply all pending migrations
//	make build-api  # compile ./tmp/api
//
// Run with:
//
//	make test-e2e
package e2e
