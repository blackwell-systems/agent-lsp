#!/usr/bin/env bash
set -euo pipefail

# lsp-mcp-go skills installer
# macOS/Linux: full support. Windows WSL: use --copy. Windows native: not supported.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEST_DIR="${HOME}/.claude/skills/lsp"
USE_COPY=false
FORCE=false
DRY_RUN=false

# Files to exclude from installation
EXCLUDE=("PATTERNS.md" "install.sh")

usage() {
    cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Install lsp-mcp-go skill files to ~/.claude/skills/lsp/

Options:
  --copy      Copy files instead of creating symlinks
  --force     Overwrite existing files/symlinks
  --dry-run   Show what would be done without making changes
  --help      Show this help message

By default, skill files are installed as symlinks (idempotent, non-destructive).
Running this script twice produces the same result.

Exclusions: PATTERNS.md and install.sh are not installed.

Examples:
  $(basename "$0")              # Symlink all skills
  $(basename "$0") --copy       # Copy all skills
  $(basename "$0") --dry-run    # Preview what would happen
  $(basename "$0") --force      # Replace existing links/files
EOF
}

is_excluded() {
    local file="$1"
    local basename
    basename="$(basename "$file")"
    for excluded in "${EXCLUDE[@]}"; do
        if [[ "$basename" == "$excluded" ]]; then
            return 0
        fi
    done
    return 1
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --copy)
            USE_COPY=true
            shift
            ;;
        --force)
            FORCE=true
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --help|-h)
            usage
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            usage >&2
            exit 1
            ;;
    esac
done

# Collect skill files
mapfile -t skill_files < <(find "$SCRIPT_DIR" -maxdepth 1 -name "*.md" | sort)

if [[ ${#skill_files[@]} -eq 0 ]]; then
    echo "No .md skill files found in $SCRIPT_DIR"
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

for src in "${skill_files[@]}"; do
    if is_excluded "$src"; then
        continue
    fi

    filename="$(basename "$src")"
    dest="$DEST_DIR/$filename"

    # Check if destination already exists
    if [[ -e "$dest" || -L "$dest" ]]; then
        if [[ "$FORCE" == false ]]; then
            echo "  skip  $filename (already exists; use --force to overwrite)"
            ((skipped++)) || true
            continue
        fi
        # Force mode: remove existing
        if [[ "$DRY_RUN" == false ]]; then
            rm -f "$dest"
        fi
    fi

    if [[ "$USE_COPY" == true ]]; then
        if [[ "$DRY_RUN" == true ]]; then
            echo "[dry-run] Would copy: $filename -> $dest"
        else
            if cp "$src" "$dest"; then
                echo "  copy  $filename"
                ((installed++)) || true
            else
                echo "  ERROR copying $filename" >&2
                ((errors++)) || true
            fi
        fi
    else
        if [[ "$DRY_RUN" == true ]]; then
            echo "[dry-run] Would symlink: $filename -> $dest"
        else
            if ln -s "$src" "$dest"; then
                echo "  link  $filename"
                ((installed++)) || true
            else
                echo "  ERROR symlinking $filename" >&2
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
    if [[ $errors -gt 0 ]]; then
        exit 1
    fi
fi
