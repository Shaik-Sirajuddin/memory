## omniagent

> Do not read implementation files unless explicitly asked to.

### Data Structs

**`omniagent.go`**
| Struct | Fields |
|---|---|
| `AgentInfo` | `ID string`, `Name string`, `WorkspaceDir sandbox.WorkspaceDir`, `MemoryDir string` |
| `CodeSession` | `Id string`, `Model *codeagent.Model`, `Idx int`, `IsActive bool`, `Prompts int`, `LastSyncPrompt int` |
| `PersistentMemory` | _(empty — agent write memory placeholder)_ |
| `Settings` | embeds `config.Settings`, `Sandbox *sandbox.Config`, `DefaultModel *codeagent.Model`, embeds `hooks.Capabilities` |
| `Data` | `Info *AgentInfo`, `ActiveWorkSpace *sandbox.Config`, `ActiveSession *CodeSession`, `Settings *Settings`, `Sessions []*CodeSession` |
| `ConfigPaths` | `GlobalConfigDirs []string`, `WorkspaceConfigDirs []string` |

**`config/settings.go`**
| Struct | Fields |
|---|---|
| `OmniConfig` | `Dev DevConfig` |
| `DevConfig` | `Debug bool` |
| `Settings` | `Model *string`, `Timeout *float64`, `MaxTurns *float64`, `Sandbox *string`, `Env map[string]string`, `Hooks map[string][]HookEntry`, `Theme *string`, `Cwd *string`, `LogPrompts *bool` |
| `HookEntry` | `Command *string`, `Args []string`, `Timeout *float64`, `Url *string` |

**`config/logger.go`**
| Function | Signature |
|---|---|
| `NewLogger` | `NewLogger(cfg OmniConfig) *slog.Logger` — `Debug=true`→`LevelDebug`, else `LevelInfo` |

**`store/omniagent.go`**
| Struct | Fields |
|---|---|
| `ListAgentParams` | `Workspace sandbox.WorkspaceDir` |

---

### Interfaces

**`omniagent.go`**
| Interface | Methods |
|---|---|
| `OmniAgentActions` | `New()`, `SyncMemory()`, `NewCodeSession()` |
| `OmniAgent` | embeds `codeagent.CodeAgent`, embeds `OmniAgentActions`, `Data() *Data` |
| `OmniAgentEntrypoint` | `PreToolUse()`, `PrePrompt()`, `PostPrompt()`, `PostToolUse()`, `PreSessionStart()`, `PostSessionStart()` |

**`store/omniagent.go`**
| Interface | Methods |
|---|---|
| `OmniAgentStore` | `Save(*Data) error`, `Create(*Data) error`, `GetAgent(ID) (*Data, error)`, `GetActiveSession(ID) (*CodeSession, error)`, `UpdateActiveSession(ID, *CodeSession) error`, `CreateSession(ID, *CodeSession) error`, `GetSettings(ID) (*Settings, error)`, `UpdateSettings(ID, *Settings) error`, `ListAgents(ListAgentParams)`, `DeleteAgent()` |

**`store/session.go`**
| Interface | Methods |
|---|---|
| `CodeSessionStore` | `GetSession(agentID) (*CodeSession, error)`, `CreateSession(agentID, *CodeSession) error`, `UpdateSession(agentID, *CodeSession) error`, `ListSessions(agentID, filter *CodeSession) ([]*CodeSession, error)` |

---

### Database

- `database/db.go` — SQLite singleton (`GetDB`)

---

### Constants & Vars

**`omniagent.go`**
| Name | Value |
|---|---|
| `AGENTS_ROOT_DIR` | `"/agents"` |
| `CONFIG_FILE` | `"/config.json"` |
| `Config` | `ConfigPaths{ GlobalConfigDirs: [".omni"], WorkspaceConfigDirs: [".omni"] }` |

---

### File Map

```
omniagent/
├── omniagent.go          — core structs, interfaces, constants
├── config/
│   ├── settings.go       — OmniConfig, DevConfig, Settings, HookEntry structs
│   └── logger.go         — NewLogger(OmniConfig) — level from Dev.Debug
├── store/
│   ├── omniagent.go      — OmniAgentStore interface + ListAgentParams
│   ├── session.go        — CodeSessionStore interface
│   ├── omniagentstore.go — OmniAgentStore implementation
│   └── codesessionstore.go — CodeSessionStore implementation
└── database/
    └── db.go             — SQLite singleton
```
