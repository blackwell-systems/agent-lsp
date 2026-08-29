package main_test

// Tier-2 capability baseline for the multi-language harness.
//
// Purpose: catch REGRESSIONS. The class of bug this guards against is a tool
// that silently drops to "fail" (for example, a future GCF format or encoding
// change breaks response parsing again, as happened before Wave 2). The
// baseline records, per language and per tool, the WORST acceptable outcome.
// TestMultiLanguage hard-fails only when an actual result is strictly worse
// than its baseline entry. Results better than baseline never fail.
//
// Severity order: pass(2) > skip(1) > fail(0).
//
// Baseline entry semantics (expectedTier2Status):
//   - "pass"         the tool must pass; skip or fail is a regression.
//   - "allowed-skip" the tool must not fail; skip or pass are both fine.
//                    This is the safe, conservative default for languages we
//                    cannot verify locally: it will not break a currently-green
//                    CI job (whose tools pass or skip) but WILL trip if a tool
//                    regresses to a parse-fail.
//   - "allowed-fail" any outcome is acceptable, including fail. Used for
//                    documented real failures and for weak/flaky servers on
//                    continue-on-error CI jobs.
//
// A tool absent from a language's map defaults to "pass" (strict). Go and Luau
// maps are therefore exhaustive: every tool they run has an explicit entry
// derived from a real local run (gopls / luau-lsp), so nothing silently
// defaults to strict "pass" for them.
//
// DERIVATION: Go and Luau entries below were derived from REAL local results
// (PATH=gopls:luau-lsp GOWORK=off go test -run 'TestMultiLanguage/^(Go|Luau)$').
// All other languages default every Tier-2 tool to "allowed-skip" (via
// defaultAllowedSkip), because we cannot verify their exact per-tool pass
// status locally; only their documented real failures and continue-on-error
// weaknesses are pinned to "allowed-fail". Do NOT mark a non-local tool "pass"
// without real evidence: that risks breaking a required CI job (multi-lang-core
// / multi-lang-extended) for a tool whose true status is skip.
//
// REGENERATION: after an intended capability change, regenerate the observed
// map deliberately with:
//
//   AGENT_LSP_DUMP_BASELINE=1 PATH="/path/to/servers:$PATH" GOWORK=off \
//     go test -v -run TestMultiLanguage ./test/
//
// This prints a copy-pasteable Go literal (lang -> tool -> pass|allowed-skip,
// mapping the observed severity: pass->"pass", skip->"allowed-skip",
// fail->"allowed-skip") for every language that actually ran, so a maintainer
// can update the baseline on purpose. It is gated on AGENT_LSP_DUMP_BASELINE
// and never runs in normal test mode.

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

// expectedTier2Status is one of "pass", "allowed-skip", or "allowed-fail".
type expectedTier2Status = string

const (
	statusPass        expectedTier2Status = "pass"
	statusAllowedSkip expectedTier2Status = "allowed-skip"
	statusAllowedFail expectedTier2Status = "allowed-fail"
)

// allTier2Tools is the full set of Tier-2 tool names run by runLanguageTest,
// in the order they are invoked. Used to build conservative default baselines.
var allTier2Tools = []string{
	"list_symbols",
	"go_to_definition",
	"find_references",
	"get_completions",
	"find_symbol",
	"format_document",
	"go_to_declaration",
	"type_hierarchy",
	"inspect_symbol",
	"find_callers",
	"get_semantic_tokens",
	"get_signature_help",
	"get_document_highlights",
	"get_inlay_hints",
	"suggest_fixes",
	"prepare_rename",
	"rename_symbol",
	"get_server_capabilities",
	"workspace_folders",
	"go_to_type_definition",
	"go_to_implementation",
	"format_range",
	"apply_edit",
	"detect_lsp_servers",
	"close_document",
	"did_change_watched_files",
	"run_build",
	"run_tests",
	"get_tests_for_file",
	"get_symbol_source",
	"go_to_symbol",
	"restart_lsp_server",
	"set_log_level",
	"execute_command",
}

// severityOf maps an actual toolResult status ("pass"/"skip"/"fail") to a
// numeric severity for comparison. Higher is better.
func severityOf(status string) int {
	switch status {
	case "pass":
		return 2
	case "skip":
		return 1
	default: // "fail" or anything unexpected
		return 0
	}
}

// severityOfExpected maps a baseline expectation to the minimum acceptable
// severity. "allowed-fail" tolerates any outcome (0); "allowed-skip" requires
// at least skip (1); "pass" requires pass (2).
func severityOfExpected(expected expectedTier2Status) int {
	switch expected {
	case statusPass:
		return 2
	case statusAllowedSkip:
		return 1
	case statusAllowedFail:
		return 0
	default:
		// Unknown expectation: treat as strict pass so typos surface loudly.
		return 2
	}
}

// defaultAllowedSkip builds a baseline map where every Tier-2 tool is
// "allowed-skip", then applies the given per-tool overrides. This is the safe
// conservative default for languages whose exact per-tool pass status is not
// verifiable locally: it never breaks a currently-green job (pass or skip both
// satisfy allowed-skip) but trips if any tool regresses to a hard parse-fail.
func defaultAllowedSkip(overrides map[string]expectedTier2Status) map[string]expectedTier2Status {
	m := make(map[string]expectedTier2Status, len(allTier2Tools))
	for _, tool := range allTier2Tools {
		m[tool] = statusAllowedSkip
	}
	for tool, status := range overrides {
		m[tool] = status
	}
	return m
}

// tier2Baseline maps language name -> tool name -> expected status. A tool
// absent from a language's map defaults to "pass" (strict). See file header for
// derivation and regeneration.
var tier2Baseline = map[string]map[string]expectedTier2Status{
	// --- Locally verified (real gopls run) ---
	// Go: every tool has an explicit entry derived from the observed local run
	// (AGENT_LSP_DUMP_BASELINE on a clean fixture checkout). Tools that genuinely
	// pass are pinned "pass" so a regression to skip/fail trips. Tools that
	// legitimately skip on gopls are "allowed-skip".
	//
	// Fixture-mutation caveat: rename_symbol / format_document / apply_edit
	// rewrite fixtures on disk (a pre-existing harness bug). On a clean checkout
	// they pass, but a dirty re-run makes rename_symbol skip. rename_symbol is
	// therefore "allowed-skip" so a dirty-fixture re-run does not spuriously
	// fail; format_document still passes because gopls formatting is idempotent
	// on the already-formatted fixture.
	"Go": {
		"list_symbols":             statusPass,
		"go_to_definition":         statusPass,
		"find_references":          statusPass,
		"get_completions":          statusPass,
		"find_symbol":              statusPass,
		"format_document":          statusPass,
		"go_to_declaration":        statusAllowedSkip, // non-C: skip
		"type_hierarchy":           statusAllowedSkip, // not configured for Go
		"inspect_symbol":           statusPass,
		"find_callers":             statusAllowedSkip, // gopls returned no items at position
		"get_semantic_tokens":      statusAllowedSkip, // no tokens for range
		"get_signature_help":       statusPass,
		"get_document_highlights":  statusPass,
		"get_inlay_hints":          statusAllowedSkip, // no hints for range
		"suggest_fixes":            statusAllowedSkip, // no actions at position
		"prepare_rename":           statusPass,
		"rename_symbol":            statusAllowedSkip, // fixture-mutation sensitive (see note)
		"get_server_capabilities":  statusPass,
		"workspace_folders":        statusPass,
		"go_to_type_definition":    statusPass,
		"go_to_implementation":     statusPass,
		"format_range":             statusPass,
		"apply_edit":               statusAllowedSkip, // no edits on clean fixture / mutation sensitive
		"detect_lsp_servers":       statusPass,
		"close_document":           statusPass,
		"did_change_watched_files": statusPass,
		"run_build":                statusPass, // Go fixture has go.mod; build dispatch runs
		"run_tests":                statusPass,
		"get_tests_for_file":       statusPass,
		"get_symbol_source":        statusPass,
		"go_to_symbol":             statusPass,
		"restart_lsp_server":       statusPass,
		"set_log_level":            statusPass,
		"execute_command":          statusPass,
	},
	// Luau: every tool has an explicit entry derived from the observed local run
	// (AGENT_LSP_DUMP_BASELINE, clean checkout). luau-lsp does not format, so
	// format_document/format_range/apply_edit skip; there is no build/test
	// dispatch for luau, so run_build/run_tests skip. rename_symbol is
	// "allowed-skip" for the same fixture-mutation reason as Go.
	"Luau": {
		"list_symbols":             statusPass,
		"go_to_definition":         statusPass,
		"find_references":          statusPass,
		"get_completions":          statusPass,
		"find_symbol":              statusPass,
		"format_document":          statusAllowedSkip, // supportsFormatting=false
		"go_to_declaration":        statusAllowedSkip, // non-C: skip
		"type_hierarchy":           statusAllowedSkip, // not configured for Luau
		"inspect_symbol":           statusPass,
		"find_callers":             statusAllowedSkip, // no items at position
		"get_semantic_tokens":      statusPass,
		"get_signature_help":       statusAllowedSkip, // no signatures at position
		"get_document_highlights":  statusAllowedSkip, // no highlights returned
		"get_inlay_hints":          statusAllowedSkip, // no hints for range
		"suggest_fixes":            statusPass,
		"prepare_rename":           statusAllowedSkip, // prepareRename not supported
		"rename_symbol":            statusAllowedSkip, // fixture-mutation sensitive (see note)
		"get_server_capabilities":  statusPass,
		"workspace_folders":        statusPass,
		"go_to_type_definition":    statusPass,
		"go_to_implementation":     statusPass,
		"format_range":             statusAllowedSkip, // supportsFormatting=false
		"apply_edit":               statusAllowedSkip, // supportsFormatting=false
		"detect_lsp_servers":       statusPass,
		"close_document":           statusPass,
		"did_change_watched_files": statusPass,
		"run_build":                statusAllowedSkip, // no build dispatch for luau
		"run_tests":                statusAllowedSkip, // no test dispatch for luau
		"get_tests_for_file":       statusPass,
		"get_symbol_source":        statusPass,
		"go_to_symbol":             statusPass,
		"restart_lsp_server":       statusPass,
		"set_log_level":            statusPass,
		"execute_command":          statusAllowedSkip, // no command exercised
	},

	// --- NOT locally verifiable: default every tool to allowed-skip. ---
	// Documented real failures (see docs/reference/language-support.md CI
	// matrix) and continue-on-error jobs get "allowed-fail" overrides so a weak
	// or flaky server never hard-breaks the harness.

	"TypeScript": defaultAllowedSkip(nil),
	"Python":     defaultAllowedSkip(nil),
	"Rust":       defaultAllowedSkip(nil),

	// Java: continue-on-error CI job (jdtls cold-start is flaky). Its tool
	// coverage is minimal in the matrix; tolerate failures liberally.
	"Java": defaultAllowedSkip(map[string]expectedTier2Status{
		"list_symbols":     statusAllowedFail,
		"go_to_definition": statusAllowedFail,
		"find_references":  statusAllowedFail,
		"get_completions":  statusAllowedFail,
		"find_symbol":      statusAllowedFail,
		"format_document":  statusAllowedFail,
	}),

	"C":          defaultAllowedSkip(nil),
	"PHP":        defaultAllowedSkip(nil),
	"C++":        defaultAllowedSkip(nil),
	"JavaScript": defaultAllowedSkip(nil),
	"Ruby":       defaultAllowedSkip(nil),
	"YAML":       defaultAllowedSkip(nil),
	"JSON":       defaultAllowedSkip(nil),
	"Dockerfile": defaultAllowedSkip(nil),
	"C#":         defaultAllowedSkip(nil),
	"CSharp":     defaultAllowedSkip(nil),
	"Kotlin":     defaultAllowedSkip(nil),
	"Lua":        defaultAllowedSkip(nil),
	"Swift":      defaultAllowedSkip(nil),

	// Zig: documented real failure — find_symbol (workspace) fails.
	"Zig": defaultAllowedSkip(map[string]expectedTier2Status{
		"find_symbol": statusAllowedFail,
	}),

	"CSS":       defaultAllowedSkip(nil),
	"HTML":      defaultAllowedSkip(nil),
	"Terraform": defaultAllowedSkip(nil),

	// Scala: continue-on-error CI job (metals, experimental).
	"Scala": defaultAllowedSkip(map[string]expectedTier2Status{
		"list_symbols":     statusAllowedFail,
		"go_to_definition": statusAllowedFail,
		"find_references":  statusAllowedFail,
		"get_completions":  statusAllowedFail,
		"find_symbol":      statusAllowedFail,
		"format_document":  statusAllowedFail,
	}),

	// Gleam: documented real failure — find_symbol (workspace) fails.
	"Gleam": defaultAllowedSkip(map[string]expectedTier2Status{
		"find_symbol": statusAllowedFail,
	}),

	// Elixir: continue-on-error CI job (experimental) AND documented real
	// failure — list_symbols fails.
	"Elixir": defaultAllowedSkip(map[string]expectedTier2Status{
		"list_symbols":     statusAllowedFail,
		"go_to_definition": statusAllowedFail,
		"find_references":  statusAllowedFail,
		"get_completions":  statusAllowedFail,
		"find_symbol":      statusAllowedFail,
		"format_document":  statusAllowedFail,
	}),

	// Prisma: continue-on-error CI job (experimental).
	"Prisma": defaultAllowedSkip(map[string]expectedTier2Status{
		"list_symbols":     statusAllowedFail,
		"go_to_definition": statusAllowedFail,
		"find_references":  statusAllowedFail,
		"get_completions":  statusAllowedFail,
		"find_symbol":      statusAllowedFail,
		"format_document":  statusAllowedFail,
	}),

	"SQL":     defaultAllowedSkip(nil),
	"Clojure": defaultAllowedSkip(nil),

	// Nix: continue-on-error CI job (nil, experimental).
	"Nix": defaultAllowedSkip(map[string]expectedTier2Status{
		"list_symbols":     statusAllowedFail,
		"go_to_definition": statusAllowedFail,
		"find_references":  statusAllowedFail,
		"get_completions":  statusAllowedFail,
		"find_symbol":      statusAllowedFail,
		"format_document":  statusAllowedFail,
	}),

	"Dart": defaultAllowedSkip(nil),

	// MongoDB: continue-on-error CI job (experimental).
	"MongoDB": defaultAllowedSkip(map[string]expectedTier2Status{
		"list_symbols":     statusAllowedFail,
		"go_to_definition": statusAllowedFail,
		"find_references":  statusAllowedFail,
		"get_completions":  statusAllowedFail,
		"find_symbol":      statusAllowedFail,
		"format_document":  statusAllowedFail,
	}),
}

// assertTier2Baseline fails the subtest when any actual Tier-2 result is
// strictly worse than its baseline entry. Only call this when Tier 1 passed;
// languages that skip or fail at Tier 1 are not baseline-checked.
//
// For each result, the expected status is looked up in tier2Baseline[langName]
// (default "pass" when the tool is absent). If the actual severity is below the
// expected minimum, t.Errorf reports a regression naming lang, tool, expected,
// and actual. Results equal to or better than baseline never fail.
func assertTier2Baseline(t *testing.T, langName string, results []toolResult) {
	t.Helper()
	langBaseline := tier2Baseline[langName]
	for _, r := range results {
		expected := statusPass // absent => strict pass
		if langBaseline != nil {
			if e, ok := langBaseline[r.tool]; ok {
				expected = e
			}
		}
		if severityOf(r.status) < severityOfExpected(expected) {
			detail := r.detail
			if detail == "" {
				detail = "(no detail)"
			}
			t.Errorf("[%s] Tier-2 regression: tool %q expected %q but got %q — %s",
				langName, r.tool, expected, r.status, detail)
		}
	}
}

// dumpTier2Baseline prints a copy-pasteable Go literal of the observed
// lang -> tool -> status map for every language that actually ran (Tier 1
// passed), mapping observed severity to a baseline expectation:
//
//	pass -> "pass", skip -> "allowed-skip", fail -> "allowed-skip".
//
// It is gated on AGENT_LSP_DUMP_BASELINE=1 and is intended for deliberate
// regeneration after an intended capability change. It is a no-op otherwise.
func dumpTier2Baseline(t *testing.T, results []langResult) {
	if os.Getenv("AGENT_LSP_DUMP_BASELINE") == "" {
		return
	}
	var b strings.Builder
	b.WriteString("\n// ===== AGENT_LSP_DUMP_BASELINE: observed Tier-2 baseline =====\n")
	b.WriteString("// Copy the relevant language blocks into tier2Baseline after verifying.\n")
	b.WriteString("var observedTier2Baseline = map[string]map[string]expectedTier2Status{\n")
	for _, r := range results {
		if r.tier1 != "pass" {
			continue
		}
		b.WriteString(fmt.Sprintf("\t%q: {\n", r.name))
		// Sort tools for stable output.
		sorted := make([]toolResult, len(r.tier2))
		copy(sorted, r.tier2)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].tool < sorted[j].tool })
		for _, tr := range sorted {
			expected := statusAllowedSkip
			if tr.status == "pass" {
				expected = statusPass
			}
			b.WriteString(fmt.Sprintf("\t\t%q: %q,\n", tr.tool, expected))
		}
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n")
	t.Logf("%s", b.String())
}
