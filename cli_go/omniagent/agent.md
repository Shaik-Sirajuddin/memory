## omniagent

> Do not read implementation files unless explicitly asked to.

### Data Structs
- `omniagent.go` — `AgentInfo`, `CodeSession`, `PersistentMemory`, `Settings`, `Data`, `UpdateSettingsParams`

### Interfaces
- `omniagent.go` — `OmniAgent`, `OmniAgentEntrypoint`
- `store/omniagent.go` — `OmniAgentStore`
- `store/session.go` — `CodeSessionStore`

### Database
- `database/db.go` — SQLite singleton (`GetDB`)
- `database/schema/schema.sql` — table schemas (`agents`, `code_sessions`, `agent_settings`)
