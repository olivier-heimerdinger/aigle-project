// Package api — Gestionnaire de la Zone Publique (Vitrine de Présentation & Centre de Téléchargement Multi-OS).
// Sécurité :
//   - Liste blanche stricte (Whitelist) des fichiers téléchargeables autorisés.
//   - Nettoyage par filepath.Base() et rejet de tout caractère '..', '/' ou '\' (Anti-Path-Traversal OWASP A01).
//   - Calcul et mise en cache de l'empreinte cryptographique SHA-256 de chaque binaire (Windows, macOS, Linux)
//     pour permettre aux utilisateurs de vérifier l'intégrité du fichier téléchargé.
package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ReleaseAsset décrit un fichier téléchargeable publiquement (binaire OS ou support de présentation).
type ReleaseAsset struct {
	ID          string `json:"id"`
	OS          string `json:"os"`           // "windows", "macos", "linux", "presentation"
	Arch        string `json:"arch"`         // "amd64", "arm64", "universal"
	Label       string `json:"label"`        // ex: "Windows 10 / 11 (64-bit)"
	Filename    string `json:"filename"`     // ex: "aigle-windows-amd64.exe"
	Description string `json:"description"`  // Description de l'exécutable
	CommandHint string `json:"command_hint"` // Commande de lancement rapide
	Available   bool   `json:"available"`    // Vrai si le binaire est présent dans DownloadsDir
	SizeBytes   int64  `json:"size_bytes"`
	SizeFormatted string `json:"size_formatted"`
	SHA256      string `json:"sha256"`
	DownloadURL string `json:"download_url"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

type cachedHash struct {
	modTime time.Time
	size    int64
	sha256  string
}

var (
	hashCacheMu sync.Mutex
	hashCache   = make(map[string]cachedHash)
)

// Catalogue officiel des fichiers autorisés au téléchargement public (Liste Blanche stricte).
var allowedPublicCatalog = []ReleaseAsset{
	{
		ID:          "win-amd64",
		OS:          "windows",
		Arch:        "amd64",
		Label:       "Windows 10 / 11 (x64)",
		Filename:    "aigle-windows-amd64.exe",
		Description: "Binaire autonome tout-en-un pour Windows (Serveur Go + SQLite WAL + Interface Web embarquée).",
		CommandHint: ".\\aigle-windows-amd64.exe -open",
	},
	{
		ID:          "mac-arm64",
		OS:          "macos",
		Arch:        "arm64",
		Label:       "macOS Apple Silicon (M1 / M2 / M3 / M4)",
		Filename:    "aigle-darwin-arm64",
		Description: "Binaire natif ARM64 pour Mac Apple Silicon (Zéro dépendance externe, SQLite embarqué).",
		CommandHint: "chmod +x aigle-darwin-arm64 && ./aigle-darwin-arm64 -open",
	},
	{
		ID:          "mac-amd64",
		OS:          "macos",
		Arch:        "amd64",
		Label:       "macOS Intel (x86_64)",
		Filename:    "aigle-darwin-amd64",
		Description: "Binaire natif x86_64 pour Mac à processeur Intel.",
		CommandHint: "chmod +x aigle-darwin-amd64 && ./aigle-darwin-amd64 -open",
	},
	{
		ID:          "linux-amd64",
		OS:          "linux",
		Arch:        "amd64",
		Label:       "Linux (x86_64 — Binaire Statique)",
		Filename:    "aigle-linux-amd64",
		Description: "Binaire statique (CGO_ENABLED=0) compatible Debian, Ubuntu, Fedora, RHEL, Arch et Alpine.",
		CommandHint: "chmod +x aigle-linux-amd64 && ./aigle-linux-amd64 -open",
	},
	{
		ID:          "pres-pptx",
		OS:          "presentation",
		Arch:        "pptx",
		Label:       "Présentation PowerPoint (.pptx)",
		Filename:    "aigle-presentation.pptx",
		Description: "Support de présentation officiel (12 diapositives : Cas concrets d'usage & Capacités fonctionnelles).",
		CommandHint: "Ouvrir avec Microsoft PowerPoint, LibreOffice Impress ou Google Slides",
	},
	{
		ID:          "pres-html",
		OS:          "presentation",
		Arch:        "html",
		Label:       "Présentation Interactive Web (.html)",
		Filename:    "aigle-presentation.html",
		Description: "Diaporama HTML5 autonome consultable directement dans le navigateur (navigation au clavier).",
		CommandHint: "Ouvrir dans n'importe quel navigateur web",
	},
}

func computeFileSHA256(fullPath string, info os.FileInfo) string {
	hashCacheMu.Lock()
	if c, ok := hashCache[fullPath]; ok && c.modTime.Equal(info.ModTime()) && c.size == info.Size() {
		hashCacheMu.Unlock()
		return c.sha256
	}
	hashCacheMu.Unlock()

	f, err := os.Open(fullPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	sum := hex.EncodeToString(h.Sum(nil))

	hashCacheMu.Lock()
	hashCache[fullPath] = cachedHash{
		modTime: info.ModTime(),
		size:    info.Size(),
		sha256:  sum,
	}
	hashCacheMu.Unlock()
	return sum
}

func formatByteSize(b int64) string {
	if b <= 0 {
		return "—"
	}
	const mb = 1024.0 * 1024.0
	if float64(b) >= mb {
		return fmt.Sprintf("%.2f Mo", float64(b)/mb)
	}
	return fmt.Sprintf("%.1f Ko", float64(b)/1024.0)
}

// handlePublicReleases retourne les métadonnées de la vitrine publique et la liste des binaires téléchargeables.
// Cet endpoint est PUBLIC (accessible sans authentification Authelia).
func (s *Server) handlePublicReleases(w http.ResponseWriter, r *http.Request) {
	assets := make([]ReleaseAsset, 0, len(allowedPublicCatalog))

	for _, item := range allowedPublicCatalog {
		entry := item
		entry.DownloadURL = "/public/downloads/" + item.Filename

		fullPath := filepath.Join(s.Cfg.DownloadsDir, item.Filename)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			entry.Available = true
			entry.SizeBytes = info.Size()
			entry.SizeFormatted = formatByteSize(info.Size())
			entry.SHA256 = computeFileSHA256(fullPath, info)
			entry.UpdatedAt = info.ModTime().UTC().Format(time.RFC3339)
		} else {
			entry.Available = false
			entry.SizeFormatted = "Non compilé"
		}
		assets = append(assets, entry)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_name":    "Aigle — Suite de Gestion de Projet (MS Project, Kanban, EVM & Wiki)",
		"version":     "2.0.0",
		"app_domain":  s.Cfg.AppDomain,
		"auth_domain": s.Cfg.AuthDomain,
		"saas_url":    "/app",
		"assets":      assets,
	})
}

// handlePublicDownloadFile sert en téléchargement direct un binaire ou une présentation depuis DownloadsDir.
// Sécurité : vérifie que le fichier demandé appartient strictement à la liste blanche allowedPublicCatalog.
func (s *Server) handlePublicDownloadFile(w http.ResponseWriter, r *http.Request) {
	rawName := r.PathValue("filename")
	// Protection stricte contre toute tentative de Path Traversal (../, \, /)
	if strings.Contains(rawName, "..") || strings.Contains(rawName, "/") || strings.Contains(rawName, "\\") {
		writeErr(w, http.StatusBadRequest, "Nom de fichier invalide.")
		return
	}
	cleanName := filepath.Base(rawName)

	var matched *ReleaseAsset
	for i := range allowedPublicCatalog {
		if allowedPublicCatalog[i].Filename == cleanName {
			matched = &allowedPublicCatalog[i]
			break
		}
	}
	if matched == nil {
		writeErr(w, http.StatusNotFound, "Fichier non répertorié dans le catalogue public des téléchargements.")
		return
	}

	fullPath := filepath.Join(s.Cfg.DownloadsDir, cleanName)
	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		writeErr(w, http.StatusNotFound, "Ce binaire n'est pas encore généré sur le serveur.")
		return
	}

	sha := computeFileSHA256(fullPath, info)
	if sha != "" {
		w.Header().Set("X-Checksum-SHA256", sha)
	}

	// Si c'est la présentation HTML et que l'utilisateur souhaite la visionner en ligne (?view=1)
	if strings.HasSuffix(cleanName, ".html") && r.URL.Query().Get("view") == "1" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", cleanName))
	}

	http.ServeFile(w, r, fullPath)
}

