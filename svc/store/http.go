package store

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
)

// Handler exposes the store over HTTP.
type Handler struct {
	store Store
}

// NewHTTPHandler builds an http.Handler for the store API.
func NewHTTPHandler(store Store) http.Handler {
	return &Handler{store: store}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/instructions":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleGetInstructions(w, r)
	case "/v1/instructions/meta":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleGetMeta(w, r)
	case "/v1/instructions/partial":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleGetPartial(w, r)
	case "/v1/instructions/append":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleAppend(w, r)
	case "/v1/instructions/update":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleUpdate(w, r)
	case "/v1/instructions/delete":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleDelete(w, r)
	case "/v1/instructions/discard":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleDiscard(w, r)
	case "/v1/folders/index":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleFolderIndex(w, r)
	case "/v1/subscriptions/stream":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleStream(w, r)
	default:
		http.NotFound(w, r)
	}
}

type instructionPayload struct {
	AccountPrefix string `json:"account_prefix"`
	Bucket        string `json:"bucket"`
	Path          string `json:"path"`
	FileName      string `json:"file_name"`
	SessionID     string `json:"session_id"`
	IgnoreSelf    bool   `json:"ignore_self"`
	Start         int    `json:"start"`
	End           int    `json:"end"`
	Rules         []Rule `json:"rules"`
	SeqNo         []int  `json:"seq_no"`
}

func (p instructionPayload) id() InstructionID {
	return InstructionID{
		AccountPrefix: p.AccountPrefix,
		Bucket:        p.Bucket,
		Path:          p.Path,
		FileName:      p.FileName,
		SessionID:     p.SessionID,
	}
}

func (h *Handler) handleGetInstructions(w http.ResponseWriter, r *http.Request) {
	doc, err := h.store.GetInstructions(r.Context(), payloadFromQuery(r).id())
	writeJSON(w, doc, err)
}

func (h *Handler) handleGetMeta(w http.ResponseWriter, r *http.Request) {
	meta, err := h.store.GetInstructionsMeta(r.Context(), payloadFromQuery(r).id())
	writeJSON(w, meta, err)
}

func (h *Handler) handleGetPartial(w http.ResponseWriter, r *http.Request) {
	q := payloadFromQuery(r)
	doc, err := h.store.GetInstructionsPartial(r.Context(), q.id(), GetPartialInstructionParams{
		Range: Range{Start: q.Start, End: q.End},
	})
	writeJSON(w, doc, err)
}

func (h *Handler) handleAppend(w http.ResponseWriter, r *http.Request) {
	p, err := decodePayload(r.Body)
	if err != nil {
		writeJSON(w, nil, err)
		return
	}
	doc, err := h.store.AppendInstructions(r.Context(), p.id(), AppendInstructionParams{Rules: p.Rules})
	writeJSON(w, doc, err)
}

func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	p, err := decodePayload(r.Body)
	if err != nil {
		writeJSON(w, nil, err)
		return
	}
	doc, err := h.store.UpdateInstructions(r.Context(), p.id(), UpdateInstructionParams{Rules: p.Rules})
	writeJSON(w, doc, err)
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	p, err := decodePayload(r.Body)
	if err != nil {
		writeJSON(w, nil, err)
		return
	}
	doc, err := h.store.DeleteInstructions(r.Context(), p.id(), DeleteInstructionParams{SeqNo: p.SeqNo})
	writeJSON(w, doc, err)
}

func (h *Handler) handleDiscard(w http.ResponseWriter, r *http.Request) {
	p, err := decodePayload(r.Body)
	if err != nil {
		writeJSON(w, nil, err)
		return
	}
	doc, err := h.store.DiscardInstructions(r.Context(), p.id(), DiscardInstructionParams{SeqNo: p.SeqNo})
	writeJSON(w, doc, err)
}

func (h *Handler) handleFolderIndex(w http.ResponseWriter, r *http.Request) {
	out, err := h.store.GetFolderIndex(r.Context(), payloadFromQuery(r).id())
	writeJSON(w, out, err)
}

func (h *Handler) handleStream(w http.ResponseWriter, r *http.Request) {
	p := payloadFromQuery(r)
	sub, err := h.store.SubscribeInstructions(r.Context(), p.id(), SubscriptionParams{IgnoreSelf: p.IgnoreSelf})
	if err != nil {
		writeJSON(w, nil, err)
		return
	}
	defer sub.Close()

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, nil, errors.New("streaming not supported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	enc := json.NewEncoder(w)
	for {
		select {
		case evt, ok := <-sub.Events:
			if !ok {
				return
			}
			if _, err := io.WriteString(w, "data: "); err != nil {
				return
			}
			if err := enc.Encode(evt); err != nil {
				return
			}
			if _, err := io.WriteString(w, "\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}

func payloadFromQuery(r *http.Request) instructionPayload {
	q := r.URL.Query()
	return instructionPayload{
		AccountPrefix: q.Get("account_prefix"),
		Bucket:        q.Get("bucket"),
		Path:          q.Get("path"),
		FileName:      q.Get("file_name"),
		SessionID:     q.Get("session_id"),
		IgnoreSelf:    parseBool(q.Get("ignore_self")),
		Start:         parseInt(q.Get("start")),
		End:           parseInt(q.Get("end")),
	}
}

func decodePayload(body io.ReadCloser) (instructionPayload, error) {
	defer body.Close()
	var p instructionPayload
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return instructionPayload{}, err
	}
	return p, nil
}

func writeJSON(w http.ResponseWriter, v any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": err.Error(),
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": v,
	})
}

func parseBool(v string) bool {
	switch v {
	case "1", "true", "TRUE", "True", "yes", "YES", "on":
		return true
	default:
		return false
	}
}

func parseInt(v string) int {
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

var _ http.Handler = (*Handler)(nil)
