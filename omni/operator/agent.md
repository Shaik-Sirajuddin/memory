## operator

> Do not read implementation files unless explicitly asked to.

### Data Structs
- `operator.go` — `TeamInfo`, `GetCodeAgentsParams`, `GetAgentsResult`, `CreateAgentParams`, `DeleteAgentParams`, `ListWorkspacesParams`, `ListWorkspacesResult`, `GetWorkSpaceParams`, `GetTeamResult`, `ForkAgentParams`, `DisocveryResult`

### Interfaces
- `operator.go` — `Operator`
- `store.go` — `OperatorStore`

### Factory Functions
- `impl.go` — `New() (Operator, error)` — creates Operator backed by shared omniagent DB
- `store.go` — `GetOperatorStore() (OperatorStore, error)` — singleton store factory

### Database
- `store.go` — reuses `omniagent/database.GetDB()` singleton; migrates `workspaces` table on first call
- Owned table: `workspaces` (`id`, `name`, `workspace_dir`)
- Shared table (read/write): `agents` — owned by omniagent, joined via `workspace_dir`
