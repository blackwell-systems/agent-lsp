# GCF Integration: Native Token-Optimized Output for agent-lsp

## Goal

**Status:** Shipped (v0.13.0). GCF (Graph Compact Format) is the default output format for agent-lsp's MCP tool responses. Graph-shaped tools emit the GCF graph profile; everything else emits the GCF tabular profile, saving 30-51% tokens on structured data. JSON remains available via `AGENT_LSP_OUTPUT_FORMAT=json`.

Sections below that describe the original plan are kept for history; each is marked with what actually shipped.

## Quick Start

GCF is enabled by default. To revert to JSON:

```bash
export AGENT_LSP_OUTPUT_FORMAT=json
```

## Measured Token Savings

Benchmarked on representative tool responses (`go run scripts/gcf-benchmark.go`):

| Tool Response | JSON | GCF | Savings |
|---|---|---|---|
| `list_symbols` (10 symbols) | ~334 tokens | ~165 tokens | **50.6%** |
| `list_symbols` (30 symbols) | ~1,082 tokens | ~548 tokens | **49.4%** |
| `find_references` (12 locations) | ~220 tokens | ~125 tokens | **43.2%** |
| `find_references` (50 locations) | ~858 tokens | ~437 tokens | **49.1%** |
| `get_diagnostics` (5 diagnostics) | ~213 tokens | ~133 tokens | **37.6%** |
| `blast_radius` (5 symbols, 6 callers) | ~526 tokens | ~365 tokens | **30.6%** |

Token estimates use byte length / 3.5 (validated: 0.97 correlation with o200k_base for ASCII-dominant payloads).

Savings grow with record count because the eliminated overhead (field names, delimiters, quotes) is per-record. At 50+ records, savings converge to ~49%.

### Nested-container responses (grouped data)

Some tools return **grouped** data: a nested object array inside a field (blast_radius symbols each with their own callers, detect_changes files each with affected symbols, diagnostics with `relatedInformation`, explore_symbol with parallel refs/calls). GCF encodes these with the `^` attachment marker and a nested tabular block per group:

| Tool Response (nested shape) | JSON | GCF | Savings |
|---|---|---|---|
| `blast_radius` grouped (symbols → callers) | ~139 tokens | ~112 tokens | **19.5%** |
| `get_diagnostics` (+ relatedInformation) | ~114 tokens | ~100 tokens | **12.2%** |
| `explore_symbol` (refs + incoming_calls) | ~87 tokens | ~75 tokens | **13.7%** |
| `detect_changes` (files → affected_symbols) | ~75 tokens | ~70 tokens | **5.7%** |
| **Total** | **~417 tokens** | **~359 tokens** | **13.8%** |

These savings depend on the gcf-go **v1.7.1** encoder (the v1.5.1 nested-container fix). Before it, a nested object array fell through to Go's `fmt` map printing, an unparseable blob, so the nested cases both saved less **and** were not valid GCF. v1.7.1 emits a proper tabular section, which roughly tripled savings-vs-JSON on nested data (~5% → ~14%) and, more importantly, made grouped responses parseable. Reproduce with `go run scripts/gcf-benchmark.go`.

## Why

1. **34-44% fewer tokens on structured data.** Most agent-lsp tools return arrays of objects (symbols, references, diagnostics, callers). GCF tabular encoding eliminates field name repetition and structural delimiters.
2. **Session dedup could compound (not implemented).** Multi-turn exploration reuses symbols, and gcf-go sessions can turn repeated symbols into bare references. agent-lsp does not do this today; see [Phase 4](#phase-4-session-deduplication-not-implemented).
3. **Competitive moat.** No other MCP code intelligence server has token-optimized output.
4. **We own both projects.** No external approval needed. Ship when ready.

## Profile Fit Analysis

GCF has two encoding profiles. agent-lsp's data model determines which one applies.

### Graph profile (Sections 3-6): Used for graph-shaped tools

The graph profile has a fixed schema: `@id kind qualified_name score provenance`, plus edges (`@target<@source type`). The original plan rejected it because LSP results carry no relevance score or provenance. As shipped, agent-lsp fills those fields itself:

| GCF graph field | How agent-lsp fills it |
|----------------|------------------------|
| `qualified_name` | Last two directory segments of the file plus the symbol name (`tools/change_impact.HandleGetChangeImpact`); spaces become underscores (`internal/encoding/gcf/graph.go`) |
| `kind` | Mapped from LSP `SymbolKind` (`blast_radius` reports every symbol as `fn`) |
| `score` | Positional heuristic, not a relevance model: the query target is `1.0`; related symbols score lower by distance and result order (for example call-hierarchy callers start at `0.9` and step down by `0.05`; symbol lists step down by `0.02` per position, floor `0.1`; `blast_radius` uses fixed `0.9` for callers and `0.7` for test functions) |
| `provenance` | `lsp_resolved` (all results come from the language server); multi-server `find_symbol` results use `lsp_resolved@<server>` |
| `distance` | `0` for the target, `1` for directly related symbols |

When the output format is `gcf`, these tools build a `*gcf.Payload` (via `gcf.BuildGraphPayload`) and `EncodeResult` (`internal/tools/helpers.go`) encodes it with the graph profile:

| Tool | Edge types |
|------|-----------|
| `list_symbols` | (symbols only) |
| `find_symbol` | (symbols only) |
| `blast_radius` | `calls` |
| `find_callers` | `calls` |
| `explore_symbol` | `calls` |
| `type_hierarchy` | `extends`, `implements` |
| `get_cross_repo_references` | `references` |

### Tabular profile (Section 6a): Fits naturally

The tabular profile encodes **arbitrary arrays of objects** with a field declaration header:

```
## symbols [12]{name,kind,file,line,test_callers,non_test_callers}
AuthMiddleware|fn|auth.go|45|2|5
NewServer|fn|server.go|12|1|8
HandleRequest|method|handler.go|78|3|12
```

Every other structured tool response goes through the tabular profile:

| Tool | Current JSON shape | Tabular fit |
|------|-------------------|-------------|
| blast_radius | `{changed_symbols: [{name, file, line, callers}]}` | Direct: array of uniform objects |
| find_references | `[{file, line, character}]` | Direct: flat array |
| list_symbols | `[{name, kind, range, detail}]` | Direct: flat array |
| find_callers | `{items, incoming, outgoing}` | Direct: arrays + nested |
| get_diagnostics | `[{file, range, severity, message}]` | Direct: flat array |
| explore_symbol | `{hover, impls, calls, refs}` | Object with nested arrays |
| get_completions | `[{label, kind, detail}]` | Direct: flat array |

## Current State

### Serialization Layer

Tool handlers encode their responses with `EncodeResult(ctx, data)` (`internal/tools/helpers.go`), which picks the format from the request context:

- `gcf` (default): a `*gcf.Payload` is encoded with the graph profile; any other value is encoded with the tabular profile (`internal/encoding/gcf/encode.go`). If GCF encoding fails, the response falls back to JSON.
- `json`: plain `json.Marshal`.

The format is read once at startup from `AGENT_LSP_OUTPUT_FORMAT` (default `gcf`, `cmd/agent-lsp/server.go`) and injected into each tool call's context by the tool registration wrapper. There is no per-session or per-tool negotiation.

Payloads a caller must round-trip byte-exact (a `WorkspaceEdit` handed back to `apply_edit`) always use `EncodeResultJSON`, regardless of the format, because LLM callers corrupt the tabular form of a text edit (issue #12).

The response type is always:

```go
// internal/types/types.go
type ToolResult struct {
    Content []ContentItem `json:"content"`
    IsError bool          `json:"isError,omitempty"`
}

type ContentItem struct {
    Type string `json:"type"` // always "text"
    Text string `json:"text"` // <-- JSON string lives here
}
```

MCP protocol requires `Content` to be a list of text/image items. GCF output replaces the JSON string inside `Text` with a GCF string. The MCP protocol envelope stays JSON; only the tool response payload changes format.

### Tool Categories by Expected Savings

| Category | Tools | Current output | Expected savings |
|----------|-------|---------------|-----------------|
| **High-value arrays** | blast_radius, find_references, list_symbols, find_callers, call_hierarchy, cross_repo | arrays of uniform objects with 4-8 fields | **34-44%** |
| **Medium-value arrays** | get_diagnostics, suggest_fixes, get_completions, semantic_tokens | arrays of objects | **30-40%** |
| **Mixed structures** | explore_symbol, detect_changes, simulate_chain | objects with nested arrays | **25-35%** |
| **Simulation results** | simulate_edit, preview_edit, evaluate_session | diff results, diagnostic deltas | **20-30%** |
| **Low-value / skip** | hover, get_signature_help, format_document, apply_edit, rename_symbol | mostly text or small objects | **Skip** |

### Priority: High-value array tools first

blast_radius, find_references, list_symbols, and find_callers produce the largest payloads with the most uniform structure. These get the biggest savings and prove the concept.

## Architecture

The phases below are the original plan. Shipped: a process-wide format switch (instead of negotiation), the encoding package, the `EncodeResult` helper, and graph-profile payloads for graph-shaped tools. Not implemented: session deduplication and delta encoding.

### Phase 1: Format negotiation (not implemented; replaced by an env var)

Planned: add format preference to the MCP session:

```go
// Option A: Per-tool parameter
{"tool": "blast_radius", "args": {"file": "auth.go", "format": "gcf"}}

// Option B: Session-level capability (preferred)
// Client sends during initialize:
{"capabilities": {"experimental": {"output_format": "gcf"}}}
```

Neither option shipped. The format is set process-wide with `AGENT_LSP_OUTPUT_FORMAT` and defaults to `gcf`.

### Phase 2: Encoding layer (shipped)

Package: `internal/encoding/gcf/` (`encode.go` for the tabular profile via `EncodeGenericChecked`, `graph.go` for graph payloads). Original sketch:

```go
// internal/encoding/gcf/encode.go
package gcf

import gcfgo "github.com/blackwell-systems/gcf-go"

// Encode converts a tool response to GCF tabular format.
// Falls back to JSON if the data structure is not suitable for tabular encoding.
func Encode(tool string, data any) (string, error) {
    return gcfgo.EncodeGeneric(data)
}
```

### Phase 3: Migration helper (shipped as `EncodeResult`)

Shipped as `EncodeResult(ctx, data)` in `internal/tools/helpers.go`, described under [Serialization Layer](#serialization-layer). Original sketch:

```go
// internal/tools/helpers.go
func encodeResult(ctx context.Context, tool string, data any) (types.ToolResult, error) {
    format := session.GetOutputFormat(ctx)  // "json" or "gcf"

    switch format {
    case "gcf":
        encoded, err := gcf.Encode(tool, data)
        if err != nil {
            // Fall back to JSON if GCF encoding fails
            raw, _ := json.Marshal(data)
            return types.TextResult(string(raw)), nil
        }
        return types.TextResult(encoded), nil
    default:
        raw, err := json.Marshal(data)
        if err != nil {
            return types.ErrorResult(err.Error()), nil
        }
        return types.TextResult(string(raw)), nil
    }
}
```

### Phase 4: Session deduplication (not implemented)

Planned: track symbols sent in prior responses with a gcf-go session so repeats become bare references (`internal/session/gcf_state.go`). This was never built; there is no `gcf_state.go`, and each tool response is encoded independently. The projected compounding savings (92.7% cumulative by the 5th call) are a gcf-go benchmark figure, not something agent-lsp delivers today.

### Phase 5: Delta encoding (future)

When a user re-queries after an edit, delta encoding sends only what changed. This depends on the tabular profile gaining delta support in GCF (currently only the graph profile has it).

**Decision:** Defer until tabular delta encoding is specified and implemented in gcf-go.

## Example: tabular encoding of a blast_radius-shaped response

With the default `gcf` format, `blast_radius` itself now emits the graph profile (see [Graph Profile Output](#graph-profile-output)). This example shows how the tabular profile encodes the same nested shape, as it does for other grouped responses such as `detect_changes`.

### Current JSON output (estimated ~2,400 tokens for 12 symbols)

```json
{
  "changed_symbols": [
    {"name": "AuthMiddleware", "file": "auth.go", "line": 45},
    {"name": "NewServer", "file": "server.go", "line": 12}
  ],
  "affected_symbols": [
    {"name": "AuthMiddleware", "file": "auth.go", "line": 45,
     "test_callers": [{"name": "TestAuth", "file": "auth_test.go", "line": 10}],
     "non_test_callers": [{"name": "main", "file": "main.go", "line": 5}]},
    {"name": "NewServer", "file": "server.go", "line": 12,
     "test_callers": [{"name": "TestServer", "file": "server_test.go", "line": 8}],
     "non_test_callers": [{"name": "main", "file": "main.go", "line": 3}, {"name": "Setup", "file": "setup.go", "line": 20}]}
  ],
  "test_files": ["auth_test.go", "server_test.go"],
  "test_functions": ["TestAuth", "TestServer"],
  "non_test_callers": [
    {"name": "main", "file": "main.go", "line": 5},
    {"name": "main", "file": "main.go", "line": 3},
    {"name": "Setup", "file": "setup.go", "line": 20}
  ],
  "summary": "Found 2 changed symbols with 2 test references across 2 test files."
}
```

### GCF tabular output (estimated ~1,500 tokens, 37% savings)

```
summary=Found 2 changed symbols with 2 test references across 2 test files.
## changed_symbols [2]{name,file,line}
AuthMiddleware|auth.go|45
NewServer|server.go|12
## affected_symbols [2]{name,file,line,sync_guarded}
@0 AuthMiddleware|auth.go|45|false
  .test_callers
    ## [1]{name,file,line}
    TestAuth|auth_test.go|10
  .non_test_callers
    ## [1]{name,file,line}
    main|main.go|5
@1 NewServer|server.go|12|false
  .test_callers
    ## [1]{name,file,line}
    TestServer|server_test.go|8
  .non_test_callers
    ## [2]{name,file,line}
    main|main.go|3
    Setup|setup.go|20
## test_files [2]
auth_test.go
server_test.go
## test_functions [2]
TestAuth
TestServer
## non_test_callers [3]{name,file,line}
main|main.go|5
main|main.go|3
Setup|setup.go|20
```

The field declarations (`{name,file,line}`) appear once per section. Each record is just pipe-separated values. For a blast_radius response with 50 symbols, the savings grow linearly because the eliminated waste is per-record.

## Rollout Status

| Planned milestone | Status |
|-------------------|--------|
| M1: Format negotiation | Replaced by `AGENT_LSP_OUTPUT_FORMAT` (default `gcf`); no capability negotiation, no `--format` flag |
| M2: Tabular encoding for high-value tools | Shipped; graph-shaped tools went further and use the graph profile |
| M3: Tabular encoding for remaining tools | Shipped via `EncodeResult`; edits that must round-trip stay JSON |
| M4: Session deduplication | Not implemented |
| M5: Benchmarks and documentation | Shipped: `scripts/gcf-benchmark.go` and the measured savings above |

## Dependencies

| Dependency | Version | Purpose |
|-----------|---------|---------|
| `github.com/blackwell-systems/gcf-go` | v1.7.1 | Tabular encoding (`EncodeGenericChecked`) and graph encoding (`Encode` on a `Payload`) |

Zero additional runtime dependencies (gcf-go is zero-dep itself).

## Risks

### 1. MCP clients that can't handle non-JSON text

**Risk:** Some MCP clients might parse the `text` field as JSON and break on GCF output.
**Mitigation:** GCF is the default, so affected clients must set `AGENT_LSP_OUTPUT_FORMAT=json`. When GCF is active, the server instructions describe the format so agents can read it.

### 2. Comprehension regression on tabular format

**Risk:** LLMs might understand JSON better than GCF tabular for small payloads.
**Mitigation:** GCF tabular has 100% comprehension parity with JSON for structured extraction. For payloads under 10 records, savings are minimal, so keep JSON for scalar/text tools. Test with real agent-lsp payloads before expanding.

### 3. Debugging difficulty

**Risk:** GCF output is less familiar than JSON for debugging.
**Mitigation:** GCF is human-readable by design. Set `AGENT_LSP_OUTPUT_FORMAT=json` when debugging needs JSON.

### 4. Tabular profile maturity

**Risk:** GCF tabular profile is v1.1 (released same day as v1.0). May have edge cases with deeply nested data.
**Mitigation:** agent-lsp's nested data is shallow (1-2 levels max: affected_symbols with test_callers/non_test_callers). Test all tool outputs through encode/decode round-trip before shipping.

### 5. EncodeGeneric performance

**Risk:** Reflection-based generic encoding may be slower than direct json.Marshal.
**Mitigation:** Profile on real payloads. If slow, add type-specific encoders for high-frequency tools (blast_radius, find_references). Encoding time is negligible compared to LSP round-trip latency.

## Success Metrics

| Metric | Target | How to measure |
|--------|--------|---------------|
| Token savings (high-value tools) | >30% vs JSON | Benchmark blast_radius, find_references, list_symbols |
| Token savings (medium-value tools) | >20% vs JSON | Benchmark get_diagnostics, explore_symbol |
| Session dedup (5-call session) | >80% cumulative | Not applicable: session dedup is not implemented |
| Comprehension accuracy | 100% on extraction tasks | Run extraction eval with real agent-lsp GCF output |
| Zero regressions | All existing tests pass | CI green after migration |

## Graph Profile Output

Graph-shaped tools emit the graph profile (see [Profile Fit Analysis](#graph-profile-sections-3-6-used-for-graph-shaped-tools)). Its shape:

```
GCF profile=graph tool=blast_radius symbols=5 edges=3
## targets
@0 fn api/auth.AuthMiddleware 1.00 lsp_resolved
@1 fn api/server.NewServer 1.00 lsp_resolved
## related
@2 fn cmd/app.main 0.90 lsp_resolved
@3 fn api/setup.Setup 0.90 lsp_resolved
@4 fn api/auth_test.TestAuth 0.70 lsp_resolved
## edges [3]
@0<@2 calls
@1<@2 calls
@1<@3 calls
```

Symbols are grouped by distance (`targets` at distance 0, `related` at distance 1) and sorted by score within a group. For `blast_radius`, changed symbols score `1.00`, non-test callers `0.90`, and test functions `0.70`. Edges read `@target<@source type`. (Illustrative values; the qualified names depend on your file layout.)

## Open Questions

1. **Should GCF be the default?** Resolved: yes, since v0.13.0. JSON is available with `AGENT_LSP_OUTPUT_FORMAT=json`.
2. **MCP spec alignment?** Is there an emerging standard for output format negotiation in MCP? Check with spec contributors (Sam Morrow is one).
3. **Per-tool opt-in?** Resolved: the format is global; individual tools return JSON where GCF would hurt (edits that must round-trip) and fall back to JSON when GCF encoding fails.
4. **Tabular delta encoding?** Currently only the graph profile supports delta. Should GCF spec add tabular deltas? If so, agent-lsp would benefit from re-query efficiency.
