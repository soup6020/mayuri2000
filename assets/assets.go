// Package assets embeds the static images and fonts used by image commands.
package assets

import "embed"

//go:embed fonts petpet templates vw
var FS embed.FS
