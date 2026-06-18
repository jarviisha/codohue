// Package sse provides a minimal HTTP Server-Sent Events writer used by the
// admin plane to stream real-time updates to the web UI. The writer sets the
// canonical SSE headers (including `X-Accel-Buffering: no` so Nginx bypasses
// its default response buffering), flushes after every event, and reports
// client disconnect via the request context.
//
// Test helpers live in subpackage [ssetest] so production handlers do not pull
// in the testing dependency graph.
package sse
