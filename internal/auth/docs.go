// Package auth provides HTTP middleware and helpers for validating API keys.
// It supports two tiers:
//   - Admin key: a single global key for administrative endpoints (e.g. namespace provisioning).
//   - Namespace key: a per-namespace key stored as a bcrypt hash in namespace_configs.
//
// If a namespace has no key configured, the admin key is accepted as a fallback
// to allow gradual migration of existing clients.
package auth
