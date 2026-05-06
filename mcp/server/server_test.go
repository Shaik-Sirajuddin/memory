//go:build unit

package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestInitialize(t *testing.T) {
	srv := New(time.Hour)
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp RPCMessage
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != float64(1) {
		t.Fatalf("id = %#v, want 1", resp.ID)
	}
	if resp.Result == nil {
		t.Fatal("result is nil")
	}
}

func TestDiscoveryMethods(t *testing.T) {
	cases := []struct {
		name     string
		method   string
		expected string
	}{
		{name: "tools list", method: "tools/list", expected: `"tools":[]`},
		{name: "resources list", method: "resources/list", expected: `"resources":[]`},
		{name: "resource templates list", method: "resources/templates/list", expected: `"resourceTemplates":[]`},
		{name: "prompts list", method: "prompts/list", expected: `"prompts":[]`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := New(time.Hour)
			req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"`+tc.method+`"}`))
			rec := httptest.NewRecorder()

			srv.Routes().ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("discovery status should be OK: got %d", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), tc.expected) {
				t.Fatalf("discovery response should contain %s: %s", tc.expected, rec.Body.String())
			}
		})
	}
}

func TestAdminRunInferenceQueuesSamplingRequest(t *testing.T) {
	t.Run("default delivery", func(t *testing.T) {
		srv := NewWithDelivery(time.Hour, "")
		conn := srv.manager.Add("127.0.0.1:1234", "test")
		defer srv.manager.Remove(conn.ID)

		body := bytes.NewBufferString(`{"connection_id":"` + conn.ID + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/admin/run_inference", body)
		rec := httptest.NewRecorder()

		srv.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("status should be accepted: got %d", rec.Code)
		}

		select {
		case msg := <-conn.Out:
			if msg.Method != "notifications/event" {
				t.Fatalf("method should default to notification delivery: got %q", msg.Method)
			}
			if !strings.Contains(mustJSON(t, msg.Params), "cat docs/setup.md") {
				t.Fatalf("default prompt should contain cat command: %#v", msg.Params)
			}
		case <-time.After(time.Second):
			t.Fatal("default delivery should be queued")
		}
	})

	t.Run("sampling delivery", func(t *testing.T) {
		srv := NewWithDelivery(time.Hour, DeliverySampling)
		conn := srv.manager.Add("127.0.0.1:1234", "test")
		defer srv.manager.Remove(conn.ID)

		body := bytes.NewBufferString(`{"connection_id":"` + conn.ID + `","prompt":"hello"}`)
		req := httptest.NewRequest(http.MethodPost, "/admin/run_inference", body)
		rec := httptest.NewRecorder()

		srv.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("status should be accepted: got %d", rec.Code)
		}

		select {
		case msg := <-conn.Out:
			if msg.Method != "sampling/createMessage" {
				t.Fatalf("method should use sampling delivery: got %q", msg.Method)
			}
		case <-time.After(time.Second):
			t.Fatal("sampling request should be queued")
		}
	})

	t.Run("notification delivery", func(t *testing.T) {
		srv := NewWithDelivery(time.Hour, DeliveryNotification)
		conn := srv.manager.Add("127.0.0.1:1234", "test")
		defer srv.manager.Remove(conn.ID)

		body := bytes.NewBufferString(`{"connection_id":"` + conn.ID + `","prompt":"hello"}`)
		req := httptest.NewRequest(http.MethodPost, "/admin/run_inference", body)
		rec := httptest.NewRecorder()

		srv.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("status should be accepted: got %d", rec.Code)
		}

		select {
		case msg := <-conn.Out:
			if msg.Method != "notifications/event" {
				t.Fatalf("method should use notification delivery: got %q", msg.Method)
			}
			if !strings.Contains(mustJSON(t, msg.Params), "hello") {
				t.Fatalf("provided prompt should be preserved: %#v", msg.Params)
			}
			if msg.ID != nil {
				t.Fatalf("notification id should be omitted: got %#v", msg.ID)
			}
		case <-time.After(time.Second):
			t.Fatal("notification event should be queued")
		}
	})
}

func TestConnectionsEndpointListsOpenConnections(t *testing.T) {
	srv := New(time.Hour)
	conn := srv.manager.Add("127.0.0.1:1234", "test")
	defer srv.manager.Remove(conn.ID)

	req := httptest.NewRequest(http.MethodGet, "/admin/connections", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), conn.ID) {
		t.Fatalf("response %q does not contain connection id %q", rec.Body.String(), conn.ID)
	}
}

func TestSSEWritesQueuedMessages(t *testing.T) {
	srv := NewWithDelivery(time.Hour, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil).WithContext(ctx)
	rec := newStreamingRecorder()
	done := make(chan struct{})

	go func() {
		defer close(done)
		srv.Routes().ServeHTTP(rec, req)
	}()

	connectionID := waitForConnectionID(t, srv)
	if _, err := srv.RunInference(context.Background(), connectionID, "hello"); err != nil {
		t.Fatalf("run inference: %v", err)
	}

	rec.waitForText(t, "notifications/event")
	cancel()
	<-done
}

func TestStdioTransport(t *testing.T) {
	t.Run("initialize response", func(t *testing.T) {
		srv := NewStdioWithDelivery(time.Hour, DeliverySampling)
		in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n")
		var out bytes.Buffer

		if err := srv.Serve(context.Background(), in, &out); err != nil {
			t.Fatalf("stdio serve should read initialize without error: %v", err)
		}

		if !strings.Contains(out.String(), `"protocolVersion":"2025-03-26"`) {
			t.Fatalf("stdio initialize response should contain protocol version: %s", out.String())
		}
	})

	t.Run("discovery responses", func(t *testing.T) {
		srv := NewStdioWithDelivery(time.Hour, DeliverySampling)
		in := strings.NewReader(
			`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n" +
				`{"jsonrpc":"2.0","id":3,"method":"resources/list"}` + "\n" +
				`{"jsonrpc":"2.0","id":4,"method":"resources/templates/list"}` + "\n" +
				`{"jsonrpc":"2.0","id":5,"method":"prompts/list"}` + "\n",
		)
		var out bytes.Buffer

		if err := srv.Serve(context.Background(), in, &out); err != nil {
			t.Fatalf("stdio serve should read discovery requests without error: %v", err)
		}

		if !strings.Contains(out.String(), `"tools":[]`) {
			t.Fatalf("stdio tools list response should contain empty tools: %s", out.String())
		}
		if !strings.Contains(out.String(), `"resources":[]`) {
			t.Fatalf("stdio resources list response should contain empty resources: %s", out.String())
		}
		if !strings.Contains(out.String(), `"resourceTemplates":[]`) {
			t.Fatalf("stdio resource templates list response should contain empty resource templates: %s", out.String())
		}
		if !strings.Contains(out.String(), `"prompts":[]`) {
			t.Fatalf("stdio prompts list response should contain empty prompts: %s", out.String())
		}
	})

	t.Run("scheduled notification delivery", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		srv := NewStdioWithDelivery(time.Millisecond, DeliveryNotification)
		reader, writer := io.Pipe()
		defer writer.Close()
		var out bytes.Buffer
		done := make(chan error, 1)

		go func() {
			done <- srv.Serve(ctx, reader, &out)
		}()

		deadline := time.After(time.Second)
		for {
			if strings.Contains(out.String(), `"method":"notifications/event"`) && strings.Contains(out.String(), "cat docs/setup.md") {
				cancel()
				<-done
				return
			}
			select {
			case <-deadline:
				t.Fatalf("stdio scheduled delivery should emit notification event: %s", out.String())
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}
	})

	t.Run("response payload logging path", func(t *testing.T) {
		srv := NewStdioWithDelivery(time.Hour, DeliveryNotification)
		req := RPCMessage{
			JSONRPC: "2.0",
			ID:      "stdio-delivery-1",
			Result:  map[string]any{"content": "cat output"},
		}

		resp, ok := srv.handleRPC(req)

		if ok {
			t.Fatalf("stdio response should not emit protocol response: %#v", resp)
		}
	})
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("value should marshal to JSON: %v", err)
	}
	return string(data)
}

func waitForConnectionID(t *testing.T, srv *Server) string {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for stream connection")
		default:
			connections := srv.manager.List()
			if len(connections) > 0 {
				return connections[0].ID
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

type streamingRecorder struct {
	header http.Header
	body   bytes.Buffer
	mu     sync.Mutex
}

func newStreamingRecorder() *streamingRecorder {
	return &streamingRecorder{header: make(http.Header)}
}

func (r *streamingRecorder) Header() http.Header {
	return r.header
}

func (r *streamingRecorder) WriteHeader(statusCode int) {}

func (r *streamingRecorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.Write(p)
}

func (r *streamingRecorder) Flush() {}

func (r *streamingRecorder) waitForText(t *testing.T, text string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			r.mu.Lock()
			body := r.body.String()
			r.mu.Unlock()
			scanner := bufio.NewScanner(strings.NewReader(body))
			lines := make([]string, 0)
			for scanner.Scan() {
				lines = append(lines, scanner.Text())
			}
			t.Fatalf("timed out waiting for %q in stream body lines %#v", text, lines)
		default:
			r.mu.Lock()
			contains := strings.Contains(r.body.String(), text)
			r.mu.Unlock()
			if contains {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}
