package auth

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"example.com/m/v2/internal/cache"
	"example.com/m/v2/internal/config"
	"example.com/m/v2/internal/domain"
	"example.com/m/v2/internal/oauth"
	"example.com/m/v2/internal/repo"
	"example.com/m/v2/internal/util"
)

type Service struct {
	repo      repo.Store
	tokens    cache.TokenStore
	limiter   cache.RateLimiter
	passwords PasswordHasher
	secrets   SecretCipher
	jwts      JWTManager
	oauth     oauth.Resolver
	cfg       config.Config
}

func NewService(store repo.Store, tokens cache.TokenStore, limiter cache.RateLimiter, passwords PasswordHasher, secrets SecretCipher, jwts JWTManager, oauthResolver oauth.Resolver, cfg config.Config) *Service {
	return &Service{
		repo:      store,
		tokens:    tokens,
		limiter:   limiter,
		passwords: passwords,
		secrets:   secrets,
		jwts:      jwts,
		oauth:     oauthResolver,
		cfg:       cfg,
	}
}

type AuthResponse struct {
	UID          string `json:"uid"`
	Email        string `json:"email,omitempty"`
	AccountType  string `json:"account_type,omitempty"`
	Organization string `json:"organization,omitempty"`
	SessionToken string `json:"session_token,omitempty"`
	JWT          string `json:"jwt,omitempty"`
	Provider     string `json:"provider,omitempty"`
	ProviderUID  string `json:"provider_uid,omitempty"`
}

func (s *Service) Signup(ctx context.Context, req domain.SignupRequest) (AuthResponse, error) {
	if err := validateSignup(req); err != nil {
		return AuthResponse{}, err
	}
	if req.Provider != "" {
		profile, err := s.oauth.Resolve(ctx, req.Provider, req.Token)
		if err != nil {
			return AuthResponse{}, err
		}
		return s.signupOAuth(ctx, profile, "")
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		return AuthResponse{}, err
	}
	hash, err := s.passwords.Hash(req.Password)
	if err != nil {
		return AuthResponse{}, err
	}
	accountType, org := detectAccountType(req.Email, s.cfg.OrgDomainWhitelist, s.cfg.DefaultAccountType)
	account, err := s.repo.CreateAccount(ctx, domain.Account{
		Email:        strings.ToLower(req.Email),
		PasswordHash: hash,
		AccountType:  accountType,
		Organization: org,
	})
	if err != nil {
		return AuthResponse{}, err
	}
	token := util.RandomToken("sess_", 24)
	_ = s.tokens.Set(ctx, cache.SessionKey(token), SessionState{AccountUID: account.UID}, s.cfg.SessionTTL)
	return AuthResponse{
		UID:          account.UID,
		Email:        account.Email,
		AccountType:  account.AccountType,
		Organization: account.Organization,
		SessionToken: token,
	}, nil
}

func (s *Service) Login(ctx context.Context, req domain.LoginRequest) (AuthResponse, error) {
	if err := validateLogin(req); err != nil {
		return AuthResponse{}, err
	}
	if req.Provider != "" {
		profile, err := s.oauth.Resolve(ctx, req.Provider, req.Token)
		if err != nil {
			return AuthResponse{}, err
		}
		return s.signupOAuth(ctx, profile, "")
	}
	account, err := s.repo.GetAccountByEmail(ctx, strings.ToLower(req.Email))
	if err != nil {
		return AuthResponse{}, err
	}
	if err := s.passwords.Compare(account.PasswordHash, req.Password); err != nil {
		return AuthResponse{}, err
	}
	token := util.RandomToken("sess_", 24)
	_ = s.tokens.Set(ctx, cache.SessionKey(token), SessionState{AccountUID: account.UID}, s.cfg.SessionTTL)
	return AuthResponse{
		UID:          account.UID,
		Email:        account.Email,
		AccountType:  account.AccountType,
		Organization: account.Organization,
		SessionToken: token,
	}, nil
}

func (s *Service) OAuthCallback(ctx context.Context, provider, token, state string) (AuthResponse, error) {
	_ = state
	profile, err := s.oauth.Resolve(ctx, provider, token)
	if err != nil {
		return AuthResponse{}, err
	}
	return s.signupOAuth(ctx, profile, token)
}

func (s *Service) signupOAuth(ctx context.Context, profile domain.OAuthProfile, rawToken string) (AuthResponse, error) {
	account, err := s.lookupOrCreateOAuthAccount(ctx, profile)
	if err != nil {
		return AuthResponse{}, err
	}
	link, err := s.repo.UpsertOAuth(ctx, domain.OAuthLink{
		AccountUID:  account.UID,
		Provider:    profile.Provider,
		ProviderUID: profile.ProviderUID,
		AccessToken: rawToken,
	})
	if err != nil {
		return AuthResponse{}, err
	}
	token := util.RandomToken("sess_", 24)
	_ = s.tokens.Set(ctx, cache.SessionKey(token), SessionState{AccountUID: account.UID}, s.cfg.SessionTTL)
	return AuthResponse{
		UID:          account.UID,
		Email:        account.Email,
		AccountType:  account.AccountType,
		Organization: account.Organization,
		SessionToken: token,
		Provider:     link.Provider,
		ProviderUID:  link.ProviderUID,
	}, nil
}

func (s *Service) lookupOrCreateOAuthAccount(ctx context.Context, profile domain.OAuthProfile) (domain.Account, error) {
	if profile.Email != "" {
		if account, err := s.repo.GetAccountByEmail(ctx, strings.ToLower(profile.Email)); err == nil {
			return account, nil
		}
		accountType, org := detectAccountType(profile.Email, s.cfg.OrgDomainWhitelist, s.cfg.DefaultAccountType)
		return s.repo.CreateAccount(ctx, domain.Account{
			Email:        strings.ToLower(profile.Email),
			AccountType:  accountType,
			Organization: org,
		})
	}
	if link, err := s.repo.GetOAuth(ctx, profile.Provider, profile.ProviderUID); err == nil {
		return s.repo.GetAccountByID(ctx, link.AccountUID)
	}
	return s.repo.CreateAccount(ctx, domain.Account{
		Email:        fmt.Sprintf("%s@%s.local", strings.ToLower(profile.ProviderUID), profile.Provider),
		AccountType:  s.cfg.DefaultAccountType,
		Organization: profile.Provider,
	})
}

func (s *Service) SessionAccount(ctx context.Context, token string) (domain.Account, error) {
	var state SessionState
	found, err := s.tokens.Get(ctx, cache.SessionKey(token), &state)
	if err != nil {
		return domain.Account{}, err
	}
	if !found {
		return domain.Account{}, errors.New("session not found")
	}
	return s.repo.GetAccountByID(ctx, state.AccountUID)
}

func detectAccountType(email string, whitelist []string, fallback string) (string, string) {
	parts := strings.Split(strings.ToLower(email), "@")
	if len(parts) != 2 {
		return fallback, ""
	}
	domainPart := parts[1]
	personal := map[string]struct{}{
		"gmail.com": {}, "outlook.com": {}, "hotmail.com": {}, "yahoo.com": {},
		"icloud.com": {}, "proton.me": {}, "protonmail.com": {},
	}
	if _, ok := personal[domainPart]; ok {
		return "user", ""
	}
	for _, allowed := range whitelist {
		if allowed == domainPart {
			return "team", domainPart
		}
	}
	if fallback == "" {
		fallback = "team"
	}
	return fallback, domainPart
}

func validateSignup(req domain.SignupRequest) error {
	if req.Provider == "" {
		if req.Email == "" || req.Password == "" {
			return errors.New("email and password are required")
		}
		if len(req.Password) < 8 {
			return errors.New("password must be at least 8 characters")
		}
		return nil
	}
	if req.Token == "" {
		return errors.New("provider token is required")
	}
	return nil
}

func validateLogin(req domain.LoginRequest) error {
	if req.Provider == "" {
		if req.Email == "" || req.Password == "" {
			return errors.New("email and password are required")
		}
		return nil
	}
	if req.Token == "" {
		return errors.New("provider token is required")
	}
	return nil
}

func VerifySecret(secretCiphertext, rawSecret string, secrets SecretCipher) bool {
	decrypted, err := secrets.Decrypt(secretCiphertext)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(decrypted), []byte(rawSecret)) == 1
}

func EncodeSecret(secret string, cipher SecretCipher) (string, error) {
	return cipher.Encrypt(secret)
}

func DecodeHMAC(secretCiphertext string, cipher SecretCipher) (string, error) {
	return cipher.Decrypt(secretCiphertext)
}

func ParseExpiry(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	ts, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, err
	}
	return &ts, nil
}

func Base64URL(input string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(input))
}
