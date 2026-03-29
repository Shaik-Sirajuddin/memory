# Pub-Sub Pattern in Rule Merger

The `RuleMerger` employs an internal Publisher-Subscriber (Pub-Sub) pattern to notify the broader `store` layer—or any other registered clients—whenever a sequential instruction file is successfully merged and updated. 

This model replaces external dependency injection (like passing a static `EventBus` downward) with an inverted control flow: the `RuleMerger` acts as the definitive topic registry for all merge operations.

## How It Works

1. **Internal Registry**: The `RuleMerger` struct contains a thread-safe map (`subscribers`) that tracks all active listener channels. 
2. **Event Emission**: When the internal worker successfully dedups and modifies a file via `actionMergeFile`, it evaluates the exact lines (`seqno`) that were inserted, deleted, or altered. It then constructs a `FileUpdatedEvent` and multicasts it to all currently registered subscriber channels.
3. **Non-Blocking Delivery**: The event loop attempts to send events to channels immediately. If a channel buffer is unexpectedly full, the send is currently non-blocking (the event drops for that listener) to prevent slow consumers from halting the critical background merge queue.

### Event Structure

```go
type FileUpdatedEvent struct {
	FileID         string // The absolute path identifier of the main file
	ModifiedSeqNos []int  // Approximated 0-indexed bounds where rules were physically shifted/replaced
}
```

## Integrating as a Subscriber

Only **one endpoint** needs to listen to all events in typical use-cases, but the registry supports multiple listeners easily.

```go
// 1. Subscribe to the registry to receive a dedicated channel
evtChan := merger.Subscribe()

// 2. Make sure to gracefully unsubscribe when your worker context dies
defer merger.Unsubscribe(evtChan)

// 3. Listen to the channel in a background goroutine
go func() {
    for event := range evtChan {
        log.Printf("File modified: %s", event.FileID)
        log.Printf("Changes detected around rules: %v", event.ModifiedSeqNos)
        
        // Push these updates directly to active users over WebSockets
        // or sync them to an external distributed queue!
    }
}()
```

Clients consuming from this registry are responsible for managing what happens when concurrent updates overlap. Due to the rapid cadence of `RuleMerger` background jobs, utilizing the channel intelligently (like batching operations inside the listener) is advised!
