package internal

import (
	"fmt"
	"os"
	"time"
)

func watchTerminal(t *PTYTerminal, store *Store, onExit func(agentID, sessionID string)) {
	err := t.cmd.Wait()

	status := StatusStopped
	if err != nil {
		status = StatusCrashed
	}

	t.setStatus(status)
	_ = store.UpdateStatus(t.AgentID, t.SessionID, status)

	if onExit != nil {
		onExit(t.AgentID, t.SessionID)
	}
}

func watchAdopted(t *PTYTerminal, store *Store, remove func(string, string)) {
	procDir := fmt.Sprintf("/proc/%d", t.PID)
	for {
		if _, err := os.Stat(procDir); os.IsNotExist(err) {
			t.setStatus(StatusStopped)
			_ = store.UpdateStatus(t.AgentID, t.SessionID, StatusStopped)
			remove(t.AgentID, t.SessionID)
			return
		}
		time.Sleep(2 * time.Second)
	}
}
