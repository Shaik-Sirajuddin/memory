package ptyunix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Shaik-Sirajuddin/memory/svc/ptydaemon/internal"
	ptylog "github.com/Shaik-Sirajuddin/memory/svc/ptydaemon/log"
	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

// session is an in-memory PTY session used in fallback (no-inner) mode.
type session struct {
	ptmx *os.File
	cmd  *exec.Cmd
	mu   sync.Mutex
}

// Daemon serves the ptyunix raw JSON protocol over a Unix domain socket.
// When constructed with NewDaemonWithInner, all session operations are
// delegated to the provided PTYDaemon (store-backed). NewDaemon returns
// a lightweight in-memory daemon for backward compatibility.
type Daemon struct {
	mu       sync.Mutex
	sessions map[string]*session // used only when inner == nil
	inner    internal.PTYDaemon  // nil = in-memory fallback

	// attachedPIDs tracks the PID of the client currently holding the PTY
	// master fd for each session (keyed by sessionID). Used instead of
	// /proc inode scanning, which is broken for PTY masters: all ptmx fds
	// share the same inode on Linux.
	attachedPIDs sync.Map // sessionID (string) → client PID (int)
}

// NewDaemon returns an in-memory daemon with no persistent store.
// Use NewDaemonWithInner for production use.
func NewDaemon() *Daemon {
	return &Daemon{sessions: make(map[string]*session)}
}

// NewDaemonWithInner returns a daemon that delegates all session operations
// to the provided PTYDaemon, which is expected to be store-backed.
func NewDaemonWithInner(inner internal.PTYDaemon) *Daemon {
	return &Daemon{inner: inner}
}

// ListenAndServe listens on socketPath and serves connections until ctx is cancelled.
func (d *Daemon) ListenAndServe(ctx context.Context, socketPath string) error {
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	if err := os.Chmod(socketPath, 0600); err != nil {
		ptylog.Logger.Warn("failed to set socket permissions", "path", socketPath, "err", err)
	}

	ptylog.Logger.Info("ptyunix listening", "socket", socketPath)

	go func() {
		<-ctx.Done()
		ln.Close()
		_ = os.Remove(socketPath)
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go d.handleConn(conn.(*net.UnixConn))
	}
}

func (d *Daemon) handleConn(conn *net.UnixConn) {
	defer conn.Close()

	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		ptylog.Logger.Debug("failed to decode request", "err", err)
		return
	}

	ptylog.Logger.Debug("request received", "op", req.Op, "agent_id", req.AgentID, "session_id", req.SessionID)

	switch req.Op {
	case "start":
		d.handleStart(conn, req)
	case "create":
		d.handleCreate(conn, req)
	case "adopt":
		d.handleAdopt(conn, req)
	case "attach":
		d.handleAttach(conn, req)
	case "pipe":
		d.handlePipe(conn, req)
	case "exec":
		d.handleExec(conn, req)
	case "keybind":
		d.handleKeybind(conn, req)
	case "stop":
		d.handleStop(conn, req)
	case "list":
		d.handleList(conn, req)
	case "get":
		d.handleGet(conn, req)
	case "list-attached":
		d.handleListAttached(conn, req)
	case "meta-attached":
		d.handleMetaAttached(conn, req)
	default:
		respond(conn, Response{Error: "unknown op: " + req.Op})
	}
}

// handleStart is the legacy in-memory start op (no agentID, no store).
// When inner is set it delegates to handleCreate for store-backed behaviour.
func (d *Daemon) handleStart(conn *net.UnixConn, req Request) {
	if d.inner != nil {
		d.handleCreate(conn, req)
		return
	}
	if len(req.Command) == 0 {
		respond(conn, Response{Error: "command is required"})
		return
	}

	ptylog.Logger.Debug("starting in-memory session", "session_id", req.SessionID, "command", req.Command)

	cmd := exec.Command(req.Command[0], req.Command[1:]...)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		ptylog.Logger.Error("pty start failed", "err", err, "session_id", req.SessionID)
		respond(conn, Response{Error: err.Error()})
		return
	}

	s := &session{ptmx: ptmx, cmd: cmd}
	d.mu.Lock()
	d.sessions[req.SessionID] = s
	d.mu.Unlock()

	go func() {
		_ = cmd.Wait()
		_ = ptmx.Close()
		d.mu.Lock()
		delete(d.sessions, req.SessionID)
		d.mu.Unlock()
		ptylog.Logger.Debug("in-memory session exited", "session_id", req.SessionID)
	}()

	ptylog.Logger.Info("in-memory session started", "session_id", req.SessionID, "pid", cmd.Process.Pid)
	respond(conn, Response{OK: true, SessionID: req.SessionID})
}

// handleCreate starts a new store-backed PTY session.
func (d *Daemon) handleCreate(conn *net.UnixConn, req Request) {
	if d.inner == nil {
		respond(conn, Response{Error: "create requires a store-backed daemon"})
		return
	}
	if len(req.Command) == 0 {
		respond(conn, Response{Error: "command is required"})
		return
	}

	p := internal.PTYCreateParams{
		AgentID:   req.AgentID,
		SessionID: req.SessionID,
		Command:   req.Command[0],
		Args:      req.Command[1:],
		SubmitKey: req.SubmitKey,
		Env:       req.Env,
		Dir:       req.Dir,
	}

	ptylog.Logger.Debug("creating session", "agent_id", p.AgentID, "session_id", p.SessionID, "command", p.Command)

	info, err := d.inner.Create(p)
	if err != nil {
		ptylog.Logger.Error("create session failed", "err", err, "session_id", req.SessionID)
		respond(conn, Response{Error: err.Error()})
		return
	}

	ptylog.Logger.Info("session created", "agent_id", info.AgentID, "session_id", info.SessionID, "pid", info.PID)
	respond(conn, Response{OK: true, SessionID: info.SessionID})
}

// handleAdopt registers an externally started process as a managed session.
func (d *Daemon) handleAdopt(conn *net.UnixConn, req Request) {
	if d.inner == nil {
		respond(conn, Response{Error: "adopt requires a store-backed daemon"})
		return
	}

	ptylog.Logger.Debug("adopting process", "agent_id", req.AgentID, "session_id", req.SessionID, "pid", req.PID)

	if err := d.inner.Adopt(req.AgentID, req.SessionID, req.PID, req.SubmitKey); err != nil {
		ptylog.Logger.Error("adopt failed", "err", err, "session_id", req.SessionID, "pid", req.PID)
		respond(conn, Response{Error: err.Error()})
		return
	}

	ptylog.Logger.Info("process adopted", "agent_id", req.AgentID, "session_id", req.SessionID, "pid", req.PID)
	respond(conn, Response{OK: true})
}

// handleAttach sends the PTY master fd to the caller via SCM_RIGHTS.
// The attach guard tracks the attached client's PID via SO_PEERCRED.
// A second attach is rejected unless the previously attached process has exited.
func (d *Daemon) handleAttach(conn *net.UnixConn, req Request) {
	// Resolve client PID before doing anything else so we can guard accurately.
	clientPID, err := peerPID(conn)
	if err != nil {
		ptylog.Logger.Warn("attach: SO_PEERCRED failed, skipping guard", "err", err, "session_id", req.SessionID)
	}

	ptylog.Logger.Debug("attach guard check", "session_id", req.SessionID, "client_pid", clientPID)

	// Reject if a live client is already attached to this specific session.
	if prev, ok := d.attachedPIDs.Load(req.SessionID); ok {
		prevPID := prev.(int)
		if processExists(prevPID) {
			ptylog.Logger.Warn("attach denied — session already attached", "session_id", req.SessionID, "holder_pid", prevPID)
			respond(conn, Response{
				OK:    false,
				Error: fmt.Sprintf("session already attached (pid %d)", prevPID),
			})
			return
		}
		// Previous client has exited; allow re-attach.
		ptylog.Logger.Debug("attach: previous client gone, allowing re-attach", "session_id", req.SessionID, "old_pid", prevPID)
		d.attachedPIDs.Delete(req.SessionID)
	}

	ptmx, err := d.getMasterFd(req)
	if err != nil {
		respond(conn, Response{Error: err.Error()})
		return
	}

	rights := unix.UnixRights(int(ptmx.Fd()))
	payload, err := json.Marshal(Response{OK: true})
	if err != nil {
		ptylog.Logger.Error("attach response encode failed", "err", err, "session_id", req.SessionID)
		respond(conn, Response{Error: err.Error()})
		return
	}
	payload = append(payload, '\n')

	if _, _, err := conn.WriteMsgUnix(payload, rights, nil); err != nil {
		ptylog.Logger.Error("SCM_RIGHTS send failed", "err", err, "session_id", req.SessionID)
		return
	}

	if clientPID > 0 {
		d.attachedPIDs.Store(req.SessionID, clientPID)
	}
	ptylog.Logger.Info("fd granted to client", "session_id", req.SessionID, "client_pid", clientPID)
}

// handlePipe writes raw bytes to the PTY master.
func (d *Daemon) handlePipe(conn *net.UnixConn, req Request) {
	if d.inner == nil {
		respond(conn, Response{Error: "pipe requires a store-backed daemon"})
		return
	}

	ptylog.Logger.Debug("pipe", "agent_id", req.AgentID, "session_id", req.SessionID, "bytes", len(req.Data))

	if err := d.inner.Pipe(req.AgentID, req.SessionID, req.Data); err != nil {
		ptylog.Logger.Error("pipe failed", "err", err, "session_id", req.SessionID)
		respond(conn, Response{Error: err.Error()})
		return
	}
	respond(conn, Response{OK: true})
}

// handleExec writes raw input to the PTY master (backward-compatible exec op).
func (d *Daemon) handleExec(conn *net.UnixConn, req Request) {
	ptylog.Logger.Debug("exec", "agent_id", req.AgentID, "session_id", req.SessionID)

	if d.inner != nil {
		if err := d.inner.Pipe(req.AgentID, req.SessionID, []byte(req.Input)); err != nil {
			if errors.Is(err, internal.ErrNotFound) {
				respond(conn, Response{Error: "session not found"})
				return
			}
			ptylog.Logger.Error("exec pipe failed", "err", err, "session_id", req.SessionID)
			respond(conn, Response{Error: err.Error()})
			return
		}
		respond(conn, Response{OK: true})
		return
	}

	// in-memory fallback
	d.mu.Lock()
	s, ok := d.sessions[req.SessionID]
	d.mu.Unlock()
	if !ok {
		respond(conn, Response{Error: "session not found"})
		return
	}
	s.mu.Lock()
	_, err := s.ptmx.Write([]byte(req.Input))
	s.mu.Unlock()
	if err != nil {
		respond(conn, Response{Error: err.Error()})
		return
	}
	respond(conn, Response{OK: true})
}

func (d *Daemon) handleKeybind(conn *net.UnixConn, req Request) {
	seq, ok := keybinds[req.Key]
	if !ok {
		respond(conn, Response{Error: fmt.Sprintf("unknown keybind: %q", req.Key)})
		return
	}

	ptylog.Logger.Debug("keybind", "session_id", req.SessionID, "key", req.Key)

	if d.inner != nil {
		if err := d.inner.Pipe(req.AgentID, req.SessionID, seq); err != nil {
			respond(conn, Response{Error: err.Error()})
			return
		}
		respond(conn, Response{OK: true})
		return
	}

	d.mu.Lock()
	s, ok2 := d.sessions[req.SessionID]
	d.mu.Unlock()
	if !ok2 {
		respond(conn, Response{Error: "session not found"})
		return
	}
	s.mu.Lock()
	_, err := s.ptmx.Write(seq)
	s.mu.Unlock()
	if err != nil {
		respond(conn, Response{Error: err.Error()})
		return
	}
	respond(conn, Response{OK: true})
}

func (d *Daemon) handleStop(conn *net.UnixConn, req Request) {
	ptylog.Logger.Debug("stop", "agent_id", req.AgentID, "session_id", req.SessionID)

	if d.inner != nil {
		if err := d.inner.Stop(req.AgentID, req.SessionID); err != nil {
			if errors.Is(err, internal.ErrNotFound) {
				respond(conn, Response{OK: true}) // idempotent
				return
			}
			ptylog.Logger.Error("stop failed", "err", err, "session_id", req.SessionID)
			respond(conn, Response{Error: err.Error()})
			return
		}
		d.attachedPIDs.Delete(req.SessionID)
		ptylog.Logger.Info("session stopped", "agent_id", req.AgentID, "session_id", req.SessionID)
		respond(conn, Response{OK: true})
		return
	}

	d.mu.Lock()
	s, ok := d.sessions[req.SessionID]
	if ok {
		delete(d.sessions, req.SessionID)
	}
	d.mu.Unlock()
	if !ok {
		respond(conn, Response{Error: "session not found"})
		return
	}
	_ = s.cmd.Process.Kill()
	_ = s.cmd.Wait()
	_ = s.ptmx.Close()
	respond(conn, Response{OK: true})
}

func (d *Daemon) handleList(conn *net.UnixConn, req Request) {
	ptylog.Logger.Debug("list", "agent_id", req.AgentID)

	if d.inner != nil {
		records, err := d.inner.ListSessions(req.AgentID)
		if err != nil {
			ptylog.Logger.Error("list sessions failed", "err", err)
			respond(conn, Response{Error: err.Error()})
			return
		}
		entries := make([]SessionEntry, 0, len(records))
		for _, r := range records {
			entries = append(entries, recordToEntry(r))
		}
		ptylog.Logger.Debug("list response", "count", len(entries))
		respond(conn, Response{OK: true, Sessions: entries})
		return
	}

	d.mu.Lock()
	entries := make([]SessionEntry, 0, len(d.sessions))
	for id := range d.sessions {
		entries = append(entries, SessionEntry{SessionID: id, Status: "active"})
	}
	d.mu.Unlock()
	respond(conn, Response{OK: true, Sessions: entries})
}

func (d *Daemon) handleGet(conn *net.UnixConn, req Request) {
	ptylog.Logger.Debug("get", "agent_id", req.AgentID, "session_id", req.SessionID)

	if d.inner != nil {
		rec, err := d.inner.GetSession(req.AgentID, req.SessionID)
		if err != nil {
			ptylog.Logger.Error("get session failed", "err", err, "session_id", req.SessionID)
			respond(conn, Response{Error: err.Error()})
			return
		}
		if rec == nil {
			respond(conn, Response{Error: "session not found"})
			return
		}
		entry := recordToEntry(rec)
		respond(conn, Response{OK: true, SessionID: rec.SessionID, Sessions: []SessionEntry{entry}})
		return
	}

	d.mu.Lock()
	_, ok := d.sessions[req.SessionID]
	d.mu.Unlock()
	if !ok {
		respond(conn, Response{Error: "session not found"})
		return
	}
	respond(conn, Response{OK: true, SessionID: req.SessionID, Sessions: []SessionEntry{
		{SessionID: req.SessionID, Status: "active"},
	}})
}

// handleListAttached returns the client process currently attached to the session.
func (d *Daemon) handleListAttached(conn *net.UnixConn, req Request) {
	ptylog.Logger.Debug("list-attached", "session_id", req.SessionID)
	var procs []AttachedProcess
	if pid, ok := d.attachedPIDs.Load(req.SessionID); ok {
		p := pid.(int)
		if processExists(p) {
			procs = []AttachedProcess{{PID: p, Comm: readComm(p)}}
		} else {
			d.attachedPIDs.Delete(req.SessionID)
		}
	}
	ptylog.Logger.Debug("list-attached response", "session_id", req.SessionID, "count", len(procs))
	respond(conn, Response{OK: true, SessionID: req.SessionID, Processes: procs})
}

// handleMetaAttached returns 1 if a live client is attached, 0 otherwise.
func (d *Daemon) handleMetaAttached(conn *net.UnixConn, req Request) {
	count := 0
	if pid, ok := d.attachedPIDs.Load(req.SessionID); ok {
		if processExists(pid.(int)) {
			count = 1
		} else {
			d.attachedPIDs.Delete(req.SessionID)
		}
	}
	ptylog.Logger.Debug("meta-attached", "session_id", req.SessionID, "count", count)
	respond(conn, Response{OK: true, SessionID: req.SessionID, Count: count})
}

// getMasterFd resolves the PTY master file for the session in the request,
// using inner when available and falling back to the in-memory sessions map.
func (d *Daemon) getMasterFd(req Request) (*os.File, error) {
	if d.inner != nil {
		f, err := d.inner.GetMasterFd(req.AgentID, req.SessionID)
		if err != nil {
			return nil, err
		}
		return f, nil
	}
	d.mu.Lock()
	s, ok := d.sessions[req.SessionID]
	d.mu.Unlock()
	if !ok {
		return nil, errors.New("session not found")
	}
	return s.ptmx, nil
}

// peerPID returns the PID of the process on the other end of a Unix socket
// using SO_PEERCRED.
func peerPID(conn *net.UnixConn) (int, error) {
	rc, err := conn.SyscallConn()
	if err != nil {
		return 0, err
	}
	var pid int
	var credErr error
	_ = rc.Control(func(fd uintptr) {
		cred, e := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if e != nil {
			credErr = e
			return
		}
		pid = int(cred.Pid)
	})
	return pid, credErr
}

// processExists reports whether the process with the given PID is still alive.
func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}

func readComm(pid int) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(b))
}

func recordToEntry(r *internal.PTYSessionRecord) SessionEntry {
	e := SessionEntry{
		AgentID:   r.AgentID,
		SessionID: r.SessionID,
		Status:    string(r.Status),
	}
	s := r.StartedAt.UTC().Format(time.RFC3339)
	e.StartedAt = &s
	if r.StoppedAt != nil {
		t := r.StoppedAt.UTC().Format(time.RFC3339)
		e.StoppedAt = &t
	}
	return e
}

func respond(conn *net.UnixConn, r Response) {
	_ = json.NewEncoder(conn).Encode(r)
}
