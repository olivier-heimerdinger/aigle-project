package authelia

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aigle/aigle-server/internal/argon2"
)

var fileMu sync.Mutex

// Paramètres Argon2id recommandés par l'OWASP et Authelia v4
const (
	argonTime    uint32 = 3
	argonMemory  uint32 = 65536
	argonThreads uint8  = 4
	argonKeyLen  uint32 = 32
	argonSaltLen int    = 16
)

// HashPasswordArgon2id génère une empreinte PHC Argon2id compatible avec Authelia :
// $argon2id$v=19$m=65536,t=3,p=4$<salt_base64_raw>$<hash_base64_raw>
func HashPasswordArgon2id(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads, b64Salt, b64Hash), nil
}

// VerifyPasswordArgon2id vérifie en temps constant un mot de passe en clair contre un hash PHC Argon2id.
func VerifyPasswordArgon2id(password, encodedHash string) bool {
	encodedHash = strings.TrimSpace(encodedHash)
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}

	var memory, timeCost uint32
	var threads uint8
	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeCost, &threads)
	if err != nil || memory == 0 || timeCost == 0 || threads == 0 {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expectedHash) == 0 {
		return false
	}

	computed := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, uint32(len(expectedHash)))
	return subtle.ConstantTimeCompare(computed, expectedHash) == 1
}

// ReadUserPasswordHashFromFile lit le hash Argon2id actuel d'un utilisateur dans users_database.yml d'Authelia.
func ReadUserPasswordHashFromFile(usersFilePath, username, email string) (string, error) {
	fileMu.Lock()
	defer fileMu.Unlock()

	raw, err := os.ReadFile(usersFilePath)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(raw), "\n")
	blockStart, blockEnd := findUserBlock(lines, username, email)
	if blockStart == -1 {
		return "", errors.New("utilisateur introuvable dans users_database.yml")
	}
	for i := blockStart; i <= blockEnd; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "password:") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "password:"))
			val = strings.Trim(val, `"'`)
			return val, nil
		}
	}
	return "", errors.New("champ password introuvable pour cet utilisateur")
}

// UpdateUserPasswordInFile met à jour le hash Argon2id d'un utilisateur dans users_database.yml d'Authelia.
// Si l'utilisateur n'existe pas encore dans users_database.yml, il est ajouté automatiquement.
func UpdateUserPasswordInFile(usersFilePath, username, email, displayName, role, newArgonHash string) error {
	fileMu.Lock()
	defer fileMu.Unlock()

	raw, err := os.ReadFile(usersFilePath)
	if err != nil {
		return err
	}
	content := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(content, "\n")

	blockStart, blockEnd := findUserBlock(lines, username, email)
	if blockStart != -1 {
		updated := false
		for i := blockStart; i <= blockEnd; i++ {
			trimmed := strings.TrimSpace(lines[i])
			if strings.HasPrefix(trimmed, "password:") {
				indent := lines[i][:strings.Index(lines[i], "password:")]
				lines[i] = fmt.Sprintf(`%spassword: "%s"`, indent, newArgonHash)
				updated = true
				break
			}
		}
		if updated {
			return os.WriteFile(usersFilePath, []byte(strings.Join(lines, "\n")), 0600)
		}
	}

	// Si l'utilisateur n'était pas encore présent dans users_database.yml, on l'ajoute proprement
	if username == "" {
		username = strings.Split(email, "@")[0]
	}
	safeUser := regexp.MustCompile(`[^a-zA-Z0-9_.-]`).ReplaceAllString(username, "")
	if safeUser == "" {
		safeUser = "user"
	}
	if displayName == "" {
		displayName = safeUser
	}
	groupName := "developers"
	switch role {
	case "superadmin", "admin":
		groupName = "admins"
	case "manager":
		groupName = "managers"
	case "viewer":
		groupName = "viewers"
	}

	newBlock := fmt.Sprintf(`
  %s:
    disabled: false
    displayname: %q
    email: %q
    password: %q
    groups:
      - %s
`, safeUser, displayName, email, newArgonHash, groupName)

	trimmedContent := strings.TrimRight(content, "\n ") + "\n" + newBlock
	return os.WriteFile(usersFilePath, []byte(trimmedContent), 0600)
}

func findUserBlock(lines []string, username, email string) (int, int) {
	username = strings.ToLower(strings.TrimSpace(username))
	email = strings.ToLower(strings.TrimSpace(email))

	type block struct {
		key   string
		start int
		end   int
		mail  string
	}
	var blocks []block
	curr := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || trimmed == "users:" {
			continue
		}
		// Clé d'utilisateur indentée de 2 espaces (ex: "  admin:")
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(trimmed, ":") {
			if curr >= 0 {
				blocks[curr].end = i - 1
			}
			key := strings.ToLower(strings.TrimSuffix(trimmed, ":"))
			blocks = append(blocks, block{key: key, start: i, end: len(lines) - 1})
			curr = len(blocks) - 1
		} else if curr >= 0 && strings.HasPrefix(trimmed, "email:") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "email:"))
			val = strings.ToLower(strings.Trim(val, `"'`))
			blocks[curr].mail = val
		}
	}

	for _, b := range blocks {
		if (username != "" && b.key == username) || (email != "" && b.mail == email) {
			return b.start, b.end
		}
	}
	return -1, -1
}

// ============================================================================
// MOTEUR 2FA TOTP (RFC 6238 - SHA1, 6 chiffres, 30 secondes)
// ============================================================================

// GenerateTOTPSecret génère un secret Base32 aléatoire de 160 bits (20 octets) conforme à la RFC 6238.
func GenerateTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	encoder := base32.StdEncoding.WithPadding(base32.NoPadding)
	return encoder.EncodeToString(raw), nil
}

// BuildOTPAuthURI construit l'URI standard otpauth://totp/... pour les applications Authenticator.
func BuildOTPAuthURI(issuer, accountEmail, secret string) string {
	if issuer == "" {
		issuer = "Aigle SaaS"
	}
	label := url.PathEscape(fmt.Sprintf("%s:%s", issuer, accountEmail))
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", "6")
	q.Set("period", "30")
	return fmt.Sprintf("otpauth://totp/%s?%s", label, q.Encode())
}

// VerifyTOTPCode vérifie un code TOTP à 6 chiffres avec une tolérance d'horloge de ±1 fenêtre de 30s.
func VerifyTOTPCode(secret, code string, now time.Time) bool {
	code = strings.TrimSpace(strings.ReplaceAll(code, " ", ""))
	if len(code) != 6 {
		return false
	}
	cleanSecret := strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(secret, " ", "")))
	cleanSecret = strings.TrimRight(cleanSecret, "=")
	decoder := base32.StdEncoding.WithPadding(base32.NoPadding)
	key, err := decoder.DecodeString(cleanSecret)
	if err != nil || len(key) == 0 {
		return false
	}

	counter := uint64(now.Unix() / 30)
	for offset := int64(-1); offset <= 1; offset++ {
		c := uint64(int64(counter) + offset)
		expected := computeHOTP(key, c)
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// CurrentTOTPCode calcule le code TOTP actuel (utile pour les tests unitaires automatisés).
func CurrentTOTPCode(secret string, now time.Time) (string, error) {
	cleanSecret := strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(secret, " ", "")))
	cleanSecret = strings.TrimRight(cleanSecret, "=")
	decoder := base32.StdEncoding.WithPadding(base32.NoPadding)
	key, err := decoder.DecodeString(cleanSecret)
	if err != nil {
		return "", err
	}
	return computeHOTP(key, uint64(now.Unix()/30)), nil
}

func computeHOTP(key []byte, counter uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	binCode := (int(sum[offset]&0x7f) << 24) |
		(int(sum[offset+1]&0xff) << 16) |
		(int(sum[offset+2]&0xff) << 8) |
		int(sum[offset+3]&0xff)
	otp := binCode % 1000000
	return fmt.Sprintf("%06d", otp)
}

// ============================================================================
// SYNCHRONISATION DYNAMIQUE DES RÈGLES TWO_FACTOR AUTHELIA PAR PROJET
// ============================================================================

const (
	beginManagedBlock = "    # BEGIN AIGLE_TOTP_PROJECTS"
	endManagedBlock   = "    # END AIGLE_TOTP_PROJECTS"
)

// SyncProjectTwoFactorRules met à jour le fichier configuration.yml d'Authelia (s'il est accessible)
// en injectant une règle access_control 'two_factor' dédiée pour chaque projet ayant RequireTOTP = true.
func SyncProjectTwoFactorRules(configFilePath, appDomain string, totpProjectIDs []string) error {
	fileMu.Lock()
	defer fileMu.Unlock()

	raw, err := os.ReadFile(configFilePath)
	if err != nil {
		// En environnement de test ou sans volume Authelia monté, on ignore silencieusement
		return err
	}
	content := strings.ReplaceAll(string(raw), "\r\n", "\n")

	// Suppression de l'ancien bloc géré s'il existe
	if startIdx := strings.Index(content, beginManagedBlock); startIdx != -1 {
		if endIdx := strings.Index(content, endManagedBlock); endIdx != -1 && endIdx > startIdx {
			afterEnd := endIdx + len(endManagedBlock)
			if afterEnd < len(content) && content[afterEnd] == '\n' {
				afterEnd++
			}
			content = content[:startIdx] + content[afterEnd:]
		}
	}

	if len(totpProjectIDs) == 0 {
		return os.WriteFile(configFilePath, []byte(content), 0600)
	}

	var ruleBuilder strings.Builder
	ruleBuilder.WriteString(beginManagedBlock + "\n")
	ruleBuilder.WriteString(fmt.Sprintf("    - domain: '%s'\n", appDomain))
	ruleBuilder.WriteString("      resources:\n")
	for _, pid := range totpProjectIDs {
		safeID := regexp.MustCompile(`[^a-zA-Z0-9_-]`).ReplaceAllString(pid, "")
		if safeID != "" {
			ruleBuilder.WriteString(fmt.Sprintf("        - '^/api/projects/%s(/.*)?$'\n", safeID))
			ruleBuilder.WriteString(fmt.Sprintf("        - '^/app/projects/%s(/.*)?$'\n", safeID))
		}
	}
	ruleBuilder.WriteString("      policy: 'two_factor'\n")
	ruleBuilder.WriteString(endManagedBlock + "\n")

	targetAnchor := "  rules:\n"
	if idx := strings.Index(content, targetAnchor); idx != -1 {
		insertPos := idx + len(targetAnchor)
		content = content[:insertPos] + ruleBuilder.String() + content[insertPos:]
	}

	return os.WriteFile(configFilePath, []byte(content), 0600)
}

