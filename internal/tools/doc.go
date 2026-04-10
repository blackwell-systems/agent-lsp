// Package tools contains the implementation functions for all agent-lsp MCP
// tools. Tool handlers in cmd/agent-lsp/ call into this package to perform
// the actual LSP operations and format results. The package is organized by
// capability area: navigation (go_to_definition, references, rename),
// analysis (hover, completion, diagnostics, symbols, semantic tokens),
// workspace lifecycle (start_lsp, open/close documents, formatting),
// session simulation (simulate_edit, evaluate_session, simulate_chain),
// build/test dispatch (run_build, run_tests, get_tests_for_file), and
// cross-repo analysis (get_change_impact, get_cross_repo_references).
package tools
