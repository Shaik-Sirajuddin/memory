package rulemerger

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Cache defines the interface for interacting with Redis to handle debounce/cooldowns.
type Cache interface {
	// SetNX sets a key only if it does not exist, with an expiration time.
	// Returns true if the key was set, false otherwise.
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error)
	// Del deletes a key from the cache.
	Del(ctx context.Context, key string) error
	// Get string value
	Get(ctx context.Context, key string) (string, error)
	// Set string value
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
}

// AgentClient defines the interface for interacting with the LLM agent.
type AgentClient interface {
	IdentifyUniqueRules(ctx context.Context, rules []string) ([]string, error)
}

// FileUpdatedEvent defines an event when a file completes a merge cycle.
type FileUpdatedEvent struct {
	FileID         string
	ModifiedSeqNos []int
}

// RuleMerger orchestrates the merging of temporary branch files into the main rule file.
type RuleMerger struct {
	cache       Cache
	agent       AgentClient
	
	subscribers map[chan FileUpdatedEvent]struct{}
	subMu       sync.RWMutex
	
	internalQueue chan string
	
	cooldown time.Duration
	
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewRuleMerger initializes a new RuleMerger.
func NewRuleMerger(cache Cache, agent AgentClient) *RuleMerger {
	ctx, cancel := context.WithCancel(context.Background())
	return &RuleMerger{
		cache:         cache,
		agent:         agent,
		subscribers:   make(map[chan FileUpdatedEvent]struct{}),
		internalQueue: make(chan string, 10000), // Buffered queue
		cooldown:      10 * time.Second,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start begins processing the internal queue.
func (m *RuleMerger) Start() {
	m.wg.Add(1)
	go m.processQueue()
}

// Stop cleanly shuts down the merger.
func (m *RuleMerger) Stop() {
	m.cancel()
	m.wg.Wait()
	close(m.internalQueue)
}

// Subscribe provides a channel to listen to all file merge events.
func (m *RuleMerger) Subscribe() chan FileUpdatedEvent {
	m.subMu.Lock()
	defer m.subMu.Unlock()
	ch := make(chan FileUpdatedEvent, 100)
	m.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a channel from the pub-sub registry.
func (m *RuleMerger) Unsubscribe(ch chan FileUpdatedEvent) {
	m.subMu.Lock()
	defer m.subMu.Unlock()
	if _, ok := m.subscribers[ch]; ok {
		delete(m.subscribers, ch)
		close(ch)
	}
}

// emitEvent multicasts the file update event to all active subscribers.
func (m *RuleMerger) emitEvent(event FileUpdatedEvent) {
	m.subMu.RLock()
	defer m.subMu.RUnlock()
	for ch := range m.subscribers {
		select {
		case ch <- event:
		default:
			// Non-blocking send; in a robust system this might need larger buffers or drop policies.
		}
	}
}

// Enqueue adds a file to the processing queue. It skips duplicates if another request
// for the same file was made within the last 10 seconds.
func (m *RuleMerger) Enqueue(ctx context.Context, fileID string) error {
	// Enqueue with a 10s timeout cache flag to prevent duplicate enqueues
	cacheKey := fmt.Sprintf("rulemerger:enqueue_timeout:%s", fileID)
	
	enqueued, err := m.cache.SetNX(ctx, cacheKey, "enqueued", m.cooldown)
	if err != nil {
		return fmt.Errorf("failed to check deduplication cache: %w", err)
	}
	
	if !enqueued {
		// Already enqueued within the timeout window; ignore duplicate.
		return nil
	}
	
	select {
	case m.internalQueue <- fileID:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Queue is full. If tracking strict non-dropping semantics, block until timeout.
		// For now, retry pushing contextually.
		select {
		case m.internalQueue <- fileID:
		case <-time.After(5 * time.Second):
			return fmt.Errorf("internal queue timeout")
		}
		return nil
	}
}

func (m *RuleMerger) processQueue() {
	defer m.wg.Done()
	
	for {
		select {
		case <-m.ctx.Done():
			return
		case fileID := <-m.internalQueue:
			// "maintain a min gap of 10 seconds for processal"
			// "use redis to maintain last proceed times"
			lastProcKey := fmt.Sprintf("rulemerger:last_processed:%s", fileID)
			
			// Simple backoff check
			val, err := m.cache.Get(m.ctx, lastProcKey)
			if err == nil && val != "" {
				lastProcTime, parseErr := time.Parse(time.RFC3339Nano, val)
				if parseErr == nil {
					elapsed := time.Since(lastProcTime)
					if elapsed < m.cooldown {
						// Wait out the remainder of the 10s cooldown
						waitDuration := m.cooldown - elapsed
						select {
						case <-time.After(waitDuration):
						case <-m.ctx.Done():
							return
						}
					}
				}
			}

			// Perform merge
			m.actionMergeFile(m.ctx, fileID)

			// Update last processed time
			_ = m.cache.Set(m.ctx, lastProcKey, time.Now().Format(time.RFC3339Nano), 24*time.Hour)
		}
	}
}
