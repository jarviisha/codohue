// Package adminui exposes the compiled React SPA as an embedded filesystem.
package adminui

import "embed"

//go:embed dist
var Files embed.FS
