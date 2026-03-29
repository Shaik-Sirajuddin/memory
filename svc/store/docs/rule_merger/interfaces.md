# Required Interfaces

The `RuleMerger` component relies on a set of external dependencies. To integrate it into the `store` service, the calling service must provide structs that implement these specific interfaces.

## 1. `Cache`
Used for distributed debouncing and state tracking. This interface maps closely to typical Redis clients (like `go-redis`).

```go
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
```

**Implementation Guide:**
A wrapper around the standard Redis client is needed. Since `RuleMerger` processes requests across servers or containers, a central Redis store (`SetNX`) acts as a global lock to ignore duplicate queues globally.

---

## 2. `AgentClient`
Used for evaluating instruction sequence permutations and compressing them.

```go
type AgentClient interface {
	IdentifyUniqueRules(ctx context.Context, rules []string) ([]string, error)
}
```

**Implementation Guide:**
This interface must call out to the internal LLM proxy layer. Given a contiguous block of modified instructions, it evaluates semantic uniqueness and removes duplicates or overly redundant rules across variations.

---

**Note on Events:** 
The `EventBus` interface was previously required as a passed dependency. As of Version 2, the `RuleMerger` exposes its own internal Publisher-Subscriber channels directly to callers. Please refer to `/pubsub.md` for registering the pub-sub listener on `merger.Subscribe()`.
