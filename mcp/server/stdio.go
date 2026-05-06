package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"sync/atomic"
	"time"

	applog "github.com/Shaik-Sirajuddin/memory/mcp/log"
)

type StdioServer struct {
	interval     time.Duration
	deliveryMode DeliveryMode
	requests     atomic.Uint64
}

func NewStdio(interval time.Duration) *StdioServer {
	return NewStdioWithDelivery(interval, DeliveryModeFromEnv())
}

func NewStdioWithDelivery(interval time.Duration, deliveryMode DeliveryMode) *StdioServer {
	if interval <= 0 {
		interval = time.Minute
	}
	if deliveryMode == "" {
		deliveryMode = DeliveryNotification
	}
	return &StdioServer{
		interval:     interval,
		deliveryMode: deliveryMode,
	}
}

func (s *StdioServer) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	enc := json.NewEncoder(out)
	send := make(chan RPCMessage, 16)

	go s.runPromptLoop(ctx, send)

	done := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(in)
		for scanner.Scan() {
			var req RPCMessage
			if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
				applog.Logger.Error("stdio rpc decode failed", "err", err)
				send <- RPCMessage{
					JSONRPC: "2.0",
					Error: &RPCError{
						Code:    -32700,
						Message: "parse error",
					},
				}
				continue
			}
			resp, ok := s.handleRPC(req)
			if ok {
				send <- resp
			}
		}
		done <- scanner.Err()
	}()

	for {
		select {
		case <-ctx.Done():
			applog.Logger.Info("stdio server stopped")
			return ctx.Err()
		case err := <-done:
			if err != nil {
				applog.Logger.Error("stdio scanner failed", "err", err)
			}
			for {
				select {
				case msg := <-send:
					applog.Logger.Debug("stdio draining message", "method", msg.Method, "id", msg.ID)
					if encodeErr := enc.Encode(msg); encodeErr != nil {
						applog.Logger.Error("stdio encode failed", "err", encodeErr, "method", msg.Method)
						return encodeErr
					}
				default:
					return err
				}
			}
		case msg := <-send:
			applog.Logger.Debug("stdio sending message", "method", msg.Method, "id", msg.ID, "delivery_mode", s.deliveryMode)
			if err := enc.Encode(msg); err != nil {
				applog.Logger.Error("stdio encode failed", "err", err, "method", msg.Method)
				return err
			}
		}
	}
}

func (s *StdioServer) handleRPC(req RPCMessage) (RPCMessage, bool) {
	applog.Logger.Debug("stdio rpc received", "id", req.ID, "method", req.Method)
	if req.Method == "" {
		applog.Logger.Info("stdio response received", "request_id", req.ID, "result", req.Result, "error", req.Error)
		return RPCMessage{}, false
	}

	switch req.Method {
	case "initialize":
		return initializeResponse(req.ID), true
	case "notifications/initialized":
		return RPCMessage{}, false
	case "ping":
		return RPCMessage{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}, true
	case "tools/list":
		return toolsListResponse(req.ID), true
	case "resources/list":
		return resourcesListResponse(req.ID), true
	case "resources/templates/list":
		return resourceTemplatesListResponse(req.ID), true
	case "prompts/list":
		return promptsListResponse(req.ID), true
	default:
		return RPCMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    -32601,
				Message: "method not found",
			},
		}, true
	}
}

func (s *StdioServer) runPromptLoop(ctx context.Context, send chan<- RPCMessage) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			id := s.nextID()
			msg := deliveryMessage(s.deliveryMode, id, DefaultInferencePrompt)
			applog.Logger.Info("stdio inference delivery queued", "request_id", id, "delivery_mode", s.deliveryMode, "method", msg.Method, "prompt", DefaultInferencePrompt)
			send <- msg
		}
	}
}

func (s *StdioServer) nextID() string {
	return "stdio-delivery-" + strconv.FormatUint(s.requests.Add(1), 10)
}
