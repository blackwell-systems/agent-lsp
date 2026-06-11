#!/usr/bin/env bash
# Queries npm, PyPI, GitHub Releases, and Docker Hub for cumulative download
# stats across all agent-lsp distribution channels. Generates:
#   assets/download-stats.svg   (visual card)
#   assets/downloads-badge.json (shields.io endpoint badge)
#
# Usage: ./scripts/download-stats.sh [output-path]
# Default output: assets/download-stats.svg
# CI: schedule every 12 hours
set -euo pipefail

REPO="blackwell-systems/agent-lsp"
NPM_PKG="@blackwell-systems/agent-lsp"
PYPI_PKG="agent-lsp"
DOCKER_REPO="blackwellsystems/agent-lsp"
OUT="${1:-assets/download-stats.svg}"
CACHE="${OUT%.svg}.cache"

UA="agent-lsp-stats/1.0 (https://github.com/blackwell-systems/agent-lsp)"

# ── Fetch all-time totals ───────────────────────────────────────────

npm_total=$(curl -sf --max-time 10 "https://api.npmjs.org/downloads/point/2000-01-01:2030-01-01/${NPM_PKG}" \
  | python3 -c "import json,sys; print(json.load(sys.stdin)['downloads'])" 2>/dev/null || echo "?")

pypi_total=$(curl -sf -A "$UA" --max-time 10 "https://pypistats.org/api/packages/${PYPI_PKG}/overall" \
  | python3 -c "import json,sys; print(sum(r['downloads'] for r in json.load(sys.stdin).get('data',[])))" 2>/dev/null || echo "?")

gh_total=$(gh api "repos/${REPO}/releases" --jq '[.[].assets[].download_count] | add // 0' 2>/dev/null || echo "?")

docker_total=$(curl -sf --max-time 10 "https://hub.docker.com/v2/repositories/${DOCKER_REPO}/" \
  | python3 -c "import json,sys; print(json.load(sys.stdin).get('pull_count', 0))" 2>/dev/null || echo "?")

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
  prev="${prev:-0}"
  if [[ "$val" != "?" && "$val" != "--" ]]; then
    if [[ "$prev" != "?" && "$prev" != "--" && "$prev" != "0" ]]; then
      if (( val >= prev )); then echo "$val"; else echo "$prev"; fi
    else
      echo "$val"
    fi
    return
  fi
  if [[ "$prev" != "0" && "$prev" != "--" && "$prev" != "?" ]]; then
    echo "$prev"
  else
    echo "$val"
  fi
}

npm_total=$(use_or_cache npm "$npm_total")
pypi_total=$(use_or_cache pypi "$pypi_total")
gh_total=$(use_or_cache gh "$gh_total")
docker_total=$(use_or_cache docker "$docker_total")

mkdir -p "$(dirname "$CACHE")"
cat > "$CACHE" << CACHEEOF
npm=${npm_total}
pypi=${pypi_total}
gh=${gh_total}
docker=${docker_total}
CACHEEOF

# ── Calculate cumulative total ──────────────────────────────────────

cumulative=0
for v in "$npm_total" "$pypi_total" "$gh_total" "$docker_total"; do
  if [[ "$v" != "?" && "$v" != "--" ]]; then
    cumulative=$((cumulative + v))
  fi
done
if (( cumulative == 0 )); then cumulative="?"; fi

fmt() { printf "%'d" "$1" 2>/dev/null || echo "$1"; }

npm_fmt=$(fmt "$npm_total" 2>/dev/null || echo "$npm_total")
pypi_fmt=$(fmt "$pypi_total" 2>/dev/null || echo "$pypi_total")
gh_fmt=$(fmt "$gh_total" 2>/dev/null || echo "$gh_total")
docker_fmt=$(fmt "$docker_total" 2>/dev/null || echo "$docker_total")
cumulative_fmt=$(fmt "$cumulative" 2>/dev/null || echo "$cumulative")

date_str=$(date +"%Y-%m-%d")

# ── Build rows dynamically ──────────────────────────────────────────

has_downloads() {
  local val="$1"
  [[ "$val" != "--" && "$val" != "0" && "$val" != "?" ]]
}

rows=""
row_count=0
add_row() {
  local label="$1" value="$2"
  local y=$(( 74 + row_count * 22 ))
  rows+="
  <text x=\"24\" y=\"${y}\" fill=\"#94a3b8\" font-family=\"ui-monospace,monospace\" font-size=\"12\">${label}</text>
  <text x=\"296\" y=\"${y}\" fill=\"#e2e8f0\" font-family=\"ui-monospace,monospace\" font-size=\"13\" font-weight=\"600\" text-anchor=\"end\">${value}</text>"
  row_count=$((row_count + 1))
}

has_downloads "$pypi_total"    && add_row "pypi (agent-lsp)"           "$pypi_fmt"
has_downloads "$docker_total"  && add_row "docker (agent-lsp)"         "$docker_fmt"
has_downloads "$npm_total"     && add_row "npm (@blackwell-systems)"   "$npm_fmt"
has_downloads "$gh_total"      && add_row "github releases"            "$gh_fmt"

svg_height=$(( 48 + row_count * 22 + 16 + 28 + 20 ))
divider_y=$(( 48 + row_count * 22 + 8 ))
total_y=$(( divider_y + 28 ))
stroke_height=$(( svg_height - 2 ))

# ── Generate SVG ─────────────────────────────────────────────────────

mkdir -p "$(dirname "$OUT")"
cat > "$OUT" << SVGEOF
<svg xmlns="http://www.w3.org/2000/svg" width="320" height="${svg_height}" viewBox="0 0 320 ${svg_height}">
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stop-color="#1a1a2e"/>
      <stop offset="100%" stop-color="#16213e"/>
    </linearGradient>
    <linearGradient id="accent" x1="0" y1="0" x2="1" y2="0">
      <stop offset="0%" stop-color="#00e5ff"/>
      <stop offset="100%" stop-color="#00b8d4"/>
    </linearGradient>
  </defs>

  <rect width="320" height="${svg_height}" rx="12" fill="url(#bg)"/>
  <rect x="1" y="1" width="318" height="${stroke_height}" rx="11" fill="none" stroke="#334155" stroke-width="1"/>

  <text x="24" y="36" fill="#e2e8f0" font-family="system-ui,-apple-system,sans-serif" font-size="14" font-weight="600">agent-lsp downloads</text>
  <text x="296" y="36" fill="#64748b" font-family="system-ui,-apple-system,sans-serif" font-size="10" text-anchor="end">${date_str}</text>

  <line x1="24" y1="48" x2="296" y2="48" stroke="#334155" stroke-width="1"/>
${rows}

  <line x1="24" y1="${divider_y}" x2="296" y2="${divider_y}" stroke="#334155" stroke-width="1"/>

  <text x="24" y="${total_y}" fill="url(#accent)" font-family="system-ui,-apple-system,sans-serif" font-size="16" font-weight="700">${cumulative_fmt} total</text>
  <text x="296" y="${total_y}" fill="#64748b" font-family="system-ui,-apple-system,sans-serif" font-size="10" text-anchor="end">cumulative installs</text>
</svg>
SVGEOF

# ── Generate shields.io endpoint badge JSON ──────────────────────────

BADGE_OUT="$(dirname "$OUT")/downloads-badge.json"
if [[ "$cumulative" != "?" ]]; then
  cat > "$BADGE_OUT" << BADGEEOF
{
  "schemaVersion": 1,
  "label": "downloads",
  "message": "${cumulative_fmt}",
  "color": "1e3a5f"
}
BADGEEOF
fi

echo "Generated ${OUT}"
echo "  npm: ${npm_total}  pypi: ${pypi_total}  github: ${gh_total}  docker: ${docker_total}  total: ${cumulative}"
