package server

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	applog "github.com/Shaik-Sirajuddin/memory/mcp/log"
)

type ConnectionManager struct {
	mu          sync.RWMutex
	connections map[string]*Connection
	nextID      atomic.Uint64
}

func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		connections: make(map[string]*Connection),
	}
}

func (m *ConnectionManager) Add(remoteAddr string, userAgent string) *Connection {
	id := fmt.Sprintf("conn-%d", m.nextID.Add(1))
	conn := &Connection{
		ID:         id,
		RemoteAddr: remoteAddr,
		UserAgent:  userAgent,
		OpenedAt:   time.Now().UTC(),
		Out:        make(chan RPCMessage, 16),
	}

	m.mu.Lock()
	m.connections[id] = conn
	m.mu.Unlock()

	applog.Logger.Info("connection registered", "connection_id", id, "remote_addr", remoteAddr)
	return conn
}

func (m *ConnectionManager) Remove(id string) {
	m.mu.Lock()
	conn, ok := m.connections[id]
	if ok {
		delete(m.connections, id)
		close(conn.Out)
	}
	m.mu.Unlock()

	if ok {
		applog.Logger.Info("connection removed", "connection_id", id)
	}
}

func (m *ConnectionManager) List() []ConnectionSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	connections := make([]ConnectionSnapshot, 0, len(m.connections))
	for _, conn := range m.connections {
		connections = append(connections, conn.Snapshot())
	}
	return connections
}

func (m *ConnectionManager) Targets(id string) []*Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if id != "" {
		conn, ok := m.connections[id]
		if !ok {
			return nil
		}
		return []*Connection{conn}
	}

	connections := make([]*Connection, 0, len(m.connections))
	for _, conn := range m.connections {
		connections = append(connections, conn)
	}
	return connections
}
