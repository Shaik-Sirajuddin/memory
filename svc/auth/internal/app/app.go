package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"example.com/m/v2/internal/auth"
	"example.com/m/v2/internal/cache"
	"example.com/m/v2/internal/config"
	"example.com/m/v2/internal/httpapi"
	"example.com/m/v2/internal/keys"
	"example.com/m/v2/internal/oauth"
	"example.com/m/v2/internal/rbac"
	"example.com/m/v2/internal/repo"
)

type Server struct {
	httpServer *http.Server
	closers    []func(context.Context) error
}

func MustLoadConfig() config.Config {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	return cfg
}

func New(cfg config.Config) (*Server, error) {
	store, closeRepo, err := repo.New(cfg)
	if err != nil {
		return nil, err
	}

	tokenStore, closeTokenStore, err := cache.New(cfg)
	if err != nil {
		if closeRepo != nil {
			_ = closeRepo(context.Background())
		}
		return nil, err
	}

	limiter := cache.NewLimiter(tokenStore)
	jwtManager := auth.NewJWTManager(cfg.JWTSecrets(), cfg.JWTTTL)
	passwordHasher := auth.NewPasswordHasher(cfg.BCryptCost)
	secretCipher := auth.NewSecretCipher(cfg.MasterSecret())
	oauthResolver := oauth.NewResolver(cfg)
	authService := auth.NewService(store, tokenStore, limiter, passwordHasher, secretCipher, jwtManager, oauthResolver, cfg)
	keyService := keys.NewService(store, tokenStore, limiter, jwtManager, secretCipher, cfg)
	rbacMatcher := rbac.NewMatcher()

	router, err := httpapi.NewRouter(httpapi.Deps{
		Config:      cfg,
		Auth:        authService,
		Keys:        keyService,
		Store:       store,
		TokenStore:  tokenStore,
		Limiter:     limiter,
		JWTManager:  jwtManager,
		RBACMatcher: rbacMatcher,
	})
	if err != nil {
		_ = closeTokenStore(context.Background())
		if closeRepo != nil {
			_ = closeRepo(context.Background())
		}
		return nil, err
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: router,
	}

	closers := []func(context.Context) error{}
	if closeTokenStore != nil {
		closers = append(closers, closeTokenStore)
	}
	if closeRepo != nil {
		closers = append(closers, closeRepo)
	}

	return &Server{httpServer: srv, closers: closers}, nil
}

func (s *Server) Run() error {
	if s == nil || s.httpServer == nil {
		return errors.New("server not initialized")
	}
	err := s.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil || s.httpServer == nil {
		return nil
	}
	err := s.httpServer.Shutdown(ctx)
	for _, closer := range s.closers {
		_ = closer(ctx)
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

func PublicURL(base string) *url.URL {
	u, _ := url.Parse(base)
	return u
}
