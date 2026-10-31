// Package web embarque le build de l'interface Svelte 5 SaaS (y compris la Console d'Administration)
// directement dans le binaire Go statique d'Aigle Server grâce à go:embed.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distEmbed embed.FS

// GetDistFS retourne le sous-système de fichiers pointant vers le dossier dist/.
func GetDistFS() fs.FS {
	sub, err := fs.Sub(distEmbed, "dist")
	if err != nil {
		return nil
	}
	return sub
}

