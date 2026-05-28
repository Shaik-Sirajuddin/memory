package internal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

var ErrNotFound = errors.New("terminal not found")

// PTYDaemon manages the lifecycle of PTY sessions backed by a persistent store.
type PTYDaemon interface {
	Create(PTYCreateParams) (*PTYTerminalInfo, error)
	Adopt(agentID, sessionID string, pid int, submitKey string) error
	Pipe(agentID, sessionID string, data []byte) error
	Exec(agentID, sessionID, prompt string) error
	Stop(agentID, sessionID string) error
	List() ([]*PTYTerminalInfo, error)
	ListSessions(agentID string) ([]*PTYSessionRecord, error)
	GetSession(agentID, sessionID string) (*PTYSessionRecord, error)
	// GetMasterFd returns the PTY master file for the session.
	// The caller must not close the file; it is owned by the daemon.
	GetMasterFd(agentID, sessionID string) (*os.File, error)
	Shutdown(ctx context.Context) error
}

type defaultDaemon struct {
	store     *Store
	mu        sync.RWMutex
	terminals map[string]*PTYTerminal
}

func NewDaemon(store *Store) PTYDaemon {
	_ = store.MarkAllActiveCrashed()
	return &defaultDaemon{
		store:     store,
		terminals: make(map[string]*PTYTerminal),
	}
}

func termKey(agentID, sessionID string) string {
	return agentID + "/" + sessionID
}

func (d *defaultDaemon) get(agentID, sessionID string) *PTYTerminal {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.terminals[termKey(agentID, sessionID)]
}

func (d *defaultDaemon) Create(p PTYCreateParams) (*PTYTerminalInfo, error) {
	key := termKey(p.AgentID, p.SessionID)

	d.mu.Lock()
	if _, exists := d.terminals[key]; exists {
		d.mu.Unlock()
		return nil, fmt.Errorf("terminal %s already exists", key)
	}
	d.mu.Unlock()

	cmd := exec.Command(p.Command, p.Args...)
	cmd.Env = append(os.Environ(), p.Env...)
	cmd.Dir = p.Dir

	master, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("pty start: %w", err)
	}

	info := PTYTerminalInfo{
		AgentID:   p.AgentID,
		SessionID: p.SessionID,
		PID:       cmd.Process.Pid,
		Status:    StatusActive,
	}

	t := &PTYTerminal{
		PTYTerminalInfo: info,
		master:          master,
		cmd:             cmd,
		submitKey:       p.SubmitKey,
	}

	d.mu.Lock()
	d.terminals[key] = t
	d.mu.Unlock()

	if err := d.store.Insert(&info, p.SubmitKey); err != nil {
		return nil, fmt.Errorf("store insert: %w", err)
	}

	go watchTerminal(t, d.store, d.removeTerminal)

	return &info, nil
}

func (d *defaultDaemon) Pipe(agentID, sessionID string, data []byte) error {
	t := d.get(agentID, sessionID)
	if t == nil {
		return ErrNotFound
	}
	return t.write(data)
}

func (d *defaultDaemon) Exec(agentID, sessionID, prompt string) error {
	t := d.get(agentID, sessionID)
	if t == nil {
		return ErrNotFound
	}
	return t.execPrompt(prompt)
}

func (d *defaultDaemon) Adopt(agentID, sessionID string, pid int, submitKey string) error {
	key := termKey(agentID, sessionID)

	d.mu.RLock()
	_, exists := d.terminals[key]
	d.mu.RUnlock()
	if exists {
		return nil // idempotent
	}

	// If caller doesn't supply a submit key, recover it from the store.
	// Non-fatal on error: exec falls back to plain \r.
	if submitKey == "" {
		rec, recErr := d.store.GetBySession(agentID, sessionID)
		if recErr != nil {
			_ = recErr // logged by caller layer; internal pkg has no logger
		} else if rec != nil {
			submitKey = rec.SubmitKey
		}
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}

	master, err := os.OpenFile(fmt.Sprintf("/proc/%d/fd/0", pid), os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open pty fd for pid %d: %w", pid, err)
	}

	info := PTYTerminalInfo{
		AgentID:   agentID,
		SessionID: sessionID,
		PID:       pid,
		Status:    StatusActive,
	}

	t := &PTYTerminal{
		PTYTerminalInfo: info,
		master:          master,
		cmd:             nil,
		proc:            proc,
		submitKey:       submitKey,
	}

	d.mu.Lock()
	d.terminals[key] = t
	d.mu.Unlock()

	if err := d.store.Insert(&info, submitKey); err != nil {
		return fmt.Errorf("store insert: %w", err)
	}

	go watchAdopted(t, d.store, d.removeTerminal)

	return nil
}

func (d *defaultDaemon) Stop(agentID, sessionID string) error {
	t := d.get(agentID, sessionID)
	if t == nil {
		return ErrNotFound
	}
	if t.cmd == nil {
		// adopted session — process not owned by daemon; just mark stopped and remove
		t.setStatus(StatusStopped)
		_ = d.store.UpdateStatus(agentID, sessionID, StatusStopped)
		d.removeTerminal(agentID, sessionID)
		return nil
	}
	return t.kill()
}

func (d *defaultDaemon) List() ([]*PTYTerminalInfo, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	infos := make([]*PTYTerminalInfo, 0, len(d.terminals))
	for _, t := range d.terminals {
		t.mu.Lock()
		cp := t.PTYTerminalInfo
		t.mu.Unlock()
		infos = append(infos, &cp)
	}
	return infos, nil
}

func (d *defaultDaemon) Shutdown(ctx context.Context) error {
	d.mu.RLock()
	terminals := make([]*PTYTerminal, 0, len(d.terminals))
	for _, t := range d.terminals {
		terminals = append(terminals, t)
	}
	d.mu.RUnlock()

	for _, t := range terminals {
		_ = t.kill()
	}
	return nil
}

func (d *defaultDaemon) ListSessions(agentID string) ([]*PTYSessionRecord, error) {
	return d.store.ListByAgent(agentID)
}

func (d *defaultDaemon) GetSession(agentID, sessionID string) (*PTYSessionRecord, error) {
	return d.store.GetBySession(agentID, sessionID)
}

// GetMasterFd returns the PTY master file for the given session.
// The returned file is owned by the daemon; callers must not close it.
func (d *defaultDaemon) GetMasterFd(agentID, sessionID string) (*os.File, error) {
	t := d.get(agentID, sessionID)
	if t == nil {
		return nil, ErrNotFound
	}
	t.mu.Lock()
	f := t.master
	t.mu.Unlock()
	if f == nil {
		return nil, errors.New("terminal has no master fd")
	}
	return f, nil
}

func (d *defaultDaemon) removeTerminal(agentID, sessionID string) {
	d.mu.Lock()
	delete(d.terminals, termKey(agentID, sessionID))
	d.mu.Unlock()
}
