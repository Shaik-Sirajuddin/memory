package internal

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Status string

const (
	StatusActive  Status = "active"
	StatusStopped Status = "stopped"
	StatusCrashed Status = "crashed"
)

const (
	ctrlU      = "\x15"
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"

	enterKey              = "\r"
	csiUShiftEnter        = "\x1b[13;2u"
	modifyOtherShiftEnter = "\x1b[27;2;13~"

	maxInputBuf   = 4096
	inputQueueCap = 2
	// carrySize must cover the longest escape sequence we detect (7 bytes for CSI-u shift-enter).
	carrySize = 8
)

type PTYCreateParams struct {
	AgentID   string   `json:"agent_id"`
	SessionID string   `json:"session_id"`
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	SubmitKey string   `json:"submit_key"`
	Env       []string `json:"env"`
	Dir       string   `json:"dir"` // working directory for the spawned process
}

type PTYTerminalInfo struct {
	AgentID   string `json:"agent_id"`
	SessionID string `json:"session_id"`
	PID       int    `json:"pid"`
	Status    Status `json:"status"`
}

type PTYTerminal struct {
	PTYTerminalInfo
	master *os.File
	cmd    *exec.Cmd
	proc   *os.Process // set for adopted processes (cmd == nil)
	// submitKey is set once at Create/Adopt and never modified; safe to read
	// under execMu without holding t.mu.
	submitKey string
	mu        sync.Mutex

	// execMu serialises the snapshot→payload→reinject triple so concurrent
	// ExecInSession calls cannot interleave their writes on the PTY master.
	execMu sync.Mutex

	// humanMu guards the input tracking state below.
	humanMu          sync.Mutex
	currentInput     []byte
	inputQueue       [inputQueueCap][]byte
	queueLen         int
	inBracketedPaste bool
	// carry holds the tail of the last relay chunk to detect escape sequences
	// that span two consecutive reads (edge case #1 in the plan).
	carry  [carrySize]byte
	carryN int

	// userInputCh is signalled (non-blocking) by trackHumanInput whenever the
	// relay receives a chunk from the human. execPrompt drains it before
	// writing, then waits on it during the submit-key delay.
	userInputCh chan struct{}
}

func (t *PTYTerminal) write(p []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.master == nil {
		return errors.New("no pty master: terminal was adopted without a writable fd")
	}
	_, err := t.master.Write(p)
	return err
}

func (t *PTYTerminal) kill() error {
	if t.cmd != nil {
		return t.cmd.Process.Kill()
	}
	if t.proc != nil {
		return t.proc.Kill()
	}
	return nil
}

// execPrompt serialises the paste write under execMu, then releases the lock
// before each sleep so concurrent ExecInSession callers are not forced to wait.
// The submit key is sent up to three times (immediate + two retries at 100ms
// intervals) to handle terminals that process the paste before the key fires.
// Each retry is skipped if the human types during the wait window.
func (t *PTYTerminal) execPrompt(prompt string) error {
	t.execMu.Lock()
	saved := t.snapshotInput()

	// Drain any stale user-input signal so retries only react to input that
	// arrives after this exec started.
	if t.userInputCh != nil {
		select {
		case <-t.userInputCh:
		default:
		}
	}

	// Attempt 1: paste + submit key + reinject in one write.
	payload := buildExecPayload(prompt, t.submitKey, saved)
	err := t.write(payload)
	t.execMu.Unlock()
	if err != nil {
		return err
	}

	submitKey := submitSeq(t.submitKey)

	for attempt := 2; attempt <= 3; attempt++ {
		if userInputArrived(t.userInputCh, 100*time.Millisecond) {
			return nil
		}
		t.execMu.Lock()
		werr := t.write(submitKey)
		t.execMu.Unlock()
		slog.Debug("ptydaemon: submit-key retry", "attempt", attempt, "session_id", t.SessionID, "err", werr)
		if werr != nil {
			return werr
		}
	}
	return nil
}

// userInputArrived waits up to d for a signal on ch.
// Returns true if the human typed during the window, false if the timer fired.
func userInputArrived(ch chan struct{}, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	if ch == nil {
		<-timer.C
		return false
	}
	select {
	case <-timer.C:
		return false
	case <-ch:
		return true
	}
}

func (t *PTYTerminal) setStatus(s Status) {
	t.mu.Lock()
	t.Status = s
	t.mu.Unlock()
}

// trackHumanInput is called by the stdin relay before forwarding each chunk
// to the PTY master. It maintains currentInput and pops the queue when the
// human submits or clears the line.
func (t *PTYTerminal) trackHumanInput(chunk []byte) {
	// Signal execPrompt that the human is at the keyboard.
	if t.userInputCh != nil {
		select {
		case t.userInputCh <- struct{}{}:
		default:
		}
	}

	t.humanMu.Lock()
	defer t.humanMu.Unlock()

	// Prepend carry bytes to detect sequences split across chunk boundaries.
	var buf []byte
	if t.carryN > 0 {
		buf = make([]byte, t.carryN+len(chunk))
		copy(buf, t.carry[:t.carryN])
		copy(buf[t.carryN:], chunk)
		t.carryN = 0
	} else {
		buf = chunk
	}

	// Update bracketed-paste state so we don't mistake embedded \r for submit.
	if bytes.Contains(buf, []byte(pasteStart)) {
		t.inBracketedPaste = true
	}
	if bytes.Contains(buf, []byte(pasteEnd)) {
		t.inBracketedPaste = false
	}

	if !t.inBracketedPaste && isSubmitOrClear(buf) {
		t.popQueueLocked()
		t.currentInput = nil
		return
	}

	// Save the tail as carry for the next read.
	n := len(buf)
	if n > carrySize {
		n = carrySize
	}
	copy(t.carry[:], buf[len(buf)-n:])
	t.carryN = n

	t.currentInput = append(t.currentInput, chunk...)
	if len(t.currentInput) > maxInputBuf {
		t.currentInput = t.currentInput[len(t.currentInput)-maxInputBuf:]
	}
}

// snapshotInput saves the current partial input to the queue and returns it.
// Returns nil when the human has not typed anything since the last snapshot.
func (t *PTYTerminal) snapshotInput() []byte {
	t.humanMu.Lock()
	defer t.humanMu.Unlock()
	if len(t.currentInput) == 0 {
		return nil
	}
	snap := append([]byte(nil), t.currentInput...)
	// Drop oldest entry when queue is full (queue overflow, edge case #4).
	if t.queueLen == inputQueueCap {
		copy(t.inputQueue[:], t.inputQueue[1:])
		t.inputQueue[inputQueueCap-1] = nil // release the old slice (minor: prevents GC leak)
		t.queueLen--
	}
	t.inputQueue[t.queueLen] = snap
	t.queueLen++
	t.currentInput = nil
	return snap
}

func (t *PTYTerminal) popQueueLocked() {
	if t.queueLen == 0 {
		return
	}
	copy(t.inputQueue[:], t.inputQueue[1:])
	t.inputQueue[t.queueLen-1] = nil // release the now-unused tail slot
	t.queueLen--
}

// isSubmitOrClear returns true when b contains a line-submit or clear-line
// sequence outside of a bracketed paste. Call only when inBracketedPaste is false.
func isSubmitOrClear(b []byte) bool {
	s := string(b)
	return strings.ContainsAny(s, "\r\x15") ||
		strings.Contains(s, csiUShiftEnter) ||
		strings.Contains(s, modifyOtherShiftEnter)
}

// buildExecPayload assembles the bracketed-paste sequence and appends reinject
// bytes in a single allocation so they are written atomically in one t.write()
// call (edge case #3: no window between payload and reinject).
func buildExecPayload(prompt, submitKey string, reinject []byte) []byte {
	seq := ctrlU + pasteStart + prompt + pasteEnd
	result := append([]byte(seq), submitSeq(submitKey)...)
	if len(reinject) > 0 {
		result = append(result, reinject...)
	}
	return result
}

func submitSeq(name string) []byte {
	switch strings.ToLower(name) {
	case "shift-enter", "shift_enter", "csi-u-shift-enter":
		return []byte(csiUShiftEnter)
	case "modify-other-keys-shift-enter", "modify_other_keys_shift_enter":
		return []byte(modifyOtherShiftEnter)
	default:
		return []byte(enterKey)
	}
}
