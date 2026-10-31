// Point d'entrée principal d'Aigle Server (Édition SaaS Multi-Tenant & Conteneurisée).
// Ce binaire statique embarque :
//   - Le serveur HTTP durci (Timeouts anti-Slowloris, Rate-Limiting, En-têtes OWASP)
//   - Le middleware Zero-Trust Authelia ForwardAuth (Traefik)
//   - Le moteur PostgreSQL 16 (production SaaS) ou SQLite WAL (développement/tests)
//   - La sonde de santé intégrée (-healthcheck) pour Docker/Podman/Kubernetes sans curl/wget
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aigle/aigle-server/internal/api"
	"github.com/aigle/aigle-server/internal/config"
	"github.com/aigle/aigle-server/internal/database"
	"github.com/aigle/aigle-server/web"
)

func main() {
	cfg := config.Load()

	// Mode sonde de santé Docker / Podman / Kubernetes :
	// Permet d'exécuter ["/app/aigle-server", "-healthcheck"] dans un conteneur minimaliste
	// sans avoir besoin d'installer curl ou wget dans l'image finale.
	if cfg.RunHealthcheck {
		client := &http.Client{Timeout: 3 * time.Second}
		url := fmt.Sprintf("http://127.0.0.1:%s/api/health", cfg.Port)
		resp, err := client.Get(url)
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

	log.Println("=======================================================================")
	log.Println("  AIGLE SERVER - EDITION SAAS MULTI-TENANT (GO + SVELTE 5 + AUTHELIA)")
	log.Printf("  Auth Mode   : %s | DB Driver : %s\n", cfg.AuthMode, cfg.DBDriver)
	log.Printf("  Domaine App : https://%s\n", cfg.AppDomain)
	log.Printf("  Portail SSO : https://%s\n", cfg.AuthDomain)
	log.Println("=======================================================================")

	store, err := database.Open(cfg)
	if err != nil {
		log.Fatalf("[FATAL] Échec d'initialisation de la base de données (%s): %v", cfg.DBDriver, err)
	}
	defer store.DB.Close()

	handler := api.NewServer(store, cfg, web.GetDistFS())

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       time.Duration(cfg.ReadTimeoutSec) * time.Second,
		WriteTimeout:      time.Duration(cfg.WriteTimeoutSec) * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 Mo max pour les en-têtes HTTP
	}

	// Canal d'écoute des signaux système (SIGINT / SIGTERM) pour arrêt gracieux en conteneur
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[AIGLE-SERVER] Écoute HTTP active sur 0.0.0.0:%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Erreur critique du serveur HTTP: %v", err)
		}
	}()

	sig := <-stopCh
	log.Printf("[AIGLE-SERVER] Signal %v reçu : arrêt gracieux des connexions en cours...", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[WARN] Arrêt forcé du serveur HTTP: %v", err)
	}
	log.Println("[AIGLE-SERVER] Serveur arrêté proprement.")
}
