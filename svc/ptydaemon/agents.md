## ptydaemon

> Do not read implementation files unless explicitly asked to.

### Data Structs
- `internal/terminal.go` — `PTYCreateParams`, `PTYTerminalInfo`, `PTYTerminal`, `Status`

### Interfaces
- `internal/daemon.go` — `PTYDaemon` (Create, Pipe, Exec, Stop, List, Shutdown)

### Factory Functions
- `internal/daemon.go` — `NewDaemon(store *Store) PTYDaemon`
- `internal/store.go` — `NewStore(dbPath string) (*Store, error)`
- `internal/server.go` — `NewHandler(d PTYDaemon) http.Handler`

### IPC
- HTTP/JSON over Unix socket at `PTYDAEMON_SOCKET` (default `/tmp/ptydaemon.sock`)
- Routes: `POST /create`, `POST /pipe`, `POST /exec`, `POST /stop`, `GET /list`

### Persistence
- `internal/store.go` — SQLite-backed `pty_sessions` store
- `migrations/001_pty_sessions.sql` — table schema

### Service
- `cmd/main.go` — daemon entrypoint, SIGTERM graceful shutdown
- `ptydaemon.service.template` — systemd unit template
