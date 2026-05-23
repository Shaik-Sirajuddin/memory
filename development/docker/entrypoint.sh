#!/usr/bin/env bash
# entrypoint.sh — install pre-built omni, write configs, then hand off to systemd PID 1
set -euo pipefail

WORKSPACE="${WORKSPACE:-/workspace}"

# ── install + setup omni (binary already baked into image at /opt/omni/bin) ───
fix_claude_binary() {
  local native="/usr/lib/node_modules/@anthropic-ai/claude-code/node_modules/@anthropic-ai/claude-code-linux-x64/claude"
  if [[ -f "$native" ]]; then
    ln -sf "$native" /usr/bin/claude
  fi
}
fix_claude_binary

install_and_setup() {
  echo "==> install_phase"
  # shellcheck source=deployment/setup.sh
  source "$WORKSPACE/deployment/setup.sh"
  # binaries are already at /opt/omni/bin from image build — skip install_binaries
  link_binaries
  write_service

  # enable service so systemd starts it on boot
  mkdir -p /etc/systemd/system/multi-user.target.wants
  ln -sf /etc/systemd/system/omni@.service \
         /etc/systemd/system/multi-user.target.wants/omni@root.service
}

install_and_setup

# ── export runtime socket paths for all child processes (hooks, CLIs) ─────────
write_runtime_env() {
  # /etc/environment is PAM-only; write to profile.d so all bash sessions inherit it
  cat > /etc/profile.d/omni-sockets.sh <<'EOF'
export HOOK_OPERATOR_SOCKET=/run/omni-root/hook-operator.sock
export OMNI_PTY_SOCKET=/run/omni-root/omni-pty.sock
EOF
  # also source into root's interactive sessions immediately
  grep -q HOOK_OPERATOR_SOCKET /root/.bashrc 2>/dev/null || \
    echo 'source /etc/profile.d/omni-sockets.sh' >> /root/.bashrc
}
write_runtime_env

# ── systemd drop-in: enable HTTP transport for dev ────────────────────────────
write_mcp_dropin() {
  mkdir -p /etc/systemd/system/omni@root.service.d
  local conf="/etc/systemd/system/omni@root.service.d/dev-mcp.conf"
  printf '[Service]\n' > "$conf"
  # pure HTTP service binding
  if [[ -n "${AXO_LINK_SERVICE_HTTP_BIND:-}" ]]; then
    printf 'Environment=AXO_LINK_SERVICE_HTTP_BIND=%s\n' "$AXO_LINK_SERVICE_HTTP_BIND" >> "$conf"
  fi
  if [[ -n "${AXO_LINK_SERVICE_UNIX_SOCKET:-}" ]]; then
    printf 'Environment=AXO_LINK_SERVICE_UNIX_SOCKET=%s\n' "$AXO_LINK_SERVICE_UNIX_SOCKET" >> "$conf"
  fi
  if [[ -n "${AXO_LINK_SERVICE_ADDR:-}" ]]; then
    printf 'Environment=AXO_LINK_SERVICE_ADDR=%s\n' "$AXO_LINK_SERVICE_ADDR" >> "$conf"
  fi
  # MCP streamable HTTP server
  if [[ -n "${AXO_LINK_MCP_ADDR:-}" ]]; then
    printf 'Environment=AXO_LINK_MCP_ADDR=%s\n' "$AXO_LINK_MCP_ADDR" >> "$conf"
  fi
}
write_mcp_dropin

# ── seed MCP configs into volumes (write-if-absent) ──────────────────────────
seed_mcp_configs() {
  local url="http://127.0.0.1:18062/mcp"

  # claude — settings.json with omni hooks
  # Rewrite if absent OR if stale top-level hook keys (PreSessionStart/PostSessionStart) are present
  local needs_seed=0
  [[ ! -f /root/.claude/settings.json ]] && needs_seed=1
  if [[ $needs_seed -eq 0 ]] && python3 -c "
import json,sys
d=json.load(open('/root/.claude/settings.json'))
stale={'PreSessionStart','PostSessionStart'}
sys.exit(0 if stale & set(d.get('hooks',{})) else 1)
" 2>/dev/null; then
    echo "==> rewriting /root/.claude/settings.json (stale hook keys detected)"
    needs_seed=1
  fi
  if [[ $needs_seed -eq 1 ]]; then
    echo "==> seeding /root/.claude/settings.json"
    mkdir -p /root/.claude
    cat > /root/.claude/settings.json <<'EOF'
{
  "hooks": {
    "PostToolUse": [{"hooks": [{"type": "command","command": "/usr/local/bin/omni hook --event PostToolUse"}]}],
    "PostToolUseFailure": [{"hooks": [{"type": "command","command": "/usr/local/bin/omni hook --event PostToolUseFailure"}]}],
    "PreToolUse": [{"hooks": [{"type": "command","command": "/usr/local/bin/omni hook --event PreToolUse"}]}],
    "SessionStart": [
      {"hooks": [{"type": "command","command": "/usr/local/bin/omni hook --event PreSessionStart"}]},
      {"hooks": [{"type": "command","command": "/usr/local/bin/omni hook --event PostSessionStart"}]}
    ],
    "Stop": [{"hooks": [{"type": "command","command": "/usr/local/bin/omni hook --event Stop"}]}],
    "UserPromptSubmit": [{"hooks": [{"type": "command","command": "/usr/local/bin/omni hook --event UserPromptSubmit"}]}]
  },
  "permissions": {
    "allow": ["mcp__tunnel-mcp__*"]
  },
  "theme": "dark"
}
EOF
  fi

  # claude — /root/.claude.json (MCP config + onboarding bypass, inside agent-claude volume)
  if [[ ! -f /root/.claude.json ]]; then
    echo "==> seeding /root/.claude.json"
    local api_key_suffix=""
    if [[ -n "${ANTHROPIC_API_KEY:-}" ]]; then
      api_key_suffix="${ANTHROPIC_API_KEY: -20}"
    fi
    cat > /root/.claude.json <<EOF
{
  "mcpServers": {
    "tunnel-mcp": {
      "type": "http",
      "url": "${url}",
      "headers": {
        "Authorization": "Bearer \${AXO_LINK_MCP_AUTH_TOKEN}",
        "X-Sender-ID": "\${AXO_LINK_MCP_SENDER_ID}",
        "X-Sender-Type": "\${AXO_LINK_MCP_SENDER_TYPE}",
        "X-Agent-Workspace": "\${AXO_LINK_MCP_AGENT_WORKSPACE}"
      }
    }
  },
  "customApiKeyResponses": {
    "approved": ["${api_key_suffix}"],
    "rejected": []
  },
  "hasCompletedOnboarding": true,
  "hasTrustDialogHooksAccepted": true,
  "shiftEnterKeyBindingInstalled": true,
  "theme": "dark"
}
EOF
  fi

  # claude — accept workspace trust + project onboarding for /build (write-once)
  if [[ -f /usr/bin/claude ]] && ! claude config get hasTrustDialogAccepted 2>/dev/null | grep -q true; then
    echo "==> accepting claude workspace trust for /build"
    cd /build && claude config set hasTrustDialogAccepted true 2>/dev/null || true
    cd /build && claude config set hasCompletedProjectOnboarding true 2>/dev/null || true
  fi

  # codex — /root/.codex/config.toml (inside agent-codex volume)
  if [[ ! -f /root/.codex/config.toml ]]; then
    echo "==> seeding /root/.codex/config.toml"
    mkdir -p /root/.codex
    cat > /root/.codex/config.toml <<EOF
[mcp_servers.tunnel-mcp]
url = "${url}"
enabled = true
bearer_token_env_var = "AXO_LINK_MCP_AUTH_TOKEN"

[mcp_servers.tunnel-mcp.env_http_headers]
"X-Sender-ID"       = "AXO_LINK_MCP_SENDER_ID"
"X-Sender-Type"     = "AXO_LINK_MCP_SENDER_TYPE"
"X-Agent-Workspace" = "AXO_LINK_MCP_AGENT_WORKSPACE"
EOF
  fi

  # gemini — /root/.gemini/settings.json (inside agent-gemini volume)
  if [[ ! -f /root/.gemini/settings.json ]]; then
    echo "==> seeding /root/.gemini/settings.json"
    mkdir -p /root/.gemini
    cat > /root/.gemini/settings.json <<EOF
{
  "mcpServers": {
    "tunnel-mcp": {
      "httpUrl": "${url}",
      "headers": {
        "Authorization": "Bearer \$AXO_LINK_MCP_AUTH_TOKEN",
        "X-Sender-ID": "\$AXO_LINK_MCP_SENDER_ID",
        "X-Sender-Type": "\$AXO_LINK_MCP_SENDER_TYPE",
        "X-Agent-Workspace": "\$AXO_LINK_MCP_AGENT_WORKSPACE"
      }
    }
  }
}
EOF
  fi
}
seed_mcp_configs

# ensure DB is writable (volume may carry 644 from a previous run)
chmod -f 660 /var/lib/omni-root/ptydaemon.db 2>/dev/null || true

# ── warn about unconfigured agents ────────────────────────────────────────────
warn_missing_keys() {
  local warned=0
  if [[ -z "${ANTHROPIC_API_KEY:-}" ]]; then echo "  WARNING: ANTHROPIC_API_KEY not set — claude will not authenticate"; warned=1; fi
  if [[ -z "${OPENAI_API_KEY:-}"    ]]; then echo "  WARNING: OPENAI_API_KEY not set    — codex will not authenticate";  warned=1; fi
  if [[ -z "${GEMINI_API_KEY:-}"    ]]; then echo "  WARNING: GEMINI_API_KEY not set    — gemini will not authenticate"; warned=1; fi
  if [[ $warned -eq 1 ]]; then echo "  → set missing keys in development/.env.docker (see .env.docker.example)"; fi
}
warn_missing_keys

echo "==> handing off to systemd..."
exec /sbin/init
