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
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal hook callback request: %w", err)
	}
	resp, err := newHookUnixHTTPClient(socketPath).Post("http://localhost/hook-callback", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("connect hook-operator at %s: %w", socketPath, err)
	}
	defer resp.Body.Close()
	if err := requireHookHTTPSuccess(resp, "hook callback"); err != nil {
		return nil, err
	}
	var result HookCallbackResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode hook callback response: %w", err)
	}
	return &result, nil
}

// GetHookProviderStatuses fetches hook-provider registration health from
// hook-operator over its Unix-domain HTTP socket.
func GetHookProviderStatuses(socketPath string) ([]ProviderHookStatus, error) {
	resp, err := newHookUnixHTTPClient(socketPath).Get("http://localhost/agents/status")
	if err != nil {
		return nil, fmt.Errorf("connect hook-operator at %s: %w", socketPath, err)
	}
	defer resp.Body.Close()
	if err := requireHookHTTPSuccess(resp, "hook status"); err != nil {
		return nil, err
	}
	var statuses []ProviderHookStatus
	if err := json.NewDecoder(resp.Body).Decode(&statuses); err != nil {
		return nil, fmt.Errorf("decode hook status response: %w", err)
	}
	return statuses, nil
}

func requireHookHTTPSuccess(resp *http.Response, label string) error {
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = resp.Status
	}
	return fmt.Errorf("%s: %s", label, message)
}

func newHookUnixHTTPClient(socketPath string) *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	return &http.Client{Transport: transport, Timeout: hookOperatorHTTPTimeout}
}
