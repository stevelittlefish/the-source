// Package docs embeds the API contract so deployment cannot leave it behind.
package docs

import "embed"

//go:embed openapi.yaml API.md
var Files embed.FS
