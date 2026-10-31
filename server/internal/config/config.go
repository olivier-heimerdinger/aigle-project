// Package config gère la configuration du serveur SaaS Aigle selon la méthodologie "12-Factor App".
// Toutes les options sensibles (DSN PostgreSQL, secrets de proxy, domaines, politiques de sécurité)
// sont injectables par variables d'environnement (Docker, Podman, Kubernetes) ou par flags CLI.
package config

import (
	"flag"
	"os"
	"strconv"
	"strings"
)

// ServerConfig contient l'ensemble des paramètres d'exécution et de sécurité du serveur SaaS.
type ServerConfig struct {
	// Réseau & Serveur HTTP
	Port            string // Port interne du conteneur (par défaut "8080")
	AppDomain       string // Domaine public de l'application (ex: "aigle.heimerdinger.studio")
	AuthDomain      string // Domaine public du portail Authelia (ex: "airgle-auth.heimerdinger.studio")
	ReadTimeoutSec  int    // Timeout de lecture HTTP (protection Slowloris)
	WriteTimeoutSec int    // Timeout d'écriture HTTP
	MaxBodyBytes    int64  // Taille maximale d'un corps de requête JSON (par défaut 2 Mo)

	// Base de données (PostgreSQL en production Docker/K8s, SQLite possible en lab/test)
	DBDriver string // "postgres" ou "sqlite"
	DBDSN    string // URL PostgreSQL (postgres://user:pass@host:5432/db?sslmode=disable) ou chemin SQLite

	// Sécurité & Intégration SSO Authelia (ForwardAuth)
	AuthMode           string   // "authelia" (production derrière Traefik+Authelia) ou "dev" (développement local)
	TrustedProxySecret string   // Secret optionnel partagé entre Traefik/Authelia et Aigle (Header X-Aigle-Proxy-Secret)
	AdminGroups        []string // Groupes Authelia donnant le rôle "superadmin" (ex: ["admins", "superadmin", "aigle-admin"])
	ManagerGroups      []string // Groupes Authelia donnant le rôle "manager" (ex: ["managers", "chefs-de-projet"])
	DefaultUserEmail   string   // Utilisé uniquement si AuthMode == "dev"
	DefaultUserName    string   // Utilisé uniquement si AuthMode == "dev"

	// Protection Rate-Limiting (Anti-BruteForce / Anti-DoS applicatif)
	RateLimitRPS   float64 // Requêtes par seconde autorisées par IP
	RateLimitBurst int     // Pic (burst) autorisé par IP

	// Dossier contenant les binaires clients téléchargeables (Windows, macOS, Linux) pour la vitrine publique
	DownloadsDir string

	// Mode Healthcheck CLI (utilisé par l'instruction HEALTHCHECK du Dockerfile distroless/alpine)
	RunHealthcheck bool
}

// Load charge la configuration depuis les variables d'environnement et les arguments CLI.
func Load() *ServerConfig {
	cfg := &ServerConfig{
		Port:               getEnv("AIGLE_PORT", "8080"),
		AppDomain:          getEnv("AIGLE_APP_DOMAIN", "aigle.heimerdinger.studio"),
		AuthDomain:         getEnv("AIGLE_AUTH_DOMAIN", "airgle-auth.heimerdinger.studio"),
		ReadTimeoutSec:     getEnvInt("AIGLE_READ_TIMEOUT_SEC", 15),
		WriteTimeoutSec:    getEnvInt("AIGLE_WRITE_TIMEOUT_SEC", 30),
		MaxBodyBytes:       int64(getEnvInt("AIGLE_MAX_BODY_BYTES", 2*1024*1024)), // 2 Mo max
		DBDriver:           getEnv("AIGLE_DB_DRIVER", "postgres"),
		DBDSN:              getEnv("AIGLE_DB_DSN", "postgres://aigle:aigle@postgres:5432/aigle_saas?sslmode=disable"),
		AuthMode:           getEnv("AIGLE_AUTH_MODE", "authelia"),
		TrustedProxySecret: getEnv("AIGLE_TRUSTED_PROXY_SECRET", ""),
		AdminGroups:        splitCSV(getEnv("AIGLE_ADMIN_GROUPS", "admins,superadmin,aigle-admin")),
		ManagerGroups:      splitCSV(getEnv("AIGLE_MANAGER_GROUPS", "managers,aigle-manager")),
		DefaultUserEmail:   getEnv("AIGLE_DEV_USER_EMAIL", "admin@heimerdinger.studio"),
		DefaultUserName:    getEnv("AIGLE_DEV_USER_NAME", "Administrateur Aigle"),
		RateLimitRPS:       float64(getEnvInt("AIGLE_RATE_LIMIT_RPS", 25)),
		RateLimitBurst:     getEnvInt("AIGLE_RATE_LIMIT_BURST", 60),
		DownloadsDir:       getEnv("AIGLE_DOWNLOADS_DIR", "./downloads"),
	}

	flag.StringVar(&cfg.Port, "port", cfg.Port, "Port HTTP d'écoute")
	flag.StringVar(&cfg.DBDriver, "db-driver", cfg.DBDriver, "Driver SQL : 'postgres' ou 'sqlite'")
	flag.StringVar(&cfg.DBDSN, "db", cfg.DBDSN, "DSN PostgreSQL ou fichier SQLite")
	flag.StringVar(&cfg.AuthMode, "auth-mode", cfg.AuthMode, "Mode d'authentification : 'authelia' ou 'dev'")
	flag.StringVar(&cfg.DownloadsDir, "downloads-dir", cfg.DownloadsDir, "Dossier des binaires téléchargeables (Windows, macOS, Linux)")
	flag.BoolVar(&cfg.RunHealthcheck, "healthcheck", false, "Exécute une sonde HTTP de santé locale et quitte (pour Docker HEALTHCHECK)")
	flag.Parse()

	return cfg
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return fallback
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, strings.ToLower(trimmed))
		}
	}
	return out
}

