
[gemini]
interface_base = cli_go/connector/codeagent/gemini
interface_files:
- gemini.go
- commands.go
- parser.go
- hooks.go
- sandbox.go
- settings_resolver.go

[claude]
interface_base = cli_go/connector/codeagent/claude
interface_files:
- claude.go
- commands.go
- parser.go
- sandbox.go
- settings/resolver.go
- settings/watch_linux.go
- settings/watch_darwin.go
- settings/watch_other.go
- log/log.go
- settings.gen.go   # generated — do not edit

structs:
  claudeAgent:        # private — implements codeagent.CodeAgent
    file: claude.go
    embeds: [*ClaudeParser]
    fields:
      resolver:        settings.Resolver   # SettingsResolver delegation
      workDir:         string
      model:           string
      permMode:        codeagent.PermissionMode
      systemPrompt:    string
      sessionID:       string
      sbx:             *sandbox.Config
      info:            codeagent.CodeAgentInfo
      registeredHooks: []*hooks.HookData

  ClaudeParser:       # public — implements hooks.HookIOParser via embedding
    file: parser.go
    note: all 14 HookIOParser methods delegate to parseHookInput[T]

  Resolver:           # public — implements codeagent.SettingsResolver
    file: settings/resolver.go
    fields:
      provider:   codeagent.Provider
      watchMu:    sync.Mutex
      watchStop:  context.CancelFunc

  ConfigPaths:        # public — package-level search paths config
    file: claude.go
    fields:
      GlobalConfigDirs:    []string
      WorkspaceConfigDirs: []string

interfaces_implemented:
  codeagent.CodeAgent:
    via: claudeAgent (claude.go + commands.go + parser.go + sandbox.go)
    groups:
      identity:          [Info, GetUserIdentity]
      capabilities:      [Capabilities]
      session_lifecycle: [Create, Resume, List, Delete, Stop]
      execution:         [Exec, Stream, GetSessionConfig]
      sandbox:           [GetSessionSandbox, UpdateSessionSandbox]
      hooks_manager:     [SupportedHooks, Register, GetRegisteredHooks, DeleteHook]
      hooks_io_parser:   [*ClaudeParser — 14 methods]
      settings_resolver: [GetUserSettings, GetWorkspaceSettings, SaveDefaultSettings, WatchDefaultSettings]

  codeagent.SettingsResolver:
    via: settings.Resolver (settings/resolver.go)
    methods:
      GetUserSettings()                          # reads first-resolved ~/.claude/settings.json
      GetWorkspaceSettings(sandbox.WorkspaceDir) # reads <dir>/.claude/settings.json
      SaveDefaultSettings(*Settings)             # round-trip merge into user settings.json
      WatchDefaultSettings(func(*Settings))      # native OS watcher; replaces previous watcher

key_functions:
  claude.go:
    New(workDir, model string) codeagent.CodeAgent  # entry point; verifies binary + auth

  commands.go:
    buildExecArgs(...)  []string   # claude -p ... --output-format
    buildStreamArgs(...)[]string   # claude -p ... --output-format stream-json
    Exec(ExecuteParams)            # runs claude -p to completion
    Stream(StreamParams)           # goroutine reading stream-json lines → StreamEvent channel

  parser.go:
    parseClaudeLine(line string) StreamEvent  # maps Anthropic stream-json events → StreamEvent
    parseAuthStatus(raw string) UserIdentify  # parses claude auth status JSON
    parseHookInput[T](raw any) (*T, error)    # generic JSON round-trip for all hook types

  sandbox.go:
    syncHooksToSettings(workDir, hooks)       # writes hooks into .claude/settings.json
    hookDataToEntry(h *HookData) entry        # maps abstract HookData → claude hook JSON shape
    hookEventName map[HookID]string           # abstract hook ID → Claude event key (e.g. PreToolUse → "PreToolUse")

  settings/resolver.go:
    UserSettingsPath() (string, error)        # first-resolved user settings.json path
    WorkspaceSettingsPath(dir) string         # <dir>/.claude/settings.json
    userSettingsCandidates []string           # package-level ordered candidate relative paths
    mergeAndWrite(path, Settings)             # preserves unknown keys on save
    convert(rawFile) *Settings                # rawFile → codeagent.Settings

  settings/watch_linux.go:
    osWatch(ctx, path, onChange)  # inotify: IN_CLOSE_WRITE | IN_MOVED_TO | IN_CREATE on parent dir

  settings/watch_darwin.go:
    osWatch(ctx, path, onChange)  # kqueue EVFILT_VNODE on parent dir: NOTE_WRITE|NOTE_ATTRIB|NOTE_RENAME

  settings/watch_other.go:
    osWatch(ctx, path, onChange)  # polling fallback: 500 ms mtime check

constants_and_vars:
  claude.go:
    Claude     Provider = "claude"   # in commands.go
    DefaultModel        = ModelSonnet4
    StaticModels        = [ModelOpus4, ModelSonnet4, ModelHaiku45]
    Config ConfigPaths               # search-path config instance
  sandbox.go:
    hookEventName map[HookID]string  # hook ID → Claude settings.json event key
