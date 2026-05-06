// Package adminui exposes the compiled React SPA as an embedded filesystem.
package adminui

import "embed"

// Files is the embedded filesystem containing the compiled React SPA.
//
//go:embed dist
var Files embed.FS
