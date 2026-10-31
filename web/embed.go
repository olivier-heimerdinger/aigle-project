package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distEmbed embed.FS

// DistFS returns the embedded frontend build directory ("dist").
func DistFS() fs.FS {
	sub, err := fs.Sub(distEmbed, "dist")
	if err != nil {
		return nil
	}
	return sub
}

