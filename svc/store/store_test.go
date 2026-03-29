package store

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFileSystemAppendReadPartialDiscardAndSubscription(t *testing.T) {
	ctx := context.Background()
	fs, err := NewFileSystem(t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("new filesystem: %v", err)
	}
	defer fs.Close()

	id := InstructionID{
		AccountPrefix: "u_1",
		Bucket:        "default",
		Path:          "rules",
		FileName:      "instructions.txt",
		SessionID:     "session-a",
	}

	sub, err := fs.SubscribeInstructions(ctx, id, SubscriptionParams{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Close()

	doc, err := fs.AppendInstructions(ctx, id, AppendInstructionParams{
		Rules: []Rule{
			{Command: "alpha", Seqno: 0},
			{Command: "beta", Seqno: 1},
		},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if got := len(doc.Rules); got != 2 {
		t.Fatalf("append document rules = %d, want 2", got)
	}
	if !strings.Contains(doc.Raw, "- alpha") {
		t.Fatalf("append raw output missing first rule: %q", doc.Raw)
	}

	select {
	case evt := <-sub.Events:
		if evt.FileID == "" {
			t.Fatalf("expected file id in event")
		}
		if len(evt.ModifiedSeqNos) == 0 {
			t.Fatalf("expected modified seqnos in event")
		}
	default:
		t.Fatalf("expected subscription event")
	}

	partial, err := fs.GetInstructionsPartial(ctx, id, GetPartialInstructionParams{Range: Range{Start: 1, End: 2}})
	if err != nil {
		t.Fatalf("partial read: %v", err)
	}
	if got := len(partial.Rules); got != 1 || partial.Rules[0].Command != "beta" {
		t.Fatalf("partial read = %#v, want beta only", partial.Rules)
	}

	doc, err = fs.DiscardInstructions(ctx, id, DiscardInstructionParams{SeqNo: []int{0}})
	if err != nil {
		t.Fatalf("discard: %v", err)
	}
	if got := len(doc.Rules); got != 1 || doc.Rules[0].Command != "beta" {
		t.Fatalf("discard document = %#v, want beta only", doc.Rules)
	}

	index, err := fs.GetFolderIndex(ctx, InstructionID{
		AccountPrefix: "u_1",
		Bucket:        "default",
		Path:          "rules",
	})
	if err != nil {
		t.Fatalf("folder index: %v", err)
	}
	if len(index) == 0 {
		t.Fatalf("expected folder entries")
	}
}

func TestHTTPHandlerAppendAndGet(t *testing.T) {
	ctx := context.Background()
	fs, err := NewFileSystem(t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("new filesystem: %v", err)
	}
	defer fs.Close()

	handler := NewHTTPHandler(fs)
	body := `{"account_prefix":"u_1","bucket":"default","path":"rules","file_name":"instructions.txt","session_id":"session-a","rules":[{"Command":"alpha","Seqno":0}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/instructions/append", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("append status = %d, want 200", rr.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/instructions?account_prefix=u_1&bucket=default&path=rules&file_name=instructions.txt&session_id=session-a", nil)
	getRR := httptest.NewRecorder()
	handler.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", getRR.Code)
	}
	if !strings.Contains(getRR.Body.String(), "alpha") {
		t.Fatalf("get response missing rule: %s", getRR.Body.String())
	}

	var payload map[string]any
	if err := json.NewDecoder(bytes.NewReader(getRR.Body.Bytes())).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := payload["data"]; !ok {
		t.Fatalf("response missing data field: %v", payload)
	}

	_ = ctx
}

func TestWriteJSONErrorShape(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, nil, io.EOF)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"error"`) {
		t.Fatalf("expected error payload, got %s", rr.Body.String())
	}
}
