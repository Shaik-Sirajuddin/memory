package store

import "github.com/Shaik-Sirajuddin/memory/omniagent"

// CodeSessionStore provides persistent storage for code sessions.
// It is agent-independent; callers supply the agentID on each call.
type CodeSessionStore interface {
	GetSession(agentID string) (*omniagent.CodeSession, error)
	CreateSession(agentID string, session *omniagent.CodeSession) error
	UpdateSession(agentID string, session *omniagent.CodeSession) error
	ListSessions(agentID string, filter *omniagent.CodeSession) ([]*omniagent.CodeSession, error)
}
