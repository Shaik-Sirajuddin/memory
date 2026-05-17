package internal

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"
)

var startTime = time.Now()

func NewHandler(d PTYDaemon) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /create", handleCreate(d))
	mux.HandleFunc("POST /pipe", handlePipe(d))
	mux.HandleFunc("POST /exec", handleExec(d))
	mux.HandleFunc("POST /stop", handleStop(d))
	mux.HandleFunc("GET /list", handleList(d))
	mux.HandleFunc("GET /status", handleStatus(d))
	mux.HandleFunc("POST /adopt", handleAdopt(d))
	mux.HandleFunc("GET /sessions", handleListSessions(d))
	mux.HandleFunc("GET /session", handleGetSession(d))
	return mux
}

func handleCreate(d PTYDaemon) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var p PTYCreateParams
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		info, err := d.Create(p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, info)
	}
}

func handlePipe(d PTYDaemon) http.HandlerFunc {
	type req struct {
		AgentID   string `json:"agent_id"`
		SessionID string `json:"session_id"`
		Data      []byte `json:"data"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var p req
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := d.Pipe(p.AgentID, p.SessionID, p.Data); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleExec(d PTYDaemon) http.HandlerFunc {
	type req struct {
		AgentID   string `json:"agent_id"`
		SessionID string `json:"session_id"`
		Prompt    string `json:"prompt"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var p req
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := d.Exec(p.AgentID, p.SessionID, p.Prompt); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleStop(d PTYDaemon) http.HandlerFunc {
	type req struct {
		AgentID   string `json:"agent_id"`
		SessionID string `json:"session_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var p req
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := d.Stop(p.AgentID, p.SessionID); err != nil {
			if errors.Is(err, ErrNotFound) {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleList(d PTYDaemon) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		infos, err := d.List()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, infos)
	}
}

func handleAdopt(d PTYDaemon) http.HandlerFunc {
	type req struct {
		AgentID   string `json:"agent_id"`
		SessionID string `json:"session_id"`
		PID       int    `json:"pid"`
		SubmitKey string `json:"submit_key"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var p req
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := d.Adopt(p.AgentID, p.SessionID, p.PID, p.SubmitKey); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleStatus(d PTYDaemon) http.HandlerFunc {
	type activeEntry struct {
		AgentID   string `json:"agent_id"`
		SessionID string `json:"session_id"`
		PID       int    `json:"pid"`
		Status    Status `json:"status"`
		StartedAt string `json:"started_at"`
	}
	type statusResponse struct {
		PID      int           `json:"pid"`
		Uptime   float64       `json:"uptime"`
		Sessions int           `json:"sessions"`
		Active   []activeEntry `json:"active"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		infos, err := d.List()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		active := make([]activeEntry, 0, len(infos))
		for _, info := range infos {
			if info.Status == StatusActive {
				active = append(active, activeEntry{
					AgentID:   info.AgentID,
					SessionID: info.SessionID,
					PID:       info.PID,
					Status:    info.Status,
					StartedAt: "",
				})
			}
		}
		resp := statusResponse{
			PID:      os.Getpid(),
			Uptime:   time.Since(startTime).Seconds(),
			Sessions: len(active),
			Active:   active,
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

type sessionResponse struct {
	AgentID   string  `json:"agent_id"`
	SessionID string  `json:"session_id"`
	PID       int     `json:"pid"`
	Status    string  `json:"status"`
	StartedAt string  `json:"started_at"`
	StoppedAt *string `json:"stopped_at,omitempty"`
}

func toSessionResponse(rec *PTYSessionRecord) sessionResponse {
	r := sessionResponse{
		AgentID:   rec.AgentID,
		SessionID: rec.SessionID,
		PID:       rec.PID,
		Status:    string(rec.Status),
		StartedAt: rec.StartedAt.UTC().Format(time.RFC3339),
	}
	if rec.StoppedAt != nil {
		s := rec.StoppedAt.UTC().Format(time.RFC3339)
		r.StoppedAt = &s
	}
	return r
}

func handleListSessions(d PTYDaemon) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := r.URL.Query().Get("agent_id")
		records, err := d.ListSessions(agentID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp := make([]sessionResponse, 0, len(records))
		for _, rec := range records {
			resp = append(resp, toSessionResponse(rec))
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func handleGetSession(d PTYDaemon) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := r.URL.Query().Get("agent_id")
		sessionID := r.URL.Query().Get("session_id")
		if agentID == "" || sessionID == "" {
			http.Error(w, "agent_id and session_id are required", http.StatusBadRequest)
			return
		}
		rec, err := d.GetSession(agentID, sessionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if rec == nil {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, toSessionResponse(rec))
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
