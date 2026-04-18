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
