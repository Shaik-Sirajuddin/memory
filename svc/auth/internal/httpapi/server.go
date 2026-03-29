package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"example.com/m/v2/internal/auth"
	"example.com/m/v2/internal/cache"
	"example.com/m/v2/internal/config"
	"example.com/m/v2/internal/domain"
	"example.com/m/v2/internal/keys"
	"example.com/m/v2/internal/rbac"
	"example.com/m/v2/internal/repo"
)

type Deps struct {
	Config      config.Config
	Auth        *auth.Service
	Keys        *keys.Service
	Store       repo.Store
	TokenStore  cache.TokenStore
	Limiter     cache.RateLimiter
	JWTManager  auth.JWTManager
	RBACMatcher rbac.Matcher
}

type contextKey string

const (
	accountKey contextKey = "account"
	claimsKey  contextKey = "claims"
	stateKey   contextKey = "state"
)

func NewRouter(deps Deps) (http.Handler, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "db": "up", "redis": "up"})
	})

	mux.HandleFunc("/auth/signup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var req domain.SignupRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		res, err := deps.Auth.Signup(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, res)
	})

	mux.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var req domain.LoginRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		res, err := deps.Auth.Login(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	mux.HandleFunc("/auth/oauth/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/auth/oauth/")
		parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
		if len(parts) != 2 || parts[1] != "callback" {
			http.NotFound(w, r)
			return
		}
		provider := parts[0]
		token := firstNonEmpty(r.URL.Query().Get("code"), r.URL.Query().Get("token"), r.URL.Query().Get("email"))
		res, err := deps.Auth.OAuthCallback(r.Context(), provider, token, r.URL.Query().Get("state"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	mux.Handle("/keys/service-account/create", requireSession(deps, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var req domain.ServiceAccountCreateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		account := accountFromContext(r.Context())
		res, err := deps.Keys.Create(r.Context(), account.UID, req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, res)
	})))

	mux.Handle("/keys/service-account/update", requireSession(deps, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			methodNotAllowed(w)
			return
		}
		var req domain.ServiceAccountUpdateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		res, err := deps.Keys.Update(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})))

	mux.Handle("/keys/service-account/delete", requireSession(deps, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			methodNotAllowed(w)
			return
		}
		uid := r.URL.Query().Get("uid")
		if uid == "" {
			writeError(w, http.StatusBadRequest, errors.New("uid required"))
			return
		}
		if err := deps.Keys.Delete(r.Context(), uid); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))

	mux.Handle("/keys/initiate-auth", requireSession(deps, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var req domain.InitiateAuthRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		token, err := deps.Keys.InitiateAuth(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"token": token})
	})))

	mux.Handle("/keys/authenticate", requireSession(deps, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var req domain.AuthenticateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		token, err := deps.Keys.Authenticate(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"jwt": token})
	})))

	upstream, _ := url.Parse(deps.Config.UpstreamBaseURL)
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = upstream.Scheme
		req.URL.Host = upstream.Host
		req.Host = upstream.Host
		req.Header.Del("X-Internal-Auth")
		req.Header.Del("X-Internal-Token")
	}
	storeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	})
	mux.Handle("/store/", requireJWT(deps, requireRBAC(deps, storeHandler)))

	return chain(mux, jsonLogger, requestID, cors), nil
}

func chain(next http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		next = middlewares[i](next)
	}
	return next
}

func jsonLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		_ = start
	})
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", time.Now().Format("20060102150405.000000000"))
		next.ServeHTTP(w, r)
	})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requireSession(deps Deps, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			writeError(w, http.StatusUnauthorized, errors.New("missing session token"))
			return
		}
		account, err := deps.Auth.SessionAccount(r.Context(), token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err)
			return
		}
		ctx := context.WithValue(r.Context(), accountKey, account)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requireJWT(deps Deps, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			writeError(w, http.StatusUnauthorized, errors.New("missing bearer token"))
			return
		}
		claims, err := deps.JWTManager.Parse(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, errors.New("invalid token"))
			return
		}
		var state auth.JWTAuthState
		found, err := deps.TokenStore.Get(r.Context(), cache.JWTKey(claims.ID), &state)
		if err != nil || !found {
			writeError(w, http.StatusUnauthorized, errors.New("token revoked"))
			return
		}
		ctx := context.WithValue(r.Context(), claimsKey, claims)
		ctx = context.WithValue(ctx, stateKey, state)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requireRBAC(deps Deps, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawClaims := r.Context().Value(claimsKey)
		rawState := r.Context().Value(stateKey)
		claims, _ := rawClaims.(auth.JWTClaims)
		state, _ := rawState.(auth.JWTAuthState)
		if !deps.RBACMatcher.Allowed(r.URL.Path, state.Scopes, claims.AccountType, r.Method) {
			writeError(w, http.StatusForbidden, errors.New("rbac denied"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func accountFromContext(ctx context.Context) domain.Account {
	if v := ctx.Value(accountKey); v != nil {
		if account, ok := v.(domain.Account); ok {
			return account
		}
	}
	return domain.Account{}
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return ""
	}
	return strings.TrimSpace(header[7:])
}

func decodeJSON(r *http.Request, dest any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dest)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
