package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	applog "github.com/Shaik-Sirajuddin/memory/mcp/log"
)

type Server struct {
	manager      *ConnectionManager
	interval     time.Duration
	deliveryMode DeliveryMode
	requests     atomic.Uint64
}

func New(interval time.Duration) *Server {
	return NewWithDelivery(interval, DeliveryModeFromEnv())
}

func NewWithDelivery(interval time.Duration, deliveryMode DeliveryMode) *Server {
	if interval <= 0 {
		interval = time.Minute
	}
	if deliveryMode == "" {
		deliveryMode = DeliveryNotification
	}
	s := &Server{
		manager:      NewConnectionManager(),
		interval:     interval,
		deliveryMode: deliveryMode,
	}
	go s.runPromptLoop()
	return s
}

func DeliveryModeFromEnv() DeliveryMode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("MCP_DELIVERY_MODE"))) {
	case "", string(DeliveryNotification), "event", "notifications/event":
		return DeliveryNotification
	case string(DeliverySampling):
		return DeliverySampling
	default:
		applog.Logger.Warn("unknown MCP_DELIVERY_MODE, defaulting to notification", "value", os.Getenv("MCP_DELIVERY_MODE"))
		return DeliveryNotification
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", s.handleMCP)
	mux.HandleFunc("/admin/connections", s.handleConnections)
	mux.HandleFunc("/admin/run_inference", s.handleRunInference)
	return requestLogger(mux)
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleStream(w, r)
	case http.MethodPost:
		s.handleRPC(w, r)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	conn := s.manager.Add(r.RemoteAddr, r.UserAgent())
	defer s.manager.Remove(conn.ID)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Mcp-Session-Id", conn.ID)

	applog.Logger.Info("mcp stream opened", "connection_id", conn.ID, "remote_addr", r.RemoteAddr)
	writeSSE(w, "connection", map[string]string{"id": conn.ID})
	flusher.Flush()

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			applog.Logger.Info("mcp stream closed", "connection_id", conn.ID)
			return
		case <-heartbeat.C:
			if _, err := w.Write([]byte(": heartbeat\n\n")); err != nil {
				applog.Logger.Error("mcp heartbeat failed", "err", err, "connection_id", conn.ID)
				return
			}
			flusher.Flush()
		case msg := <-conn.Out:
			applog.Logger.Debug("mcp stream sending message", "connection_id", conn.ID, "request_id", msg.ID, "method", msg.Method)
			writeSSE(w, "message", msg)
			flusher.Flush()
		}
	}
}

func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req RPCMessage
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		applog.Logger.Error("mcp rpc decode failed", "err", err)
		writeJSONError(w, http.StatusBadRequest, "invalid JSON-RPC payload")
		return
	}

	applog.Logger.Debug("mcp rpc received", "id", req.ID, "method", req.Method)
	if req.Method == "" {
		s.handleRPCResponse(w, r, req)
		return
	}

	switch req.Method {
	case "initialize":
		s.writeInitialize(w, req)
	case "notifications/initialized":
		w.WriteHeader(http.StatusAccepted)
	case "ping":
		writeJSON(w, http.StatusOK, RPCMessage{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}})
	case "tools/list":
		writeJSON(w, http.StatusOK, toolsListResponse(req.ID))
	case "resources/list":
		writeJSON(w, http.StatusOK, resourcesListResponse(req.ID))
	case "resources/templates/list":
		writeJSON(w, http.StatusOK, resourceTemplatesListResponse(req.ID))
	case "prompts/list":
		writeJSON(w, http.StatusOK, promptsListResponse(req.ID))
	default:
		writeJSON(w, http.StatusOK, RPCMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    -32601,
				Message: "method not found",
			},
		})
	}
}

func (s *Server) writeInitialize(w http.ResponseWriter, req RPCMessage) {
	resp := initializeResponse(req.ID)
	applog.Logger.Debug("mcp initialize response", "response", resp.Result)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRPCResponse(w http.ResponseWriter, r *http.Request, req RPCMessage) {
	if req.ID == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	connectionID := r.Header.Get("Mcp-Session-Id")
	applog.Logger.Info("mcp inference response received", "connection_id", connectionID, "request_id", req.ID, "result", req.Result, "error", req.Error)
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	connections := s.manager.List()
	applog.Logger.Debug("admin list connections", "count", len(connections))
	writeJSON(w, http.StatusOK, map[string]any{"connections": connections})
}

func (s *Server) handleRunInference(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	defer r.Body.Close()

	var req RunInferenceRequest
	if r.Body != http.NoBody {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			applog.Logger.Error("admin run_inference decode failed", "err", err)
			writeJSONError(w, http.StatusBadRequest, "invalid run_inference payload")
			return
		}
	}
	if req.Prompt == "" {
		req.Prompt = DefaultInferencePrompt
	}

	sent, err := s.RunInference(r.Context(), req.ConnectionID, req.Prompt)
	if err != nil {
		applog.Logger.Error("admin run_inference failed", "err", err, "connection_id", req.ConnectionID)
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{"sent": sent})
}

func (s *Server) RunInference(ctx context.Context, connectionID string, prompt string) ([]string, error) {
	if strings.TrimSpace(prompt) == "" {
		prompt = DefaultInferencePrompt
	}

	targets := s.manager.Targets(connectionID)
	if len(targets) == 0 {
		if connectionID != "" {
			return nil, fmt.Errorf("connection %q not found", connectionID)
		}
		return nil, errors.New("no open connections")
	}

	sent := make([]string, 0, len(targets))
	for _, conn := range targets {
		id := fmt.Sprintf("delivery-%d", s.requests.Add(1))
		msg := s.deliveryMessage(id, prompt)
		applog.Logger.Debug("sending inference delivery", "connection_id", conn.ID, "request_id", id, "prompt", prompt, "delivery_mode", s.deliveryMode, "method", msg.Method)
		select {
		case <-ctx.Done():
			applog.Logger.Error("sending inference delivery canceled", "err", ctx.Err(), "connection_id", conn.ID, "request_id", id, "delivery_mode", s.deliveryMode)
			return sent, ctx.Err()
		case conn.Out <- msg:
			conn.MarkSent()
			sent = append(sent, conn.ID)
			applog.Logger.Info("inference delivery queued", "connection_id", conn.ID, "request_id", id, "delivery_mode", s.deliveryMode, "method", msg.Method)
		default:
			applog.Logger.Warn("connection send queue full", "connection_id", conn.ID, "request_id", id, "delivery_mode", s.deliveryMode)
		}
	}
	return sent, nil
}

func (s *Server) runPromptLoop() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := s.RunInference(ctx, "", DefaultInferencePrompt)
		cancel()
		if err != nil {
			applog.Logger.Warn("scheduled inference prompt skipped", "err", err)
		}
	}
}

func (s *Server) deliveryMessage(id string, prompt string) RPCMessage {
	return deliveryMessage(s.deliveryMode, id, prompt)
}

func deliveryMessage(deliveryMode DeliveryMode, id string, prompt string) RPCMessage {
	if deliveryMode == DeliveryNotification {
		return notificationEvent(prompt)
	}
	return samplingRequest(id, prompt)
}

func initializeResponse(id any) RPCMessage {
	return RPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"protocolVersion": "2025-03-26",
			"serverInfo": map[string]string{
				"name":    "memory-tree-minimal-mcp",
				"version": "0.1.0",
			},
			"capabilities": map[string]any{
				"tools":     map[string]any{"listChanged": false},
				"resources": map[string]any{},
			},
		},
	}
}

func toolsListResponse(id any) RPCMessage {
	return RPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"tools": []any{},
		},
	}
}

func resourcesListResponse(id any) RPCMessage {
	return RPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"resources": []any{},
		},
	}
}

func resourceTemplatesListResponse(id any) RPCMessage {
	return RPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"resourceTemplates": []any{},
		},
	}
}

func promptsListResponse(id any) RPCMessage {
	return RPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"prompts": []any{},
		},
	}
}

func samplingRequest(id string, prompt string) RPCMessage {
	return RPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "sampling/createMessage",
		Params: map[string]any{
			"messages": []map[string]any{
				{
					"role": "user",
					"content": map[string]string{
						"type": "text",
						"text": prompt,
					},
				},
			},
			"maxTokens": 128,
		},
	}
}

func notificationEvent(prompt string) RPCMessage {
	return RPCMessage{
		JSONRPC: "2.0",
		Method:  "notifications/event",
		Params: map[string]any{
			"type":   "run_inference",
			"prompt": prompt,
		},
	}
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		applog.Logger.Info("request received", "method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

func methodNotAllowed(w http.ResponseWriter, allowed ...string) {
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func writeSSE(w http.ResponseWriter, event string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		applog.Logger.Error("sse marshal failed", "err", err, "event", event)
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\n", event)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		applog.Logger.Error("json response encode failed", "err", err, "status", status)
	}
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
