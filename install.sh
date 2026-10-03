#!/usr/bin/env bash
# Installs the latest md release: curl -fsSL <release>/install.sh | bash
set -euo pipefail

REPO="eureka-corp/md-cli"
BINARY="md"
INSTALL_DIR="${MD_INSTALL_DIR:-$HOME/.local/bin}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux|darwin) ;;
  *) echo "Unsupported OS: $os" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

tag="${MD_VERSION:-}"
if [ -z "$tag" ]; then
  # Parsed in bash: piping curl into a reader that stops early fails under pipefail.
  json=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest")
  if [[ $json =~ \"tag_name\":[[:space:]]*\"([^\"]+)\" ]]; then tag=${BASH_REMATCH[1]}; fi
fi
[ -n "$tag" ] || { echo "Could not determine the latest version" >&2; exit 1; }
archive="${BINARY}_${tag#v}_${os}_${arch}.tar.gz"
base="https://github.com/${REPO}/releases/download/${tag}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading ${BINARY} ${tag} for ${os}/${arch}…"
curl -fsSL "${base}/${archive}" -o "${tmp}/${archive}"
curl -fsSL "${base}/checksums.txt" -o "${tmp}/checksums.txt"

expected=$(grep " ${archive}\$" "${tmp}/checksums.txt" | cut -d' ' -f1)
if command -v sha256sum >/dev/null 2>&1; then actual=$(sha256sum "${tmp}/${archive}" | cut -d' ' -f1)
else actual=$(shasum -a 256 "${tmp}/${archive}" | cut -d' ' -f1); fi
if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
  echo "Checksum mismatch for ${archive}" >&2
  exit 1
fi

tar -xzf "${tmp}/${archive}" -C "$tmp" "$BINARY"
mkdir -p "$INSTALL_DIR"
install -m 0755 "${tmp}/${BINARY}" "${INSTALL_DIR}/${BINARY}"

echo "Installed ${BINARY} ${tag} to ${INSTALL_DIR}/${BINARY}"
case ":$PATH:" in *":${INSTALL_DIR}:"*) ;; *) echo "Add ${INSTALL_DIR} to your PATH." ;; esac
