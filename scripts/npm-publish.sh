#!/bin/bash
# Publish @blackwell-systems/agent-lsp packages to npm.
# Downloads binaries from the GitHub release for TAG, injects them into
# platform packages, and publishes all packages.
#
# Usage: ./scripts/npm-publish.sh [TAG]
# TAG defaults to the latest git tag (e.g. v0.1.1)
#
# Requires: npm (authenticated), curl, tar, unzip, node

set -euo pipefail

TAG="${1:-$(git describe --tags --abbrev=0)}"
VERSION="${TAG#v}"
REPO="blackwell-systems/agent-lsp"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NPM_DIR="${SCRIPT_DIR}/../npm"

echo "Publishing @blackwell-systems/agent-lsp@${VERSION} (tag: ${TAG})"

# Mapping: goreleaser_key -> "npm_suffix:binary_name:archive_ext"
declare -A PLATFORMS=(
  ["darwin_arm64"]="darwin-arm64:agent-lsp:tar.gz"
  ["darwin_amd64"]="darwin-x64:agent-lsp:tar.gz"
  ["linux_arm64"]="linux-arm64:agent-lsp:tar.gz"
  ["linux_amd64"]="linux-x64:agent-lsp:tar.gz"
  ["windows_amd64"]="win32-x64:agent-lsp.exe:zip"
  ["windows_arm64"]="win32-arm64:agent-lsp.exe:zip"
)

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

for GOKEY in "${!PLATFORMS[@]}"; do
  IFS=: read -r NPM_SUFFIX BINARY_NAME ARCHIVE_EXT <<< "${PLATFORMS[$GOKEY]}"

  ARCHIVE="agent-lsp_${GOKEY}.${ARCHIVE_EXT}"
  URL="https://github.com/${REPO}/releases/download/${TAG}/${ARCHIVE}"
  PKG_DIR="${NPM_DIR}/agent-lsp-${NPM_SUFFIX}"
  BIN_DIR="${PKG_DIR}/bin"

  echo "  [${NPM_SUFFIX}] Downloading ${ARCHIVE}..."
  curl -fsSL "$URL" -o "${TMP_DIR}/${ARCHIVE}"

  mkdir -p "$BIN_DIR"

  if [ "$ARCHIVE_EXT" = "tar.gz" ]; then
    tar -xzf "${TMP_DIR}/${ARCHIVE}" -C "$TMP_DIR" "$BINARY_NAME"
  else
    unzip -o "${TMP_DIR}/${ARCHIVE}" "$BINARY_NAME" -d "$TMP_DIR"
  fi

  cp "${TMP_DIR}/${BINARY_NAME}" "${BIN_DIR}/${BINARY_NAME}"
  chmod +x "${BIN_DIR}/${BINARY_NAME}"
  rm -f "${TMP_DIR}/${BINARY_NAME}"

  # Update version
  node -e "
    const fs = require('fs');
    const p = '${PKG_DIR}/package.json';
    const pkg = JSON.parse(fs.readFileSync(p));
    pkg.version = '${VERSION}';
    fs.writeFileSync(p, JSON.stringify(pkg, null, 2) + '\n');
  "

  echo "  [${NPM_SUFFIX}] Publishing @blackwell-systems/agent-lsp-${NPM_SUFFIX}@${VERSION}..."
  npm publish "${PKG_DIR}" --access public
done

# Update root package version + optionalDependencies
ROOT_PKG="${NPM_DIR}/agent-lsp/package.json"
node -e "
  const fs = require('fs');
  const pkg = JSON.parse(fs.readFileSync('${ROOT_PKG}'));
  pkg.version = '${VERSION}';
  for (const dep of Object.keys(pkg.optionalDependencies)) {
    pkg.optionalDependencies[dep] = '${VERSION}';
  }
  fs.writeFileSync('${ROOT_PKG}', JSON.stringify(pkg, null, 2) + '\n');
"

echo "  [root] Publishing @blackwell-systems/agent-lsp@${VERSION}..."
npm publish "${NPM_DIR}/agent-lsp" --access public

echo ""
echo "Done. Install with: npm install -g @blackwell-systems/agent-lsp"
