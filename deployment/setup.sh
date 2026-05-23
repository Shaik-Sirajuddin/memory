#!/usr/bin/env bash
# setup.sh — bundled inside each release tarball
set -euo pipefail

OMNI_PREFIX="${OMNI_PREFIX:-/opt/omni}"
OMNI_BIN="$OMNI_PREFIX/bin"
SYMLINK_DIR="/usr/local/bin"
SERVICE_TEMPLATE="omni@"
SERVICE_FILE="/etc/systemd/system/omni@.service"
TARGET_USER="${SUDO_USER:-${USER:-$(id -un)}}"
SERVICE_NAME="omni@${TARGET_USER}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${BIN_DIR:-$SCRIPT_DIR/bin}"

BINARIES=(omni omni-server)

need_root() {
  if [[ "$EUID" -ne 0 ]]; then
    echo "error: run as root (sudo $0)" >&2
    exit 1
  fi
}

install_binaries() {
  echo "==> installing binaries to $OMNI_BIN"
  mkdir -p "$OMNI_BIN"
  for bin in "${BINARIES[@]}"; do
    install -m 755 "$BIN_DIR/$bin" "$OMNI_BIN/$bin"
  done
}

link_binaries() {
  echo "==> symlinking into $SYMLINK_DIR"
  mkdir -p "$SYMLINK_DIR"
  for bin in "${BINARIES[@]}"; do
    ln -sf "$OMNI_BIN/$bin" "$SYMLINK_DIR/$bin"
    echo "    $SYMLINK_DIR/$bin -> $OMNI_BIN/$bin"
  done
}

setup_user() {
  echo "==> creating omni system group and user"
  /usr/sbin/groupadd -f omni
  if ! id -u omni &>/dev/null; then
    /usr/sbin/useradd -r -g omni -s /sbin/nologin -d /var/lib/omni-pty -M omni
  fi
  # add the invoking user to the omni group so they can reach the socket
  if [[ -n "${SUDO_USER:-}" ]]; then
    /usr/sbin/usermod -aG omni "$SUDO_USER"
    echo "    added $SUDO_USER to group omni (re-login required)"
  fi
}

write_service() {
  echo "==> writing systemd service template $SERVICE_FILE"
  local debug_env=""
  if [[ "${DEBUG:-0}" == "1" ]]; then
    debug_env=$'\nEnvironment=DEV=1'
    echo "    DEBUG=1: enabling slog debug logging (DEV=1)"
  fi
  cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=Omni PTY daemon for %i
After=network.target

[Service]
Type=simple
User=%i
ExecStart=/opt/omni/bin/omni-server
Restart=on-failure
RestartSec=3s
RuntimeDirectory=omni-%i
RuntimeDirectoryMode=0700
StateDirectory=omni-%i
StateDirectoryMode=0700
Environment=OMNI_PTY_SOCKET=/run/omni-%i/omni-pty.sock
Environment=PTYDAEMON_DB=/var/lib/omni-%i/ptydaemon.db
Environment=HOOK_OPERATOR_SOCKET=/run/omni-%i/hook-operator.sock
Environment=PATH=/usr/local/bin:/usr/bin:/bin:/home/%i/.local/bin${debug_env}
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF
}

check_agent_binaries() {
  echo "==> checking agent binaries are system-wide"
  local missing=()
  for bin in claude codex gemini; do
    if ! /usr/bin/which "$bin" &>/dev/null && ! [[ -x "/usr/local/bin/$bin" ]]; then
      missing+=("$bin")
    fi
  done
  if [[ ${#missing[@]} -gt 0 ]]; then
    echo "    WARNING: the following agent binaries were not found in system PATH:" >&2
    for bin in "${missing[@]}"; do
      echo "      - $bin  (install system-wide, e.g. sudo npm install -g @anthropic-ai/claude-code)" >&2
    done
    echo "    Install them system-wide or ensure they are in ~/.local/bin for the target user." >&2
  else
    echo "    all agent binaries found"
  fi
}

reload_and_enable() {
  systemctl daemon-reload
  if systemctl is-active --quiet "$SERVICE_NAME"; then
    echo "==> restarting $SERVICE_NAME"
    systemctl restart "$SERVICE_NAME"
  else
    echo "==> enabling and starting $SERVICE_NAME"
    systemctl enable --now "$SERVICE_NAME"
  fi
  echo "==> $SERVICE_NAME status:"
  systemctl status "$SERVICE_NAME" --no-pager || true
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  need_root
  install_binaries
  link_binaries
  write_service
  check_agent_binaries
  reload_and_enable
  echo "==> setup complete — 'omni' is ready for user ${TARGET_USER}"
fi
