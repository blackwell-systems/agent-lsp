#!/usr/bin/env bash
set -euo pipefail

# lsp-mcp-go skills installer
# Installs Agent Skills directories to ~/.claude/skills/
# macOS/Linux: full support. Windows WSL: use --copy. Windows native: not supported.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEST_DIR="${HOME}/.claude/skills"
USE_COPY=false
FORCE=false
DRY_RUN=false

usage() {
    cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Install lsp-mcp-go skills to ~/.claude/skills/

Each skill is a directory containing a SKILL.md file (Agent Skills format).
Skills are installed as symlinks by default (idempotent, non-destructive).

Options:
  --copy      Copy directories instead of creating symlinks
  --force     Overwrite existing links/directories
  --dry-run   Show what would be done without making changes
  --help      Show this help message

Examples:
  $(basename "$0")              # Symlink all skills
  $(basename "$0") --copy       # Copy all skills
  $(basename "$0") --dry-run    # Preview what would happen
  $(basename "$0") --force      # Replace existing links/directories
EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --copy)   USE_COPY=true; shift ;;
        --force)  FORCE=true; shift ;;
        --dry-run) DRY_RUN=true; shift ;;
        --help|-h) usage; exit 0 ;;
        *) echo "Unknown option: $1" >&2; usage >&2; exit 1 ;;
    esac
done

# Collect skill directories (those containing a SKILL.md)
skill_dirs=()
while IFS= read -r skillmd; do
    skill_dirs+=("${skillmd%/SKILL.md}")
done < <(find "$SCRIPT_DIR" -maxdepth 2 -name "SKILL.md" | sort)

if [[ ${#skill_dirs[@]} -eq 0 ]]; then
    echo "No skill directories found in $SCRIPT_DIR"
    exit 0
fi

# Create destination directory
if [[ "$DRY_RUN" == true ]]; then
    echo "[dry-run] Would create directory: $DEST_DIR"
else
    mkdir -p "$DEST_DIR"
fi

installed=0
skipped=0
errors=0

for src in "${skill_dirs[@]}"; do
    skill_name="$(basename "$src")"
    dest="$DEST_DIR/$skill_name"

    if [[ -e "$dest" || -L "$dest" ]]; then
        if [[ "$FORCE" == false ]]; then
            echo "  skip  $skill_name (already exists; use --force to overwrite)"
            ((skipped++)) || true
            continue
        fi
        if [[ "$DRY_RUN" == false ]]; then
            rm -rf "$dest"
        fi
    fi

    if [[ "$USE_COPY" == true ]]; then
        if [[ "$DRY_RUN" == true ]]; then
            echo "[dry-run] Would copy: $skill_name/ -> $dest/"
        else
            if cp -r "$src" "$dest"; then
                echo "  copy  $skill_name/"
                ((installed++)) || true
            else
                echo "  ERROR copying $skill_name/" >&2
                ((errors++)) || true
            fi
        fi
    else
        if [[ "$DRY_RUN" == true ]]; then
            echo "[dry-run] Would symlink: $skill_name/ -> $dest"
        else
            if ln -s "$src" "$dest"; then
                echo "  link  $skill_name/"
                ((installed++)) || true
            else
                echo "  ERROR symlinking $skill_name/" >&2
                ((errors++)) || true
            fi
        fi
    fi
done

echo ""
if [[ "$DRY_RUN" == true ]]; then
    echo "Dry run complete. No changes made."
else
    echo "Done. Installed: $installed, Skipped: $skipped, Errors: $errors"
    [[ $errors -gt 0 ]] && exit 1
fi
