#!/usr/bin/env bash
set -euo pipefail

# agent-lsp skills installer
# Installs AgentSkills-conformant skill directories to any agent's skill folder.
# Default: ~/.claude/skills/ (Claude Code). Use --dest for other agents.
# macOS/Linux: full support. Windows WSL: use --copy. Windows native: not supported.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEST_DIR="${HOME}/.claude/skills"
USE_COPY=false
FORCE=false
DRY_RUN=false

usage() {
    cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Install agent-lsp skills (AgentSkills format) to an agent's skill directory.

Each skill is a directory containing a SKILL.md file conforming to the
AgentSkills open standard (https://agentskills.io/specification).
Skills are installed as symlinks by default (idempotent, non-destructive).

Options:
  --dest DIR  Install to DIR instead of ~/.claude/skills/
  --copy      Copy directories instead of creating symlinks
  --force     Overwrite existing links/directories
  --dry-run   Show what would be done without making changes
  --help      Show this help message

Examples:
  $(basename "$0")                          # Claude Code (default)
  $(basename "$0") --dest ~/.cursor/skills  # Cursor
  $(basename "$0") --dest ~/.config/gemini-cli/skills  # Gemini CLI
  $(basename "$0") --copy                   # Copy instead of symlink
  $(basename "$0") --dry-run                # Preview what would happen
EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dest)   DEST_DIR="$2"; shift 2 ;;
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
    if [[ $errors -gt 0 ]]; then exit 1; fi
fi

# Update CLAUDE.md with managed skills table (Claude Code only)
# Skip this step when installing to a non-Claude destination.
if [[ "$DEST_DIR" != "${HOME}/.claude/skills" ]]; then
    exit 0
fi
# Finds ~/.claude/CLAUDE.md and replaces content between sentinel comments.
# If sentinels don't exist, appends the block at the end of the file.
CLAUDE_MD="${HOME}/.claude/CLAUDE.md"

if [[ ! -f "$CLAUDE_MD" ]]; then
    if [[ "$DRY_RUN" == false ]]; then
        echo ""
        echo "No ~/.claude/CLAUDE.md found — skipping skills table update."
    fi
else
    # Build the skills table rows from SKILL.md frontmatter
    table_rows=""
    while IFS= read -r skillmd; do
        skill_dir="${skillmd%/SKILL.md}"
        skill_name="$(basename "$skill_dir")"
        # Extract description from frontmatter (first line starting with 'description:')
        desc="$(grep '^description:' "$skillmd" | head -1 | sed 's/^description: *//' | tr -d '"')"
        if [[ -n "$desc" ]]; then
            table_rows+="| \`/${skill_name}\` | ${desc} |"$'\n'
        fi
    done < <(find "$SCRIPT_DIR" -maxdepth 2 -name "SKILL.md" | sort)

    # Build the full managed block
    managed_block="<!-- agent-lsp:skills:start -->"$'\n'
    managed_block+="| Skill | When to use |"$'\n'
    managed_block+="|-------|-------------|"$'\n'
    managed_block+="${table_rows}"
    managed_block+="<!-- agent-lsp:skills:end -->"

    START_SENTINEL="<!-- agent-lsp:skills:start -->"
    END_SENTINEL="<!-- agent-lsp:skills:end -->"

    if grep -qF "$START_SENTINEL" "$CLAUDE_MD"; then
        # Replace between sentinels (inclusive)
        if [[ "$DRY_RUN" == true ]]; then
            echo "[dry-run] Would update agent-lsp skills block in $CLAUDE_MD"
        else
            # Write managed block to a temp file — avoids passing multi-line
            # strings via awk -v, which BSD awk (macOS) does not support.
            _block_file=$(mktemp)
            printf '%s' "$managed_block" > "$_block_file"
            awk -v start="$START_SENTINEL" \
                -v end="$END_SENTINEL" \
                -v block_file="$_block_file" \
                'BEGIN{skip=0}
                 $0==start{
                   while ((getline line < block_file) > 0) print line
                   skip=1; next
                 }
                 $0==end{skip=0; next}
                 !skip{print}' \
                "$CLAUDE_MD" > "${CLAUDE_MD}.tmp" && mv "${CLAUDE_MD}.tmp" "$CLAUDE_MD"
            rm -f "$_block_file"
            echo "  updated  ~/.claude/CLAUDE.md skills table"
        fi
    else
        # Append managed block at end of file
        if [[ "$DRY_RUN" == true ]]; then
            echo "[dry-run] Would append agent-lsp skills block to $CLAUDE_MD"
        else
            printf '\n## LSP Skills\n\n%s\n' "$managed_block" >> "$CLAUDE_MD"
            echo "  appended  agent-lsp skills block to ~/.claude/CLAUDE.md"
        fi
    fi
fi
