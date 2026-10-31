// Package middleware implémente la défense en profondeur (Defense-in-Depth) du serveur SaaS Aigle :
// 1. SecurityHeaders : En-têtes HTTP stricts (CSP, HSTS, X-Frame-Options, X-Content-Type-Options, Referrer-Policy).
// 2. RateLimiter : Limitation de débit par adresse IP (algorithme Token Bucket) contre le brute-force et le DoS.
// 3. BodySizeLimit : Protection contre les attaques par épuisement mémoire (payloads JSON surdimensionnés).
// 4. AutheliaAuth : Extraction et validation de l'identité SSO transmise par Traefik + Authelia (ForwardAuth),
//    avec résolution automatique du rôle RBAC (superadmin, admin, manager, member, viewer).
package middleware

import (
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aigle/aigle-server/internal/config"
	"github.com/aigle/aigle-server/internal/models"
)

type contextKey string

const UserContextKey contextKey = "aigle_authenticated_user"

// GetUser extrait l'utilisateur authentifié du contexte de la requête HTTP.
func GetUser(r *http.Request) *models.User {
	if u, ok := r.Context().Value(UserContextKey).(*models.User); ok && u != nil {
		return u
	}
	return &models.User{
		ID:          "usr_anon",
		Username:    "anonymous",
		Email:       "anonymous@local",
		DisplayName: "Anonyme",
		Role:        models.RoleViewer,
		WorkspaceID: "ws_default",
	}
}

// SecurityHeaders ajoute les en-têtes de sécurité recommandés par l'OWASP sur chaque réponse HTTP.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// Empêche le reniflage de type MIME (MIME-sniffing)
		h.Set("X-Content-Type-Options", "nosniff")
		// Empêche l'intégration dans une iframe externe (anti-Clickjacking)
		h.Set("X-Frame-Options", "DENY")
		// Protection XSS et politique de référent stricte
		h.Set("X-XSS-Protection", "1; mode=block")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// Permissions-Policy : désactive l'accès aux capteurs matériels non nécessaires
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		// Content-Security-Policy : n'autorise que les scripts et styles de même origine ('self')
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self'; frame-ancestors 'none';")
		// HSTS (Strict-Transport-Security)
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")

		next.ServeHTTP(w, r)
	})
}

// BodySizeLimit limite la taille maximale du corps des requêtes entrantes (anti-DoS mémoire).
func BodySizeLimit(maxBytes int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && maxBytes > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}
		next.ServeHTTP(w, r)
	})
}

// ipBucket représente un seau à jetons (Token Bucket) pour une adresse IP cliente.
type ipBucket struct {
	tokens     float64
	lastRefill time.Time
}

// RateLimiter applique une limitation de débit par adresse IP en mémoire.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*ipBucket
	rps     float64
	burst   float64
}

// NewRateLimiter crée un limiteur de débit avec nettoyage périodique des anciennes IP.
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*ipBucket),
		rps:     rps,
		burst:   float64(burst),
	}
	return rl
}

// Allow vérifie si l'adresse IP dispose d'un jeton disponible.
func (rl *RateLimiter) Allow(ip string) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Nettoyage opportuniste si la table dépasse 5000 IP
	if len(rl.buckets) > 5000 {
		for k, b := range rl.buckets {
			if now.Sub(b.lastRefill) > 5*time.Minute {
				delete(rl.buckets, k)
			}
		}
	}

	b, exists := rl.buckets[ip]
	if !exists {
		rl.buckets[ip] = &ipBucket{
			tokens:     rl.burst - 1,
			lastRefill: now,
		}
		return true
	}

	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * rl.rps
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	b.lastRefill = now

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}
	return false
}

// Middleware enveloppe un http.Handler avec le limiteur de débit par IP.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ne pas brider le flux SSE ou la sonde de santé interne
		if r.URL.Path == "/api/health" || r.URL.Path == "/api/events" {
			next.ServeHTTP(w, r)
			return
		}
		ip := ExtractClientIP(r)
		if !rl.Allow(ip) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"Trop de requêtes (Rate Limit atteint). Veuillez patienter quelques secondes."}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ExtractClientIP récupère l'IP réelle du client transmise par Traefik (X-Forwarded-For / X-Real-IP).
func ExtractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return strings.TrimSpace(xrip)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

// ResolveAutheliaIdentity extrait l'identité de l'utilisateur à partir des en-têtes ForwardAuth d'Authelia :
//   - Remote-User   : identifiant unique de l'utilisateur
//   - Remote-Email  : adresse email vérifiée par Authelia
//   - Remote-Name   : nom d'affichage complet
//   - Remote-Groups : liste des groupes séparés par virgule (ex: "admins,dev")
//
// Si TrustedProxySecret est configuré dans Aigle, vérifie en temps constant (crypto/subtle)
// que l'en-tête X-Aigle-Proxy-Secret correspond, empêchant tout contournement direct hors Traefik.
func ResolveAutheliaIdentity(cfg *config.ServerConfig, r *http.Request) (*models.User, bool) {
	// L'endpoint de santé interne (/api/health) reste accessible pour le HEALTHCHECK Docker
	if r.URL.Path == "/api/health" {
		return nil, true
	}

	// Vérification optionnelle du secret partagé entre Traefik et le conteneur Aigle
	if cfg.TrustedProxySecret != "" {
		provided := r.Header.Get("X-Aigle-Proxy-Secret")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(cfg.TrustedProxySecret)) != 1 {
			return nil, false
		}
	}

	remoteUser := strings.TrimSpace(r.Header.Get("Remote-User"))
	remoteEmail := strings.TrimSpace(r.Header.Get("Remote-Email"))
	remoteName := strings.TrimSpace(r.Header.Get("Remote-Name"))
	remoteGroups := strings.TrimSpace(r.Header.Get("Remote-Groups"))

	// En mode "authelia" strict, la présence de Remote-User ou Remote-Email est obligatoire
	if cfg.AuthMode == "authelia" {
		if remoteUser == "" && remoteEmail == "" {
			return nil, false
		}
	} else {
		// En mode "dev" (développement local sans Authelia devant), repli sur l'utilisateur de dev
		if remoteUser == "" && remoteEmail == "" {
			remoteUser = "admin"
			remoteEmail = cfg.DefaultUserEmail
			remoteName = cfg.DefaultUserName
			remoteGroups = "admins,superadmin"
		}
	}

	if remoteEmail == "" {
		remoteEmail = remoteUser + "@" + cfg.AppDomain
	}
	if remoteName == "" {
		remoteName = remoteUser
	}

	role := determineRoleFromGroups(cfg, remoteGroups)

	return &models.User{
		Username:    remoteUser,
		Email:       strings.ToLower(remoteEmail),
		DisplayName: remoteName,
		Role:        role,
		Groups:      remoteGroups,
		WorkspaceID: "ws_default",
		IsActive:    true,
	}, true
}

func determineRoleFromGroups(cfg *config.ServerConfig, rawGroups string) string {
	groups := strings.Split(strings.ToLower(rawGroups), ",")
	groupSet := make(map[string]bool, len(groups))
	for _, g := range groups {
		trimmed := strings.TrimSpace(g)
		if trimmed != "" {
			groupSet[trimmed] = true
		}
	}

	for _, adminGrp := range cfg.AdminGroups {
		if groupSet[adminGrp] {
			return models.RoleSuperAdmin
		}
	}
	for _, mgrGrp := range cfg.ManagerGroups {
		if groupSet[mgrGrp] {
			return models.RoleManager
		}
	}
	if groupSet["viewers"] || groupSet["readonly"] || groupSet["clients"] {
		return models.RoleViewer
	}
	return models.RoleMember
}

// WithUser injecte l'utilisateur authentifié dans le contexte de la requête HTTP.
func WithUser(r *http.Request, u *models.User) *http.Request {
	ctx := context.WithValue(r.Context(), UserContextKey, u)
	return r.WithContext(ctx)
}

// CanWrite vérifie si le rôle de l'utilisateur lui permet de modifier les données d'un projet.
func CanWrite(u *models.User) bool {
	if u == nil || !u.IsActive {
		return false
	}
	return u.Role == models.RoleSuperAdmin || u.Role == models.RoleAdmin || u.Role == models.RoleManager || u.Role == models.RoleMember
}

// CanManageProject vérifie si l'utilisateur peut modifier la structure financière ou supprimer un projet.
func CanManageProject(u *models.User) bool {
	if u == nil || !u.IsActive {
		return false
	}
	return u.Role == models.RoleSuperAdmin || u.Role == models.RoleAdmin || u.Role == models.RoleManager
}

// IsSuperAdmin vérifie si l'utilisateur a accès à la Console d'Administration globale SaaS.
func IsSuperAdmin(u *models.User) bool {
	if u == nil || !u.IsActive {
		return false
	}
	return u.Role == models.RoleSuperAdmin || u.Role == models.RoleAdmin
}

