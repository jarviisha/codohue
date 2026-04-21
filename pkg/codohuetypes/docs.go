// Package codohuetypes defines the HTTP and Redis Streams wire types shared
// between the Codohue server and external clients (e.g., the Go SDK).
//
// Types in this package are intentionally dependency-free (stdlib only) so
// that SDK consumers do not transitively pull server infrastructure
// dependencies like pgx, Qdrant, or go-redis.
package codohuetypes
