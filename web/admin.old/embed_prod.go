//go:build embedui

package adminui

import "embed"

//go:embed dist
var Files embed.FS
