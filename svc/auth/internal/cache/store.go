package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type TokenStore interface {
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
	Get(ctx context.Context, key string, dest any) (bool, error)
	Delete(ctx context.Context, key string) error
	Ping(ctx context.Context) error
}

type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
	Ping(ctx context.Context) error
}

func New(_ any) (TokenStore, func(context.Context) error, error) {
	return NewMemoryTokenStore(), func(context.Context) error { return nil }, nil
}

type MemoryTokenStore struct {
	mu    sync.Mutex
	items map[string]memoryItem
}

type memoryItem struct {
	value   []byte
	expires time.Time
	hasTTL  bool
}

func NewMemoryTokenStore() *MemoryTokenStore {
	return &MemoryTokenStore{items: map[string]memoryItem{}}
}

func (m *MemoryTokenStore) Set(_ context.Context, key string, value any, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	item := memoryItem{value: raw}
	if ttl > 0 {
		item.hasTTL = true
		item.expires = time.Now().Add(ttl)
	}
	m.items[key] = item
	return nil
}

func (m *MemoryTokenStore) Get(_ context.Context, key string, dest any) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[key]
	if !ok {
		return false, nil
	}
	if item.hasTTL && time.Now().After(item.expires) {
		delete(m.items, key)
		return false, nil
	}
	return true, json.Unmarshal(item.value, dest)
}

func (m *MemoryTokenStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, key)
	return nil
}

func (m *MemoryTokenStore) Ping(context.Context) error { return nil }

func NewLimiter(store TokenStore) RateLimiter {
	return &limiter{store: store}
}

type limiter struct {
	store TokenStore
}

type rateWindow struct {
	Count int       `json:"count"`
	Reset time.Time `json:"reset"`
}

func (l *limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	if limit <= 0 {
		return true, nil
	}
	var state rateWindow
	found, err := l.store.Get(ctx, "ratelimit:"+key, &state)
	if err != nil {
		return false, err
	}
	now := time.Now()
	if !found || now.After(state.Reset) {
		state = rateWindow{Count: 0, Reset: now.Add(window)}
	}
	if state.Count >= limit {
		return false, nil
	}
	state.Count++
	return true, l.store.Set(ctx, "ratelimit:"+key, state, time.Until(state.Reset))
}

func (l *limiter) Ping(context.Context) error { return nil }

func SessionKey(token string) string { return "auth:session:" + token }
func SigningKey(token string) string { return "auth:signing:" + token }
func JWTKey(jti string) string       { return "auth:jwt:" + jti }
func LimiterKey(prefix, value string) string {
	return fmt.Sprintf("%s:%s", prefix, value)
}

var ErrNotFound = errors.New("not found")

func NormalizeKey(k string) string { return strings.TrimSpace(k) }
