package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"example.com/m/v2/internal/config"
	"example.com/m/v2/internal/domain"
	"example.com/m/v2/internal/util"
)

type Store interface {
	Health(ctx context.Context) error
	CreateAccount(ctx context.Context, in domain.Account) (domain.Account, error)
	GetAccountByEmail(ctx context.Context, email string) (domain.Account, error)
	GetAccountByID(ctx context.Context, uid string) (domain.Account, error)
	UpsertOAuth(ctx context.Context, link domain.OAuthLink) (domain.OAuthLink, error)
	GetOAuth(ctx context.Context, provider, providerUID string) (domain.OAuthLink, error)
	CreateServiceAccount(ctx context.Context, sa domain.ServiceAccount, scopes []domain.Scope) (domain.ServiceAccount, []domain.Scope, error)
	UpdateServiceAccount(ctx context.Context, sa domain.ServiceAccount, scopes []domain.Scope) (domain.ServiceAccount, []domain.Scope, error)
	DeleteServiceAccount(ctx context.Context, uid string) error
	GetServiceAccountByAccessKey(ctx context.Context, accessKey string) (domain.ServiceAccount, []domain.Scope, error)
	GetServiceAccountByID(ctx context.Context, uid string) (domain.ServiceAccount, []domain.Scope, error)
	ListScopes(ctx context.Context, serviceAccountUID string) ([]domain.Scope, error)
}

func New(cfg config.Config) (Store, func(context.Context) error, error) {
	_ = cfg
	return NewMemoryStore(), func(context.Context) error { return nil }, nil
}

type MemoryStore struct {
	mu             sync.RWMutex
	accounts       map[string]domain.Account
	accountsByMail map[string]string
	oauth          map[string]domain.OAuthLink
	service        map[string]domain.ServiceAccount
	serviceByKey   map[string]string
	scopes         map[string][]domain.Scope
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		accounts:       map[string]domain.Account{},
		accountsByMail: map[string]string{},
		oauth:          map[string]domain.OAuthLink{},
		service:        map[string]domain.ServiceAccount{},
		serviceByKey:   map[string]string{},
		scopes:         map[string][]domain.Scope{},
	}
}

func (m *MemoryStore) Health(context.Context) error { return nil }

func (m *MemoryStore) CreateAccount(_ context.Context, in domain.Account) (domain.Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.accountsByMail[strings.ToLower(in.Email)]; ok {
		return domain.Account{}, fmt.Errorf("account already exists")
	}
	if in.UID == "" {
		in.UID = util.UUID()
	}
	now := time.Now().UTC()
	in.CreatedAt = now
	in.UpdatedAt = now
	m.accounts[in.UID] = in
	m.accountsByMail[strings.ToLower(in.Email)] = in.UID
	return in, nil
}

func (m *MemoryStore) GetAccountByEmail(_ context.Context, email string) (domain.Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	uid, ok := m.accountsByMail[strings.ToLower(email)]
	if !ok {
		return domain.Account{}, sql.ErrNoRows
	}
	return m.accounts[uid], nil
}

func (m *MemoryStore) GetAccountByID(_ context.Context, uid string) (domain.Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	account, ok := m.accounts[uid]
	if !ok {
		return domain.Account{}, sql.ErrNoRows
	}
	return account, nil
}

func (m *MemoryStore) UpsertOAuth(_ context.Context, link domain.OAuthLink) (domain.OAuthLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if link.UID == "" {
		link.UID = util.UUID()
	}
	link.CreatedAt = time.Now().UTC()
	m.oauth[oauthKey(link.Provider, link.ProviderUID)] = link
	return link, nil
}

func (m *MemoryStore) GetOAuth(_ context.Context, provider, providerUID string) (domain.OAuthLink, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	link, ok := m.oauth[oauthKey(provider, providerUID)]
	if !ok {
		return domain.OAuthLink{}, sql.ErrNoRows
	}
	return link, nil
}

func (m *MemoryStore) CreateServiceAccount(_ context.Context, sa domain.ServiceAccount, scopes []domain.Scope) (domain.ServiceAccount, []domain.Scope, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sa.UID == "" {
		sa.UID = util.UUID()
	}
	now := time.Now().UTC()
	sa.CreatedAt = now
	sa.UpdatedAt = now
	if sa.AccessKey == "" {
		sa.AccessKey = "sa_" + util.RandomToken("", 12)
	}
	sa.IsActive = true
	m.service[sa.UID] = sa
	m.serviceByKey[sa.AccessKey] = sa.UID
	m.scopes[sa.UID] = cloneScopes(sa.UID, scopes)
	return sa, cloneScopes(sa.UID, scopes), nil
}

func (m *MemoryStore) UpdateServiceAccount(_ context.Context, sa domain.ServiceAccount, scopes []domain.Scope) (domain.ServiceAccount, []domain.Scope, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.service[sa.UID]
	if !ok {
		return domain.ServiceAccount{}, nil, sql.ErrNoRows
	}
	existing.Name = sa.Name
	if sa.SecretKey != "" {
		existing.SecretKey = sa.SecretKey
	}
	existing.ExpiryDate = sa.ExpiryDate
	if sa.AccessKey != "" && sa.AccessKey != existing.AccessKey {
		delete(m.serviceByKey, existing.AccessKey)
		existing.AccessKey = sa.AccessKey
		m.serviceByKey[sa.AccessKey] = sa.UID
	}
	existing.UpdatedAt = time.Now().UTC()
	existing.IsActive = sa.IsActive
	m.service[sa.UID] = existing
	m.scopes[sa.UID] = cloneScopes(sa.UID, scopes)
	return existing, cloneScopes(sa.UID, scopes), nil
}

func (m *MemoryStore) DeleteServiceAccount(_ context.Context, uid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sa, ok := m.service[uid]
	if !ok {
		return sql.ErrNoRows
	}
	delete(m.service, uid)
	delete(m.serviceByKey, sa.AccessKey)
	delete(m.scopes, uid)
	return nil
}

func (m *MemoryStore) GetServiceAccountByAccessKey(_ context.Context, accessKey string) (domain.ServiceAccount, []domain.Scope, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	uid, ok := m.serviceByKey[accessKey]
	if !ok {
		return domain.ServiceAccount{}, nil, sql.ErrNoRows
	}
	return m.service[uid], cloneScopes(uid, m.scopes[uid]), nil
}

func (m *MemoryStore) GetServiceAccountByID(_ context.Context, uid string) (domain.ServiceAccount, []domain.Scope, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sa, ok := m.service[uid]
	if !ok {
		return domain.ServiceAccount{}, nil, sql.ErrNoRows
	}
	return sa, cloneScopes(uid, m.scopes[uid]), nil
}

func (m *MemoryStore) ListScopes(_ context.Context, serviceAccountUID string) ([]domain.Scope, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneScopes(serviceAccountUID, m.scopes[serviceAccountUID]), nil
}

func cloneScopes(serviceAccountUID string, scopes []domain.Scope) []domain.Scope {
	if len(scopes) == 0 {
		return []domain.Scope{}
	}
	out := make([]domain.Scope, len(scopes))
	copy(out, scopes)
	now := time.Now().UTC()
	for i := range out {
		if out[i].UID == "" {
			out[i].UID = util.UUID()
		}
		out[i].ServiceAccountUID = serviceAccountUID
		out[i].CreatedAt = now
	}
	return out
}

func oauthKey(provider, providerUID string) string { return provider + ":" + providerUID }

var ErrNotFound = errors.New("not found")
