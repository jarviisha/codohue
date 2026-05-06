//go:build !embedui

// Package adminui exposes the compiled React SPA as an embedded filesystem.
//
// The default build leaves Files empty so that compiling the admin binary
// does not require a populated dist/ directory (useful for CI and local
// dev where the SPA is served by Vite). Production images build with
// `-tags=embedui` to embed the real bundle — see embed_prod.go.
package adminui

import "embed"

// Files is the embedded filesystem containing the compiled React SPA.
// Empty in default builds; populated when compiled with `-tags=embedui`.
var Files embed.FS
