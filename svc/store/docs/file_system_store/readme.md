# File System Store

This document explains how other services can use the `store` package as a filesystem-backed instruction store.

The implementation provides:

- A concrete `Store` interface for reads, writes, folder indexing, and subscriptions.
- A `FileSystem` implementation that stores main instructions and session temp branches on disk.
- An HTTP handler for services that want to expose the store over REST and SSE.
- An OpenAPI spec at [openapi.yaml](./openapi.yaml).

## Core Interfaces

### `Store`

```go
type Store interface {
	GetInstructions(ctx context.Context, id InstructionID) (InstructionDocument, error)
	GetInstructionsMeta(ctx context.Context, id InstructionID) (InstructionMeta, error)
	GetInstructionsPartial(ctx context.Context, id InstructionID, params GetPartialInstructionParams) (InstructionDocument, error)
	GetFolderIndex(ctx context.Context, id InstructionID) ([]FolderEntry, error)

	AppendInstructions(ctx context.Context, id InstructionID, params AppendInstructionParams) (InstructionDocument, error)
	UpdateInstructions(ctx context.Context, id InstructionID, params UpdateInstructionParams) (InstructionDocument, error)
	DeleteInstructions(ctx context.Context, id InstructionID, params DeleteInstructionParams) (InstructionDocument, error)
	DiscardInstructions(ctx context.Context, id InstructionID, params DiscardInstructionParams) (InstructionDocument, error)

	SubscribeInstructions(ctx context.Context, id InstructionID, params SubscriptionParams) (Subscription, error)
}
```

### `InstructionID`

```go
type InstructionID struct {
	AccountPrefix string
	Bucket        string
	Path          string
	FileName      string
	SessionID     string
}
```

Callers should resolve request headers and route params before using the store:

- `AccountPrefix` should already be normalized, for example `u_123` or `t_456`.
- `Bucket` should contain the target bucket name, or `default` when empty.
- `Path` should point to the folder or relative file path.
- `FileName` should be provided when the target is a single file.
- `SessionID` should identify the caller session for temp-branch writes and subscriptions.

### Write and Read Payloads

```go
type AppendInstructionParams struct {
	Rules []Rule
}

type UpdateInstructionParams struct {
	Rules []Rule
}

type DeleteInstructionParams struct {
	SeqNo []int
}

type DiscardInstructionParams struct {
	SeqNo []int
}

type GetPartialInstructionParams struct {
	Range
}
```

## In-Process Usage

### Minimal store setup

```go
package main

import (
	"context"
	"log"
	"time"

	store "example.com/m/v2"
)

type redisCache struct{}

func (redisCache) SetNX(context.Context, string, interface{}, time.Duration) (bool, error) { return true, nil }
func (redisCache) Del(context.Context, string) error                                       { return nil }
func (redisCache) Get(context.Context, string) (string, error)                             { return "", nil }
func (redisCache) Set(context.Context, string, interface{}, time.Duration) error           { return nil }

type merger struct{}

func (merger) Enqueue(context.Context, string) error { return nil }
func (merger) Subscribe() chan store.FileUpdatedEvent {
	return make(chan store.FileUpdatedEvent)
}
func (merger) Unsubscribe(ch chan store.FileUpdatedEvent) {
	close(ch)
}

func main() {
	fs, err := store.NewFileSystem("/var/lib/my-service", redisCache{}, merger{})
	if err != nil {
		log.Fatal(err)
	}
	defer fs.Close()

	ctx := context.Background()
	doc, err := fs.AppendInstructions(ctx, store.InstructionID{
		AccountPrefix: "u_123",
		Bucket:        "default",
		Path:          "rules",
		FileName:      "instructions.txt",
		SessionID:     "session-a",
	}, store.AppendInstructionParams{
		Rules: []store.Rule{{Command: "keep this rule", Seqno: 0}},
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("merged rules: %v", doc.Rules)
}
```

### Reading merged content

```go
doc, err := fs.GetInstructions(ctx, store.InstructionID{
	AccountPrefix: "u_123",
	Bucket:        "default",
	Path:          "rules",
	FileName:      "instructions.txt",
})
if err != nil {
	log.Fatal(err)
}

for _, rule := range doc.Rules {
	log.Println(rule.Command)
}
```

### Partial reads

```go
partial, err := fs.GetInstructionsPartial(ctx, store.InstructionID{
	AccountPrefix: "u_123",
	Bucket:        "default",
	Path:          "rules",
	FileName:      "instructions.txt",
}, store.GetPartialInstructionParams{
	Range: store.Range{Start: 0, End: 10},
})
if err != nil {
	log.Fatal(err)
}
```

### Folder index

```go
entries, err := fs.GetFolderIndex(ctx, store.InstructionID{
	AccountPrefix: "u_123",
	Bucket:        "default",
	Path:          "rules",
})
if err != nil {
	log.Fatal(err)
}

for _, entry := range entries {
	log.Printf("%s %s %d", entry.Name, entry.Format, entry.Size)
}
```

### Subscriptions

```go
sub, err := fs.SubscribeInstructions(ctx, store.InstructionID{
	AccountPrefix: "u_123",
	Bucket:        "default",
	Path:          "rules",
	FileName:      "instructions.txt",
	SessionID:     "session-a",
}, store.SubscriptionParams{IgnoreSelf: true})
if err != nil {
	log.Fatal(err)
}
defer sub.Close()

go func() {
	for evt := range sub.Events {
		log.Printf("file %s updated: %v", evt.FileID, evt.ModifiedSeqNos)
	}
}()
```

## HTTP Usage

The package also exposes an `http.Handler`:

```go
handler := store.NewHTTPHandler(fs)
```

Supported routes:

- `GET /v1/instructions`
- `GET /v1/instructions/meta`
- `GET /v1/instructions/partial`
- `POST /v1/instructions/append`
- `POST /v1/instructions/update`
- `POST /v1/instructions/delete`
- `POST /v1/instructions/discard`
- `GET /v1/folders/index`
- `GET /v1/subscriptions/stream`

Example stream request:

```bash
curl -N \
  "http://localhost:8080/v1/subscriptions/stream?account_prefix=u_123&bucket=default&path=rules&file_name=instructions.txt&session_id=session-a&ignore_self=true"
```

## Notes

- The store writes temp branch files next to the main file using the documented temp format.
- Main reads return the merged view of the main file plus any branch files for the same target.
- The implementation is intentionally decoupled from any specific merger implementation. Services only need to satisfy the cache and merger interfaces shown above.
