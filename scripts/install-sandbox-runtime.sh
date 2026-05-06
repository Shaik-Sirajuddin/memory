#!/usr/bin/env sh
set -eu

log() {
  printf '%s\n' "$*" >&2
}

has_cmd() {
  command -v "$1" >/dev/null 2>&1
}

install_file() {
  src="$1"
  dst="$2"
  dst_dir="$(dirname "$dst")"

  if [ -w "$dst_dir" ]; then
    install -m 0755 "$src" "$dst"
    return 0
  fi

  if has_cmd sudo; then
    sudo install -m 0755 "$src" "$dst"
    return 0
  fi

  log "permission denied for $dst_dir; rerun as root or install sudo"
  return 1
}

run_privileged() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
    return $?
  fi

  if has_cmd sudo; then
    sudo "$@"
    return $?
  fi

  log "command requires root privileges: $*"
  return 1
}

detect_os() {
  os="$(uname -s)"
  case "$os" in
    Linux)
      printf '%s' "linux"
      ;;
    Darwin)
      printf '%s' "darwin"
      ;;
    *)
      printf '%s' "unsupported"
      ;;
  esac
}

detect_arch() {
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64)
      printf '%s' "x86_64"
      ;;
    aarch64|arm64)
      printf '%s' "aarch64"
      ;;
    *)
      log "unsupported architecture: $arch"
      return 1
      ;;
  esac
}

install_gvisor() {
  if has_cmd runsc; then
    log "runsc already installed: $(command -v runsc)"
    return 0
  fi

  arch="$(detect_arch)"
  bin_dir="${BIN_DIR:-/usr/local/bin}"
  runsc_url="${RUNSC_URL:-https://storage.googleapis.com/gvisor/releases/release/latest/${arch}/runsc}"
  tmp_dir="${TMPDIR:-/tmp}"
  tmp_file="${tmp_dir}/runsc.$$"

  trap 'rm -f "$tmp_file"' EXIT INT TERM
  log "downloading runsc from ${runsc_url}"

  if has_cmd curl; then
    curl -fsSL "$runsc_url" -o "$tmp_file"
  elif has_cmd wget; then
    wget -qO "$tmp_file" "$runsc_url"
  else
    log "curl or wget is required to download runsc"
    return 1
  fi

  install_file "$tmp_file" "${bin_dir}/runsc"
  if has_cmd runsc; then
    log "runsc installed: $(command -v runsc)"
    return 0
  fi

  log "runsc installed to ${bin_dir}/runsc; ensure ${bin_dir} is in PATH"
}

install_uidmap_tools() {
  if has_cmd newuidmap && has_cmd newgidmap; then
    log "uidmap helpers present: $(command -v newuidmap), $(command -v newgidmap)"
    return 0
  fi

  log "installing uidmap helpers (newuidmap/newgidmap)"

  if has_cmd apt-get; then
    run_privileged apt-get update
    run_privileged apt-get install -y uidmap
  elif has_cmd dnf; then
    run_privileged dnf install -y shadow-utils
  elif has_cmd yum; then
    run_privileged yum install -y shadow-utils
  elif has_cmd pacman; then
    run_privileged pacman -Sy --noconfirm shadow
  elif has_cmd zypper; then
    run_privileged zypper --non-interactive install shadow
  elif has_cmd apk; then
    run_privileged apk add shadow
  else
    log "unsupported package manager; install newuidmap/newgidmap manually"
    return 1
  fi

  if has_cmd newuidmap && has_cmd newgidmap; then
    log "uidmap helpers installed: $(command -v newuidmap), $(command -v newgidmap)"
    return 0
  fi

  log "uidmap helper install completed but newuidmap/newgidmap still not found in PATH"
  return 1
}

fix_uidmap_permissions() {
  uidmap_bin="$(command -v newuidmap)"
  gidmap_bin="$(command -v newgidmap)"

  log "fixing uidmap helper ownership/permissions"
  run_privileged chown root:root "$uidmap_bin" "$gidmap_bin"
  run_privileged chmod 4755 "$uidmap_bin" "$gidmap_bin"
}

ensure_subid_entry() {
  file="$1"
  user="$2"
  default_range="${3:-100000:65536}"

  if [ ! -f "$file" ]; then
    run_privileged sh -c "touch '$file'"
  fi

  if grep -q "^${user}:" "$file"; then
    log "${file} already has entry for ${user}"
    return 0
  fi

  log "adding ${user} mapping to ${file}"
  run_privileged sh -c "printf '%s\\n' '${user}:${default_range}' >> '$file'"
}

configure_rootless_uidmap() {
  if [ "$(id -u)" -eq 0 ]; then
    user_name="${SUDO_USER:-}"
    if [ -z "$user_name" ]; then
      log "running as root without SUDO_USER; skipping subuid/subgid user mapping setup"
      return 0
    fi
  else
    user_name="$(id -un)"
  fi

  fix_uidmap_permissions
  ensure_subid_entry /etc/subuid "$user_name"
  ensure_subid_entry /etc/subgid "$user_name"
}

verify_seatbelt() {
  if has_cmd sandbox-exec || [ -x "/usr/bin/sandbox-exec" ]; then
    log "seatbelt available"
    return 0
  fi
  log "seatbelt (sandbox-exec) not found on this macOS system"
  return 1
}

main() {
  os_name="$(detect_os)"
  case "$os_name" in
    linux)
      install_gvisor
      install_uidmap_tools
      configure_rootless_uidmap
      ;;
    darwin)
      verify_seatbelt
      ;;
    *)
      log "unsupported OS for sandbox runtime installation: $(uname -s)"
      return 1
      ;;
  esac
}

main "$@"
