# Shared LSP Skill Patterns

## @@ Position Marker

When calling position-based tools (get_info_on_location, go_to_definition,
get_references), prefer the position_pattern parameter over line/column:

    "position_pattern": "func (c *LSPClient) Initiali@@ze"

The @@ marker indicates cursor position (character immediately after @@).
More reliable than coordinate math. line/column still work as fallback.

## start_lsp Guard Pattern

Every skill that calls LSP tools SHOULD include a guard:
"If LSP is not yet initialized, call start_lsp with the workspace root first."

Auto-init note: agent-lsp supports workspace auto-inference from file paths.
Explicit start_lsp only needed when switching workspace roots.

## Diagnostic Diff Output Format

    ## Diagnostic Summary
    - Errors introduced:   N  (each as: file:line - message)
    - Errors resolved:     N  (each as: file:line - message)
    - Net change:         +N / -N / 0
    - Warnings introduced: N (only if N > 0)
    - Warnings resolved:   N (only if N > 0)

Rules: only show sections where N > 0. Net change of 0 = safe to proceed.
