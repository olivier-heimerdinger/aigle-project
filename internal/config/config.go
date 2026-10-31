// Package config gère le paramétrage d'exécution d'Aigle.
// Il permet au même binaire de fonctionner soit en mode Local mono-poste (SQLite),
// soit en mode Équipe / SaaS (connecté à un serveur SQL externe).
package config

import (
	"flag"
	"os"
)

// Config regroupe les paramètres d'exécution du serveur Aigle.
type Config struct {
	Mode        string // "local" (mono-poste / petite équipe locale) ou "saas" (hébergement multi-tenant externe)
	DBDriver    string // "sqlite" (fichier local sans CGO) ou "postgres" (base externe)
	DBDSN       string // Chemin du fichier SQLite (ex: "aigle.db") ou chaîne de connexion PostgreSQL
	Port        string // Port d'écoute HTTP (par défaut "8080")
	JWTSecret   string // Clé secrète pour la signature des jetons en mode SaaS
	OpenBrowser bool   // Si vrai (-open), ouvre automatiquement le navigateur au lancement
}

// Load lit la configuration depuis les variables d'environnement puis applique les flags de ligne de commande.
func Load() *Config {
	cfg := &Config{
		Mode:      getEnv("AIGLE_MODE", "local"),
		DBDriver:  getEnv("AIGLE_DB_DRIVER", "sqlite"),
		DBDSN:     getEnv("AIGLE_DB_DSN", "aigle.db"),
		Port:      getEnv("AIGLE_PORT", "8080"),
		JWTSecret: getEnv("AIGLE_JWT_SECRET", "aigle-local-dev-secret-change-in-saas"),
	}

	flag.StringVar(&cfg.Mode, "mode", cfg.Mode, "Mode d'exécution : 'local' ou 'saas'")
	flag.StringVar(&cfg.DBDriver, "db-driver", cfg.DBDriver, "Driver de base de données : 'sqlite' ou 'postgres'")
	flag.StringVar(&cfg.DBDSN, "db", cfg.DBDSN, "Chemin du fichier SQLite ou DSN PostgreSQL")
	flag.StringVar(&cfg.Port, "port", cfg.Port, "Port HTTP du serveur")
	flag.BoolVar(&cfg.OpenBrowser, "open", false, "Ouvrir automatiquement le navigateur au démarrage")
	flag.Parse()

	return cfg
}

// getEnv récupère une variable d'environnement ou retourne la valeur par défaut si elle est absente.
func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return fallback
}
