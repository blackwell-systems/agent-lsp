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

# ── Update agent instruction files with managed skills table ─────────────────
# Supports the three major agent instruction file formats.
# Each file gets the same managed block (skills table between sentinel comments).
# Files that don't exist are silently skipped — only existing files are updated.

# Build the skills table rows from SKILL.md frontmatter (shared across all files)
table_rows=""
while IFS= read -r skillmd; do
    skill_dir="${skillmd%/SKILL.md}"
    skill_name="$(basename "$skill_dir")"
    desc="$(grep '^description:' "$skillmd" | head -1 | sed 's/^description: *//' | tr -d '"')"
    if [[ -n "$desc" ]]; then
        table_rows+="| \`/${skill_name}\` | ${desc} |"$'\n'
    fi
done < <(find "$SCRIPT_DIR" -maxdepth 2 -name "SKILL.md" | sort)

managed_block="<!-- agent-lsp:skills:start -->"$'\n'
managed_block+="| Skill | When to use |"$'\n'
managed_block+="|-------|-------------|"$'\n'
managed_block+="${table_rows}"
managed_block+="<!-- agent-lsp:skills:end -->"

START_SENTINEL="<!-- agent-lsp:skills:start -->"
END_SENTINEL="<!-- agent-lsp:skills:end -->"

# update_instruction_file FILE LABEL
# Updates a single agent instruction file with the managed skills block.
update_instruction_file() {
    local file="$1"
    local label="$2"

    if [[ ! -f "$file" ]]; then
        return
    fi

    if grep -qF "$START_SENTINEL" "$file"; then
        if [[ "$DRY_RUN" == true ]]; then
            echo "[dry-run] Would update agent-lsp skills block in $label"
        else
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
                "$file" > "${file}.tmp" && mv "${file}.tmp" "$file"
            rm -f "$_block_file"
            echo "  updated  $label skills table"
        fi
    else
        if [[ "$DRY_RUN" == true ]]; then
            echo "[dry-run] Would append agent-lsp skills block to $label"
        else
            printf '\n## LSP Skills\n\n%s\n' "$managed_block" >> "$file"
            echo "  appended  agent-lsp skills block to $label"
        fi
    fi
}

# Claude Code: ~/.claude/CLAUDE.md
update_instruction_file "${HOME}/.claude/CLAUDE.md" "~/.claude/CLAUDE.md"

# OpenAI Codex: AGENTS.md in current working directory
update_instruction_file "$(pwd)/AGENTS.md" "AGENTS.md"

# Gemini CLI: GEMINI.md in current working directory
update_instruction_file "$(pwd)/GEMINI.md" "GEMINI.md"
