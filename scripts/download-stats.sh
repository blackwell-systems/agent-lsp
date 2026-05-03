#!/usr/bin/env bash
# Queries PyPI, npm, GitHub Releases, and Docker Hub for cumulative download stats.
# Generates assets/download-stats.svg with a styled card.
#
# Usage: ./scripts/download-stats.sh [output-path]
# Default output: assets/download-stats.svg
set -euo pipefail

REPO="blackwell-systems/agent-lsp"
NPM_PKG="@blackwell-systems/agent-lsp"
PYPI_PKG="agent-lsp"
OUT="${1:-assets/download-stats.svg}"
CACHE="${OUT%.svg}.cache"

UA="agent-lsp-stats/1.0 (https://github.com/blackwell-systems/agent-lsp)"

# ── Fetch all-time totals ───────────────────────────────────────────

npm_total=$(curl -sf --max-time 10 "https://api.npmjs.org/downloads/point/2000-01-01:2030-01-01/${NPM_PKG}" \
  | python3 -c "import json,sys; print(json.load(sys.stdin)['downloads'])" 2>/dev/null || echo "?")

pypi_total=$(curl -sf -A "$UA" --max-time 10 "https://pypistats.org/api/packages/${PYPI_PKG}/overall" \
  | python3 -c "import json,sys; print(sum(r['downloads'] for r in json.load(sys.stdin).get('data',[])))" 2>/dev/null || echo "?")

gh_total=$(gh api "repos/${REPO}/releases" --jq '[.[].assets[].download_count] | add // 0' 2>/dev/null || echo "?")

docker_total=$(curl -sf --max-time 10 "https://hub.docker.com/v2/repositories/blackwellsystems/agent-lsp/" \
  | python3 -c "import json,sys; print(json.load(sys.stdin)['pull_count'])" 2>/dev/null || echo "--")

# ── High-water mark: never regress displayed totals ────────────────

read_cache() {
  local key="$1"
  if [[ -f "$CACHE" ]]; then
    grep "^${key}=" "$CACHE" 2>/dev/null | cut -d= -f2
  fi
}

use_or_cache() {
  local key="$1" val="$2"
  local prev
  prev=$(read_cache "$key")
  if [[ "$val" == "?" || "$val" == "--" ]]; then
    echo "${prev:-$val}"
    return
  fi
  if [[ -n "$prev" && "$prev" != "?" && "$prev" != "--" ]]; then
    if (( val < prev )); then
      echo "$prev"
      return
    fi
  fi
  echo "$val"
}

npm_total=$(use_or_cache "npm" "$npm_total")
pypi_total=$(use_or_cache "pypi" "$pypi_total")
gh_total=$(use_or_cache "gh" "$gh_total")
docker_total=$(use_or_cache "docker" "$docker_total")

# Write cache
mkdir -p "$(dirname "$CACHE")"
cat > "$CACHE" <<EOF
npm=${npm_total}
pypi=${pypi_total}
gh=${gh_total}
docker=${docker_total}
EOF

# ── Determine which rows to show ───────────────────────────────────

has_downloads() {
  [[ "$1" != "0" && "$1" != "--" ]]
}

rows=()
has_downloads "$npm_total" && rows+=("npm|${npm_total}")
has_downloads "$pypi_total" && rows+=("PyPI|${pypi_total}")
has_downloads "$gh_total" && rows+=("GitHub Releases|${gh_total}")
has_downloads "$docker_total" && rows+=("Docker Hub|${docker_total}")

ROW_COUNT=${#rows[@]}
HEIGHT=$((60 + ROW_COUNT * 28 + 10))

# ── Generate SVG ───────────────────────────────────────────────────

mkdir -p "$(dirname "$OUT")"

cat > "$OUT" <<SVGHEADER
<svg xmlns="http://www.w3.org/2000/svg" width="280" height="${HEIGHT}" viewBox="0 0 280 ${HEIGHT}">
  <rect width="280" height="${HEIGHT}" rx="8" fill="#1a1a2e"/>
  <text x="20" y="30" font-family="monospace" font-size="14" fill="#e0e0e0" font-weight="bold">agent-lsp downloads</text>
  <line x1="20" y1="42" x2="260" y2="42" stroke="#333" stroke-width="1"/>
SVGHEADER

Y=65
for row in "${rows[@]}"; do
  IFS='|' read -r label count <<< "$row"
  cat >> "$OUT" <<ROW
  <text x="20" y="${Y}" font-family="monospace" font-size="12" fill="#a0a0a0">${label}</text>
  <text x="260" y="${Y}" font-family="monospace" font-size="12" fill="#4fc3f7" text-anchor="end">${count}</text>
ROW
  Y=$((Y + 28))
done

cat >> "$OUT" <<SVGFOOTER
</svg>
SVGFOOTER

echo "Generated ${OUT} (${ROW_COUNT} channels)"
echo "  npm: ${npm_total} | PyPI: ${pypi_total} | GitHub: ${gh_total} | Docker: ${docker_total}"
