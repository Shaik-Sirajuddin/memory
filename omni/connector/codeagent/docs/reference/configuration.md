# Configuration Reference

## Config file paths by provider

### Claude Code

| Scope | Path | Shared with team |
|---|---|---|
| Managed — macOS | `/Library/Application Support/ClaudeCode/managed-settings.json` | Yes — IT deployed |
| Managed — Linux | `/etc/claude-code/managed-settings.json` | Yes — IT deployed |
| Managed — Windows | `C:\Program Files\ClaudeCode\managed-settings.json` | Yes — IT deployed |
| User | `~/.claude/settings.json` | No |
| Project | `.claude/settings.json` | Yes — committed to git |
| Local | `.claude/settings.local.json` | No — gitignored |
| MCP servers (user) | `~/.claude.json` | No |
| MCP servers (project) | `.mcp.json` | Yes — committed to git |
| Memory / instructions | `~/.claude/CLAUDE.md` or `CLAUDE.md` | Yes — project level |

**Precedence (highest → lowest):** Managed → CLI flags → Local → Project → User

---

### Gemini CLI

| Scope | Path |
|---|---|
| System override — Linux | `/etc/gemini-cli/settings.json` |
| System override — macOS | `/Library/Application Support/GeminiCli/settings.json` |
| User | `~/.gemini/settings.json` |
| Project | `.gemini/settings.json` |
| System defaults — Linux | `/etc/gemini-cli/system-defaults.json` |
| Memory / instructions | `GEMINI.md` or `.gemini/GEMINI.md` |
| Custom slash commands | `~/.gemini/commands/*.toml` or `.gemini/commands/*.toml` |

**Precedence (highest → lowest):** System override → Env vars / .env → Project → User → System defaults → Hardcoded

---

### Codex

| Scope | Path | Notes |
|---|---|---|
| User | `~/.codex/config.yaml` | Global defaults |
| Project | `.codex/config.yaml` | Written on `UpdateSessionSandbox` |

**Two-way sync:** Calling `UpdateSessionSandbox` writes the resolved sandbox mode into `.codex/config.yaml` in the active working directory so interactive sessions launched later pick it up automatically.

---

## Environment variables

### Claude Code

| Variable | Purpose |
|---|---|
| `ANTHROPIC_API_KEY` | Authentication key for Anthropic API |
| `ANTHROPIC_MODEL` | Override the default model |
| `ANTHROPIC_SMALL_FAST_MODEL` | Override the small / fast model |
| `CLAUDE_CODE_MAX_OUTPUT_TOKENS` | Cap maximum output tokens per response |
| `CLAUDE_CODE_ENABLE_TELEMETRY` | Enable telemetry — set to `1` |
| `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` | Disable telemetry and auto-updates — set to `1` |
| `CLAUDE_CODE_DEBUG_LOGS_DIR` | Directory where debug logs are written |
| `CLAUDE_CODE_SIMPLE` | Set automatically when `--bare` mode is active |
| `OTEL_METRICS_EXPORTER` | OpenTelemetry metrics exporter target |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP collector endpoint URL |

### Gemini CLI

| Variable | Purpose |
|---|---|
| `GEMINI_API_KEY` | Gemini API authentication key |
| `GOOGLE_API_KEY` | Google Cloud API key (Vertex AI express mode) |
| `GOOGLE_APPLICATION_CREDENTIALS` | Path to service account JSON file |
| `GOOGLE_CLOUD_PROJECT` | GCP project ID |
| `GOOGLE_CLOUD_LOCATION` | GCP location (e.g. `us-central1`) |
| `GEMINI_MODEL` | Override default model |
| `GEMINI_SANDBOX` | Sandbox execution mode |
| `GEMINI_TELEMETRY_ENABLED` | Enable telemetry |
| `GEMINI_CLI_HOME` | Root directory for user config |
| `NO_COLOR` | Disable all color output |
| `DEBUG` | Enable verbose logging |
| `GEMINI_CLI_SURFACE` | Custom User-Agent label for API tracking |

### Codex

| Variable | Purpose |
|---|---|
| `OPENAI_API_KEY` | Required for all API access and for `FetchModels` |

---

## Settings schema references

| Provider | Schema URL |
|---|---|
| Claude Code | `https://json.schemastore.org/claude-code-settings.json` |
| Gemini CLI | `https://raw.githubusercontent.com/google-gemini/gemini-cli/main/schemas/settings.schema.json` |
| Codex | `.codex/config.yaml` (YAML, no published schema) |
