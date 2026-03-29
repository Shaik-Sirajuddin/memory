# Usage & Integration

This document outlines how to integrate the `RuleMerger` component into the `store` layer. The service manages an asynchronous queue that consumes local rule branches into absolute instructions logically combined through external agent intelligence.

## 1. Initialization

Integration begins via dependency injection within main packages or structural factory methods. You must construct an implementation of the `Cache` and `AgentClient` dependencies outlined in `/interfaces.md`.

```go
import (
	"context"
	"time"
	"log"
	
	"example.com/m/v2/svc/store/internal/rule_merger"
)

// Initialize implementations (example Redis cache instance)
redisCache := NewRedisCacheWrapper(redisConn)
llmAgent := NewAgentService(apiClient)

// Create the RuleMerger
merger := rule_merger.NewRuleMerger(redisCache, llmAgent)

// Start the queue polling consumer locally
// (Run this globally when the service launches)
merger.Start()

defer merger.Stop()
```

## 2. Triggering File Merges

When a new temporal state rule is written (an "addition/deletion" tag or temporary file branch mapped to an actor), the caller must enqueue the file ID. The `RuleMerger` consumes the parent file ID string representations.

The internal structure assumes temporal edits take place matching `filepath.Glob` branches related to the parent `fileID` paths.

```go
ctx := context.Background()
mainFileId := "/path/to/main/rules.txt"

// Fire an enqueue request. 
// Do this asynchronously inside write event handlers.
err := merger.Enqueue(ctx, mainFileId)
if err != nil {
	log.Printf("failed queuing file %s: %v", mainFileId, err)
}

// 1. The Cache enforces a 10s cooldown window so surges of branch writes are squashed down to 1 queued action.
// 2. The routine will read all branch files resembling 'rules:*' appended by actions.
```

## 3. Stopping the Queue Gracefully

Ensuring that background workers do not abruptly close when the server terminates prevents data corruption, though file locks protect underlying strings.

```go
// Inside graceful shutdown hooks (e.g. signal INT/TERM listener)
merger.Stop()
```
`Stop()` blocks until all queued processing iterations currently executing complete, and properly cancels inner wait groups and contexts.
