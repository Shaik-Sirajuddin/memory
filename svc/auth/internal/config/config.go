package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port                   int
	DatabaseURL            string
	RedisURL               string
	JWTSecret              string
	JWTSecretPrevious      string
	JWTTTL                 time.Duration
	SigningTokenTTL        time.Duration
	SessionTTL             time.Duration
	OAuthStateTTL          time.Duration
	GoogleClientID         string
	GoogleClientSecret     string
	GoogleCallbackURL      string
	GitHubClientID         string
	GitHubClientSecret     string
	GitHubCallbackURL      string
	OrgDomainWhitelist     []string
	UpstreamBaseURL        string
	DefaultAccountType     string
	MasterSecretValue      string
	BCryptCost             int
	InitiateAuthRateLimit  int
	AuthenticateRateLimit  int
	RateLimitWindowSeconds int
}

func Load() (Config, error) {
	port := mustInt("PORT", 4000)
	jwtTTL := mustDurationSeconds("JWT_TTL_SECONDS", 900)
	signingTTL := mustDurationSeconds("SIGNING_TOKEN_TTL_SECONDS", 300)
	sessionTTL := mustDurationSeconds("SESSION_TTL_SECONDS", 86400)
	whitelist := splitCSV(os.Getenv("ORG_DOMAIN_WHITELIST"))
	cfg := Config{
		Port:                   port,
		DatabaseURL:            strings.TrimSpace(os.Getenv("DATABASE_URL")),
		RedisURL:               strings.TrimSpace(os.Getenv("REDIS_URL")),
		JWTSecret:              strings.TrimSpace(os.Getenv("JWT_SECRET")),
		JWTSecretPrevious:      strings.TrimSpace(os.Getenv("JWT_SECRET_PREVIOUS")),
		JWTTTL:                 jwtTTL,
		SigningTokenTTL:        signingTTL,
		SessionTTL:             sessionTTL,
		OAuthStateTTL:          mustDurationSeconds("OAUTH_STATE_TTL_SECONDS", 300),
		GoogleClientID:         strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID")),
		GoogleClientSecret:     strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET")),
		GoogleCallbackURL:      strings.TrimSpace(os.Getenv("GOOGLE_CALLBACK_URL")),
		GitHubClientID:         strings.TrimSpace(os.Getenv("GITHUB_CLIENT_ID")),
		GitHubClientSecret:     strings.TrimSpace(os.Getenv("GITHUB_CLIENT_SECRET")),
		GitHubCallbackURL:      strings.TrimSpace(os.Getenv("GITHUB_CALLBACK_URL")),
		OrgDomainWhitelist:     whitelist,
		UpstreamBaseURL:        defaultString(os.Getenv("UPSTREAM_BASE_URL"), "http://localhost:5000"),
		DefaultAccountType:     defaultString(os.Getenv("DEFAULT_ACCOUNT_TYPE"), "team"),
		MasterSecretValue:      strings.TrimSpace(os.Getenv("SERVICE_ACCOUNT_MASTER_KEY")),
		BCryptCost:             mustInt("BCRYPT_COST", 12),
		InitiateAuthRateLimit:  mustInt("INITIATE_AUTH_RATE_LIMIT", 20),
		AuthenticateRateLimit:  mustInt("AUTHENTICATE_RATE_LIMIT", 20),
		RateLimitWindowSeconds: mustInt("RATE_LIMIT_WINDOW_SECONDS", 60),
	}
	if cfg.JWTSecret == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}
	if cfg.MasterSecretValue == "" {
		cfg.MasterSecretValue = cfg.JWTSecret
	}
	return cfg, nil
}

func (c Config) JWTSecrets() [][]byte {
	secrets := [][]byte{decodeSecret(c.JWTSecret)}
	if c.JWTSecretPrevious != "" {
		secrets = append(secrets, decodeSecret(c.JWTSecretPrevious))
	}
	return secrets
}

func (c Config) MasterSecret() []byte {
	return decodeSecret(c.MasterSecretValue)
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, strings.ToLower(trimmed))
		}
	}
	return out
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func mustInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func mustDurationSeconds(name string, fallback int) time.Duration {
	return time.Duration(mustInt(name, fallback)) * time.Second
}

func decodeSecret(value string) []byte {
	if value == "" {
		return nil
	}
	if decoded, err := hex.DecodeString(strings.TrimSpace(value)); err == nil && len(decoded) >= 16 {
		return decoded
	}
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}
