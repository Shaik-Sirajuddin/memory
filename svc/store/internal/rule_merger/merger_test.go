package rulemerger

import (
	"context"
	"sync"
	"testing"
	"time"
)

type mockCache struct {
	mu   sync.Mutex
	data map[string]interface{}
}

func (m *mockCache) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Simulated minimal delay to encourage races if not properly locked
	time.Sleep(1 * time.Millisecond)

	if _, exists := m.data[key]; exists {
		return false, nil
	}
	m.data[key] = value
	return true, nil
}

func (m *mockCache) Del(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *mockCache) Get(ctx context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, exists := m.data[key]; exists {
		return v.(string), nil
	}
	return "", nil
}

func (m *mockCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

// TestConcurrentEnqueue ensures that multiple overlapping requests for the same file 
// are correctly deduplicated so that only 1 instance actually reaches the queue pool over a 10-second period.
func TestConcurrentEnqueue(t *testing.T) {
	cache := &mockCache{data: make(map[string]interface{})}
	merger := NewRuleMerger(cache, nil)
	
	ctx := context.Background()
	fileID := "path/to/test/rules.txt"

	var wg sync.WaitGroup
	
	// Simulate 100 concurrent requests (e.g., from multiple agent edits at same timestamp)
	numConcurrent := 100 

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := merger.Enqueue(ctx, fileID); err != nil {
				t.Errorf("expected no error from Enqueue, got %v", err)
			}
		}()
	}

	wg.Wait()

	// Check the internalQueue length. Thanks to SetNX, only exactly 1 should make it through.
	if len(merger.internalQueue) != 1 {
		t.Fatalf("expected exactly 1 item in internal queue, got %d", len(merger.internalQueue))
	}
}
