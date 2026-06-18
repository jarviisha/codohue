// Package eventbus provides an in-process publish/subscribe bus used by the
// admin plane to fan out real-time events (batch run progress, catalog state
// changes, health transitions) to SSE handlers. Single-replica only; a
// multi-replica deployment must swap the implementation for Redis pub/sub
// without changing the API surface.
package eventbus
