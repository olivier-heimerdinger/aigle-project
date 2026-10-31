// Package main est le point d'entrée de l'application tout-en-un Aigle.
// Il initialise la configuration (mode Local autonome ou mode Équipe SaaS),
// ouvre la connexion à la base de données (SQLite en pur Go par défaut ou PostgreSQL),
// exécute automatiquement les migrations et le jeu de données initial,
// puis démarre le serveur HTTP servant à la fois l'API REST/SSE et l'interface Svelte embarquée.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/aigle/aigle/internal/api"
	"github.com/aigle/aigle/internal/config"
	"github.com/aigle/aigle/internal/database"
	"github.com/aigle/aigle/web"
)

func main() {
	// 1. Chargement de la configuration depuis les variables d'environnement et flags CLI (-port, -mode, -db, -open)
	cfg := config.Load()

	// 2. Ouverture de la base de données + Migrations automatiques + Seed initial si la base est vierge
	store, err := database.Open(cfg)
	if err != nil {
		log.Fatalf("[Aigle] Erreur d'initialisation de la base de données : %v", err)
	}
	defer store.DB.Close()

	// 3. Création du serveur HTTP couplant l'API REST/SSE et le système de fichiers Svelte embarqué (go:embed)
	srv := api.NewServer(cfg, store, web.DistFS())
	addr := ":" + cfg.Port
	url := fmt.Sprintf("http://localhost:%s", cfg.Port)

	fmt.Println("================================================================")
	fmt.Printf("  AIGLE — Gestion de Projet Tout-en-un (MS Project + Kanban)\n")
	fmt.Printf("  Mode       : %s (Driver DB: %s | Fichier/DSN: %s)\n", cfg.Mode, store.Driver, cfg.DBDSN)
	fmt.Printf("  Interface  : %s\n", url)
	fmt.Println("================================================================")

	// 4. Ouverture automatique du navigateur par défaut si le flag -open est activé
	if cfg.OpenBrowser {
		go openBrowser(url)
	}

	// 5. Écoute HTTP bloquante
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatalf("[Aigle] Arrêt du serveur HTTP : %v", err)
	}
}

// openBrowser ouvre l'URL de l'interface web dans le navigateur par défaut du système d'exploitation.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
