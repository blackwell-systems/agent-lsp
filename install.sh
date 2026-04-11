#!/bin/sh
# agent-lsp installer
# Usage: curl -fsSL https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.sh | sh
set -e

REPO="blackwell-systems/agent-lsp"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BINARY="agent-lsp"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  darwin) OS="darwin" ;;
  linux)  OS="linux" ;;
  *)      echo "error: unsupported OS: $OS" >&2; exit 1 ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64)   ARCH="amd64" ;;
  aarch64|arm64)  ARCH="arm64" ;;
  *)              echo "error: unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

# Fetch latest release metadata from GitHub
echo "Fetching latest release..."
RELEASE_JSON=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest")
TAG=$(echo "$RELEASE_JSON" | grep '"tag_name"' | head -1 | cut -d'"' -f4)

if [ -z "$TAG" ]; then
  echo "error: could not determine latest release tag" >&2
  exit 1
fi

# Find the matching asset URL from the release (handles any naming convention)
ASSET_URL=$(echo "$RELEASE_JSON" | grep '"browser_download_url"' | grep "${OS}_${ARCH}.tar.gz" | head -1 | cut -d'"' -f4)

if [ -z "$ASSET_URL" ]; then
  echo "error: no release asset found for ${OS}/${ARCH}" >&2
  echo "Available releases: https://github.com/${REPO}/releases/tag/${TAG}" >&2
  exit 1
fi

echo "Installing agent-lsp ${TAG} for ${OS}/${ARCH}..."

# Download and extract
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

curl -fsSL "$ASSET_URL" -o "${TMP_DIR}/agent-lsp.tar.gz"
tar -xzf "${TMP_DIR}/agent-lsp.tar.gz" -C "$TMP_DIR"

# Install binary
if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  sudo mv "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi
chmod +x "${INSTALL_DIR}/${BINARY}"

# Verify
VERSION=$("${INSTALL_DIR}/${BINARY}" --version 2>/dev/null || echo "unknown")
echo ""
echo "Installed agent-lsp ${VERSION} to ${INSTALL_DIR}/${BINARY}"
echo ""
echo "Run 'agent-lsp init' to configure your AI tool."
