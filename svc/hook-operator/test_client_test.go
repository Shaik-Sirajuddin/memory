package hookoperator

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type hookTestClient struct {
	t *testing.T

	mu        sync.Mutex
	exchanges map[string]hookTestExchange

	server   *httptest.Server
	unixPath string
	unixSrv  *http.Server
	unixLn   net.Listener
}

type hookTestExchange struct {
	Request  hookTestRequest
	Response hookResponse
}

type hookTestRequest struct {
	Method    string
	Path      string
	EventName string
	Body      []byte
	Payload   map[string]any
}

func newHookTestClient(t *testing.T) *hookTestClient {
	t.Helper()

	c := &hookTestClient{
		t:         t,
		exchanges: map[string]hookTestExchange{},
	}
	c.server = httptest.NewServer(c.handler())
	t.Cleanup(c.Close)

	return c
}

func newUnixHookTestClient(t *testing.T) *hookTestClient {
	t.Helper()

	c := &hookTestClient{
		t:         t,
		exchanges: map[string]hookTestExchange{},
	}

	socketPath := filepath.Join(t.TempDir(), "hook-client.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix hook test client: %v", err)
	}

	c.unixPath = socketPath
	c.unixLn = ln
	c.unixSrv = &http.Server{Handler: c.handler()}

	go func() {
		if err := c.unixSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			t.Errorf("serve unix hook test client: %v", err)
		}
	}()

	t.Cleanup(c.Close)
	return c
}

func (c *hookTestClient) URL(name string) string {
	c.t.Helper()

	if c.server == nil {
		c.t.Fatalf("http hook test client is not configured")
	}
	return c.server.URL + "/" + name
}

func (c *hookTestClient) UnixURL(name string) string {
	c.t.Helper()

	if c.unixPath == "" {
		c.t.Fatalf("unix hook test client is not configured")
	}
	return "unix://" + c.unixPath + "/" + name
}

func (c *hookTestClient) SetResponse(name string, resp hookResponse) {
	c.t.Helper()

	c.mu.Lock()
	defer c.mu.Unlock()

	ex := c.exchanges[name]
	ex.Response = resp
	c.exchanges[name] = ex
}

func (c *hookTestClient) Exchange(name string) (hookTestExchange, bool) {
	c.t.Helper()

	c.mu.Lock()
	defer c.mu.Unlock()

	ex, ok := c.exchanges[name]
	if !ok {
		return hookTestExchange{}, false
	}

	body := append([]byte(nil), ex.Request.Body...)
	ex.Request.Body = body
	return ex, true
}

func (c *hookTestClient) Close() {
	if c.server != nil {
		c.server.Close()
		c.server = nil
	}
	if c.unixSrv != nil {
		_ = c.unixSrv.Shutdown(context.Background())
		c.unixSrv = nil
	}
	if c.unixLn != nil {
		_ = c.unixLn.Close()
		c.unixLn = nil
	}
	if c.unixPath != "" {
		_ = os.Remove(c.unixPath)
		c.unixPath = ""
	}
}

func (c *hookTestClient) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "default"
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var payload map[string]any
		if len(body) > 0 {
			_ = json.Unmarshal(body, &payload)
		}

		c.mu.Lock()
		ex := c.exchanges[name]
		ex.Request = hookTestRequest{
			Method:    r.Method,
			Path:      r.URL.Path,
			EventName: r.Header.Get("X-Hook-Event"),
			Body:      append([]byte(nil), body...),
			Payload:   payload,
		}
		resp := ex.Response
		c.exchanges[name] = ex
		c.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			c.t.Errorf("encode hook test response: %v", err)
		}
	})
	return mux
}
