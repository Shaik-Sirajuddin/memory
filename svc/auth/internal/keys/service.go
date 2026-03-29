package keys

import (
	"context"
	"errors"
	"strings"
	"time"

	"example.com/m/v2/internal/auth"
	"example.com/m/v2/internal/cache"
	"example.com/m/v2/internal/config"
	"example.com/m/v2/internal/domain"
	"example.com/m/v2/internal/repo"
	"example.com/m/v2/internal/util"
)

type Service struct {
	repo    repo.Store
	tokens  cache.TokenStore
	limiter cache.RateLimiter
	jwts    auth.JWTManager
	secrets auth.SecretCipher
	cfg     config.Config
}

func NewService(store repo.Store, tokens cache.TokenStore, limiter cache.RateLimiter, jwts auth.JWTManager, secrets auth.SecretCipher, cfg config.Config) *Service {
	return &Service{
		repo:    store,
		tokens:  tokens,
		limiter: limiter,
		jwts:    jwts,
		secrets: secrets,
		cfg:     cfg,
	}
}

type ServiceAccountResponse struct {
	UID        string         `json:"uid"`
	AccessKey  string         `json:"access_key"`
	Name       string         `json:"name"`
	SecretKey  string         `json:"secret_key,omitempty"`
	ExpiryDate *time.Time     `json:"expiry_date,omitempty"`
	IsActive   bool           `json:"is_active"`
	Scopes     []domain.Scope `json:"scopes,omitempty"`
}

func (s *Service) Create(ctx context.Context, accountUID string, req domain.ServiceAccountCreateRequest) (ServiceAccountResponse, error) {
	if err := validateCreate(req); err != nil {
		return ServiceAccountResponse{}, err
	}
	scopes := normalizeScopes(req.Scopes)
	sa := domain.ServiceAccount{
		AccountUID: accountUID,
		Name:       req.Name,
		AccessKey:  "sa_" + util.RandomToken("", 16),
		IsActive:   true,
	}
	if req.SecretKey != "" {
		enc, err := s.secrets.Encrypt(req.SecretKey)
		if err != nil {
			return ServiceAccountResponse{}, err
		}
		sa.SecretKey = enc
	}
	expiry, err := auth.ParseExpiry(req.ExpiryDate)
	if err != nil {
		return ServiceAccountResponse{}, err
	}
	sa.ExpiryDate = expiry
	created, storedScopes, err := s.repo.CreateServiceAccount(ctx, sa, scopes)
	if err != nil {
		return ServiceAccountResponse{}, err
	}
	return ServiceAccountResponse{
		UID:        created.UID,
		AccessKey:  created.AccessKey,
		Name:       created.Name,
		SecretKey:  req.SecretKey,
		ExpiryDate: created.ExpiryDate,
		IsActive:   created.IsActive,
		Scopes:     storedScopes,
	}, nil
}

func (s *Service) Update(ctx context.Context, req domain.ServiceAccountUpdateRequest) (ServiceAccountResponse, error) {
	if err := validateUpdate(req); err != nil {
		return ServiceAccountResponse{}, err
	}
	expiry, err := auth.ParseExpiry(req.ExpiryDate)
	if err != nil {
		return ServiceAccountResponse{}, err
	}
	sa := domain.ServiceAccount{
		UID:        req.UID,
		Name:       req.Name,
		ExpiryDate: expiry,
		IsActive:   true,
	}
	if req.IsActive != nil {
		sa.IsActive = *req.IsActive
	}
	if req.SecretKey != "" {
		enc, err := s.secrets.Encrypt(req.SecretKey)
		if err != nil {
			return ServiceAccountResponse{}, err
		}
		sa.SecretKey = enc
	}
	updated, scopes, err := s.repo.UpdateServiceAccount(ctx, sa, normalizeScopes(req.Scopes))
	if err != nil {
		return ServiceAccountResponse{}, err
	}
	return ServiceAccountResponse{
		UID:        updated.UID,
		AccessKey:  updated.AccessKey,
		Name:       updated.Name,
		SecretKey:  req.SecretKey,
		ExpiryDate: updated.ExpiryDate,
		IsActive:   updated.IsActive,
		Scopes:     scopes,
	}, nil
}

func (s *Service) Delete(ctx context.Context, uid string) error {
	return s.repo.DeleteServiceAccount(ctx, uid)
}

func (s *Service) InitiateAuth(ctx context.Context, req domain.InitiateAuthRequest) (string, error) {
	if err := validateInitiate(req); err != nil {
		return "", err
	}
	if ok, err := s.limiter.Allow(ctx, "initiate:"+req.ServiceAccountToken, s.cfg.InitiateAuthRateLimit, time.Duration(s.cfg.RateLimitWindowSeconds)*time.Second); err != nil || !ok {
		if err != nil {
			return "", err
		}
		return "", errors.New("rate limit exceeded")
	}
	sa, _, err := s.repo.GetServiceAccountByAccessKey(ctx, req.ServiceAccountToken)
	if err != nil {
		return "", err
	}
	if !sa.IsActive {
		return "", errors.New("service account inactive")
	}
	if sa.ExpiryDate != nil && time.Now().After(*sa.ExpiryDate) {
		return "", errors.New("service account expired")
	}
	if sa.SecretKey == "" {
		return "", errors.New("service account has no secret")
	}
	token := util.RandomToken("st_", 32)
	state := auth.SigningTokenState{ServiceAccountUID: sa.UID, IssuedAt: time.Now().Unix()}
	if err := s.tokens.Set(ctx, cache.SigningKey(token), state, s.cfg.SigningTokenTTL); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) Authenticate(ctx context.Context, req domain.AuthenticateRequest) (string, error) {
	if err := validateAuthenticate(req); err != nil {
		return "", err
	}
	if ok, err := s.limiter.Allow(ctx, "authenticate:"+req.Token, s.cfg.AuthenticateRateLimit, time.Duration(s.cfg.RateLimitWindowSeconds)*time.Second); err != nil || !ok {
		if err != nil {
			return "", err
		}
		return "", errors.New("rate limit exceeded")
	}
	var state auth.SigningTokenState
	found, err := s.tokens.Get(ctx, cache.SigningKey(req.Token), &state)
	if err != nil {
		return "", err
	}
	if !found {
		return "", errors.New("signing token not found")
	}
	sa, scopes, err := s.repo.GetServiceAccountByID(ctx, state.ServiceAccountUID)
	if err != nil {
		return "", err
	}
	if !sa.IsActive {
		return "", errors.New("service account inactive")
	}
	if sa.ExpiryDate != nil && time.Now().After(*sa.ExpiryDate) {
		return "", errors.New("service account expired")
	}
	if sa.SecretKey != "" {
		secret, err := s.secrets.Decrypt(sa.SecretKey)
		if err != nil {
			return "", err
		}
		if !auth.VerifyHMAC([]byte(secret), []byte(req.Token), req.SignedPayload) {
			return "", errors.New("signature mismatch")
		}
	}
	account, err := s.repo.GetAccountByID(ctx, sa.AccountUID)
	if err != nil {
		return "", err
	}
	jwtToken, err := s.jwts.Sign(ctx, auth.JWTClaims{
		AccountUID:        account.UID,
		ServiceAccountUID: sa.UID,
		AccountType:       account.AccountType,
		Scopes:            scopes,
	})
	if err != nil {
		return "", err
	}
	claims, err := s.jwts.Parse(jwtToken)
	if err != nil {
		return "", err
	}
	if err := s.tokens.Set(ctx, cache.JWTKey(claims.ID), auth.JWTAuthState{
		ServiceAccountUID: sa.UID,
		AccountUID:        account.UID,
		Scopes:            scopes,
	}, s.cfg.JWTTTL); err != nil {
		return "", err
	}
	_ = s.tokens.Delete(ctx, cache.SigningKey(req.Token))
	return jwtToken, nil
}

func (s *Service) AccountFromSession(ctx context.Context, token string) (domain.Account, error) {
	var state auth.SessionState
	found, err := s.tokens.Get(ctx, cache.SessionKey(token), &state)
	if err != nil {
		return domain.Account{}, err
	}
	if !found {
		return domain.Account{}, errors.New("session not found")
	}
	return s.repo.GetAccountByID(ctx, state.AccountUID)
}

func normalizeScopes(scopes []domain.Scope) []domain.Scope {
	out := make([]domain.Scope, 0, len(scopes))
	for _, scope := range scopes {
		scope.Root = strings.ToLower(strings.TrimSpace(scope.Root))
		scope.Path = util.NormalizePath(strings.TrimSpace(scope.Path))
		scope.Scope = strings.ToLower(strings.TrimSpace(scope.Scope))
		if scope.Root == "" || scope.Path == "" || scope.Scope == "" {
			continue
		}
		out = append(out, scope)
	}
	return out
}

func validateCreate(req domain.ServiceAccountCreateRequest) error {
	if req.Name == "" {
		return errors.New("name required")
	}
	if len(req.Scopes) == 0 {
		return errors.New("at least one scope required")
	}
	return nil
}

func validateUpdate(req domain.ServiceAccountUpdateRequest) error {
	if req.UID == "" || req.Name == "" {
		return errors.New("uid and name required")
	}
	if len(req.Scopes) == 0 {
		return errors.New("at least one scope required")
	}
	return nil
}

func validateInitiate(req domain.InitiateAuthRequest) error {
	if req.ServiceAccountToken == "" {
		return errors.New("service account token required")
	}
	return nil
}

func validateAuthenticate(req domain.AuthenticateRequest) error {
	if req.Token == "" {
		return errors.New("token required")
	}
	return nil
}
