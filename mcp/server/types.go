package server

import (
	"sync"
	"time"
)

type RPCMessage struct {
	JSONRPC string    `json:"jsonrpc,omitempty"`
	ID      any       `json:"id,omitempty"`
	Method  string    `json:"method,omitempty"`
	Params  any       `json:"params,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type RunInferenceRequest struct {
	ConnectionID string `json:"connection_id"`
	Prompt       string `json:"prompt"`
}

type DeliveryMode string

const (
	DeliverySampling     DeliveryMode = "sampling"
	DeliveryNotification DeliveryMode = "notification"
)

const DefaultInferencePrompt = "Run this example command and return the output: cat docs/setup.md"

type Connection struct {
	ID         string
	RemoteAddr string
	UserAgent  string
	OpenedAt   time.Time
	LastSentAt time.Time
	Out        chan RPCMessage
	mu         sync.RWMutex
}

func (c *Connection) Snapshot() ConnectionSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ConnectionSnapshot{
		ID:         c.ID,
		RemoteAddr: c.RemoteAddr,
		UserAgent:  c.UserAgent,
		OpenedAt:   c.OpenedAt,
		LastSentAt: c.LastSentAt,
	}
}

func (c *Connection) MarkSent() {
	c.mu.Lock()
	c.LastSentAt = time.Now().UTC()
	c.mu.Unlock()
}

type ConnectionSnapshot struct {
	ID         string    `json:"id"`
	RemoteAddr string    `json:"remote_addr"`
	UserAgent  string    `json:"user_agent"`
	OpenedAt   time.Time `json:"opened_at"`
	LastSentAt time.Time `json:"last_sent_at,omitempty"`
}
