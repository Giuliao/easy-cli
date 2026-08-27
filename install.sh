#!/usr/bin/env sh
set -eu

REPO="Giuliao/easy-cli"
BINARY="easy"

# Allow overriding the install directory.
INSTALL_DIR="${EASY_INSTALL_DIR:-}"
if [ -z "$INSTALL_DIR" ]; then
  if [ "$(id -u)" = "0" ]; then
    INSTALL_DIR="/usr/local/bin"
  else
    INSTALL_DIR="$HOME/.local/bin"
  fi
fi

# Allow pinning a specific version; defaults to latest.
VERSION="${EASY_VERSION:-latest}"

log() {
  printf '%s\n' "$*" >&2
}

err() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

command -v curl >/dev/null 2>&1 || err "curl is required"
command -v shasum >/dev/null 2>&1 || command -v sha256sum >/dev/null 2>&1 || err "shasum or sha256sum is required"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "$os" in
  linux) os="linux" ;;
  darwin) os="darwin" ;;
  *) err "unsupported OS: $os" ;;
esac

case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) err "unsupported architecture: $arch" ;;
esac

ext=""
if [ "$os" = "windows" ]; then ext=".exe"; fi

asset="${BINARY}-${os}-${arch}${ext}"

if [ "$VERSION" = "latest" ]; then
  url="https://github.com/${REPO}/releases/latest/download/${asset}"
  checksum_url="https://github.com/${REPO}/releases/latest/download/${asset}.sha256"
else
  url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"
  checksum_url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}.sha256"
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

log "downloading ${asset}..."
curl -fsSL "$url" -o "${tmpdir}/${asset}" || err "failed to download ${asset}"

log "verifying checksum..."
curl -fsSL "$checksum_url" -o "${tmpdir}/${asset}.sha256" || err "failed to download checksum"
( cd "$tmpdir" && (shasum -a 256 -c "${asset}.sha256" 2>/dev/null || sha256sum -c "${asset}.sha256") ) || err "checksum verification failed"

mkdir -p "$INSTALL_DIR"
cp "${tmpdir}/${asset}" "${INSTALL_DIR}/${BINARY}"
chmod 0755 "${INSTALL_DIR}/${BINARY}" || err "failed to chmod ${INSTALL_DIR}/${BINARY}"

log "installed ${BINARY} to ${INSTALL_DIR}/${BINARY}"
if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
  log "note: ${INSTALL_DIR} is not in your PATH; add it to use '${BINARY}' directly"
fi
