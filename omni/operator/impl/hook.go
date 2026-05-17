package impl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const hookOperatorHTTPTimeout = 30 * time.Second

// HookCallbackRequest is the payload sent to the hook-operator callback
// endpoint for a codeagent hook event.
type HookCallbackRequest struct {
	EventName string `json:"event_name"`
	Body      []byte `json:"body"`
}

// HookCallbackResult is the hook response returned by hook-operator.
type HookCallbackResult struct {
	Continue       bool    `json:"continue"`
	SuppressOutput bool    `json:"suppress_output"`
	StopReason     *string `json:"stop_reason,omitempty"`
	SystemMessage  *string `json:"system_message,omitempty"`
}

// ProviderHookStatus describes hook registration health for one provider.
type ProviderHookStatus struct {
	Provider string   `json:"provider"`
	OK       bool     `json:"ok"`
	Missing  []string `json:"missing"`
}

// PostHookCallback forwards a hook callback request to hook-operator over its
// Unix-domain HTTP socket and decodes the hook response.
func PostHookCallback(socketPath string, payload HookCallbackRequest) (*HookCallbackResult, error) {
	logger.Debug("PostHookCallback: preparing request", "socket", socketPath, "event", payload.EventName, "bodyBytes", len(payload.Body))
	body, err := json.Marshal(payload)
	if err != nil {
		logger.Error("PostHookCallback: marshal request failed", "err", err, "socket", socketPath, "event", payload.EventName, "bodyBytes", len(payload.Body))
		return nil, fmt.Errorf("marshal hook callback request: %w", err)
	}
	logger.Debug("PostHookCallback: sending request", "socket", socketPath, "event", payload.EventName, "payloadBytes", len(body), "timeout", hookOperatorHTTPTimeout)
	resp, err := newHookUnixHTTPClient(socketPath).Post("http://localhost/hook-callback", "application/json", bytes.NewReader(body))
	if err != nil {
		logger.Error("PostHookCallback: request failed", "err", err, "socket", socketPath, "event", payload.EventName)
		return nil, fmt.Errorf("connect hook-operator at %s: %w", socketPath, err)
	}
	defer resp.Body.Close()
	logger.Debug("PostHookCallback: response received", "socket", socketPath, "event", payload.EventName, "status", resp.StatusCode)
	if err := requireHookHTTPSuccess(resp, "hook callback"); err != nil {
		logger.Error("PostHookCallback: non-success response", "err", err, "socket", socketPath, "event", payload.EventName, "status", resp.StatusCode)
		return nil, err
	}
	var result HookCallbackResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		logger.Error("PostHookCallback: decode response failed", "err", err, "socket", socketPath, "event", payload.EventName, "status", resp.StatusCode)
		return nil, fmt.Errorf("decode hook callback response: %w", err)
	}
	logger.Debug("PostHookCallback: completed", "socket", socketPath, "event", payload.EventName, "continue", result.Continue, "suppressOutput", result.SuppressOutput, "hasStopReason", result.StopReason != nil, "hasSystemMessage", result.SystemMessage != nil)
	return &result, nil
}

// GetHookProviderStatuses fetches hook-provider registration health from
// hook-operator over its Unix-domain HTTP socket.
func GetHookProviderStatuses(socketPath string) ([]ProviderHookStatus, error) {
	logger.Debug("GetHookProviderStatuses: sending request", "socket", socketPath, "timeout", hookOperatorHTTPTimeout)
	resp, err := newHookUnixHTTPClient(socketPath).Get("http://localhost/agents/status")
	if err != nil {
		logger.Error("GetHookProviderStatuses: request failed", "err", err, "socket", socketPath)
		return nil, fmt.Errorf("connect hook-operator at %s: %w", socketPath, err)
	}
	defer resp.Body.Close()
	logger.Debug("GetHookProviderStatuses: response received", "socket", socketPath, "status", resp.StatusCode)
	if err := requireHookHTTPSuccess(resp, "hook status"); err != nil {
		logger.Error("GetHookProviderStatuses: non-success response", "err", err, "socket", socketPath, "status", resp.StatusCode)
		return nil, err
	}
	var statuses []ProviderHookStatus
	if err := json.NewDecoder(resp.Body).Decode(&statuses); err != nil {
		logger.Error("GetHookProviderStatuses: decode response failed", "err", err, "socket", socketPath, "status", resp.StatusCode)
		return nil, fmt.Errorf("decode hook status response: %w", err)
	}
	missingCount := 0
	for _, status := range statuses {
		missingCount += len(status.Missing)
	}
	logger.Debug("GetHookProviderStatuses: completed", "socket", socketPath, "providers", len(statuses), "missing", missingCount)
	return statuses, nil
}

func requireHookHTTPSuccess(resp *http.Response, label string) error {
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		logger.Debug("requireHookHTTPSuccess: response accepted", "label", label, "status", resp.StatusCode)
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = resp.Status
	}
	logger.Debug("requireHookHTTPSuccess: response rejected", "label", label, "status", resp.StatusCode, "message", message)
	return fmt.Errorf("%s: %s", label, message)
}

func newHookUnixHTTPClient(socketPath string) *http.Client {
	logger.Debug("newHookUnixHTTPClient: configuring client", "socket", socketPath, "timeout", hookOperatorHTTPTimeout)
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	return &http.Client{Transport: transport, Timeout: hookOperatorHTTPTimeout}
}
