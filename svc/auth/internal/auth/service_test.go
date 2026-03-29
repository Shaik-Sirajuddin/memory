package auth

import (
	"context"
	"testing"
	"time"

	"example.com/m/v2/internal/cache"
	"example.com/m/v2/internal/config"
	"example.com/m/v2/internal/domain"
	"example.com/m/v2/internal/oauth"
	"example.com/m/v2/internal/repo"
)

func TestSignupAndLogin(t *testing.T) {
	cfg := config.Config{
		OrgDomainWhitelist:     []string{"acme.com"},
		DefaultAccountType:     "team",
		SessionTTL:             time.Minute,
		JWTTTL:                 time.Minute,
		SigningTokenTTL:        time.Minute,
		RateLimitWindowSeconds: 60,
		InitiateAuthRateLimit:  100,
		AuthenticateRateLimit:  100,
	}
	store := repo.NewMemoryStore()
	tokens := cache.NewMemoryTokenStore()
	jwts := NewJWTManager([][]byte{[]byte("secretsecretsecretsecretsecretsecret")}, time.Minute)
	svc := NewService(store, tokens, cache.NewLimiter(tokens), NewPasswordHasher(12), NewSecretCipher([]byte("mastersecretmastersecret")), jwts, oauth.NewResolver(cfg), cfg)

	res, err := svc.Signup(context.Background(), domain.SignupRequest{Email: "ada@acme.com", Password: "password123"})
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	if res.SessionToken == "" {
		t.Fatalf("expected session token")
	}
	login, err := svc.Login(context.Background(), domain.LoginRequest{Email: "ada@acme.com", Password: "password123"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if login.UID != res.UID {
		t.Fatalf("expected same uid")
	}
}
