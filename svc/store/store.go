package store

import (
	"context"
	"time"
)

// Range selects a half-open instruction window.
type Range struct {
	Start int
	End   int
}

// Rule is a normalized instruction line.
type Rule struct {
	Command string
	Seqno   int
}

// InstructionID is the normalized identifier used by the store layer.
// Callers should resolve headers and path components before invoking the store.
type InstructionID struct {
	AccountPrefix string
	Bucket        string
	Path          string
	FileName      string
	SessionID     string
}

// GetPartialInstructionParams selects a subset of merged instructions.
type GetPartialInstructionParams struct {
	Range
}

// AppendInstructionParams appends normalized rules.
type AppendInstructionParams struct {
	Rules []Rule
}

// UpdateInstructionParams replaces existing rules.
type UpdateInstructionParams struct {
	Rules []Rule
}

// DeleteInstructionParams removes instructions from global and caller memory.
type DeleteInstructionParams struct {
	SeqNo []int
}

// DiscardInstructionParams removes instructions only from caller-specific memory.
type DiscardInstructionParams struct {
	SeqNo []int
}

// SubscriptionParams controls how a subscriber is registered.
type SubscriptionParams struct {
	IgnoreSelf bool
}

// InstructionDocument is the merged instruction payload returned by reads.
type InstructionDocument struct {
	ID        InstructionID
	Raw       string
	Rules     []Rule
	UpdatedAt time.Time
}

// InstructionMeta captures filesystem metadata for a target instruction file.
type InstructionMeta struct {
	ID        InstructionID
	Path      string
	Size      int64
	Mode      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// FolderEntry is returned by folder index queries.
type FolderEntry struct {
	Name      string
	Format    string
	Size      int64
	UpdatedAt time.Time
	CreatedAt time.Time
	IsDir     bool
	Path      string
}

// StoreEvent is emitted to subscribers when a file changes.
type StoreEvent struct {
	FileID          string
	ModifiedSeqNos  []int
	Operation       string
	OriginSessionID string
}

// Subscription is a concrete subscription handle.
type Subscription struct {
	ID     string
	Events <-chan StoreEvent
	Close  func() error
}

// Store defines the public store contract.
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
