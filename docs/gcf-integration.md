# GCF Integration: Native Token-Optimized Output for agent-lsp

## Goal

Add GCF (Graph Compact Format) as a native output format for agent-lsp's MCP tool responses. Tool responses currently serialized as JSON can optionally be emitted as GCF tabular format, saving 34-44% tokens on structured data.

## Quick Start

```bash
# Enable GCF output for all tool responses
export AGENT_LSP_OUTPUT_FORMAT=gcf

# Start agent-lsp normally
agent-lsp
```

Unset or set to `json` to revert to default JSON output.

## Why

1. **34-44% fewer tokens on structured data.** Most agent-lsp tools return arrays of objects (symbols, references, diagnostics, callers). GCF tabular encoding eliminates field name repetition and structural delimiters.
2. **Session dedup compounds.** Multi-turn code exploration reuses symbols. By the 5th tool call, 92.7% savings vs JSON.
3. **Competitive moat.** No other MCP code intelligence server has token-optimized output.
4. **We own both projects.** No external approval needed. Ship when ready.

## Profile Fit Analysis

GCF has two encoding profiles. agent-lsp's data model determines which one applies.

### Graph profile (Sections 3-6): Does NOT fit natively

The graph profile has a **fixed schema**: `@id kind qualified_name score provenance`

agent-lsp's tools **do not compute** scores or provenance:

| GCF graph field | knowing (has it) | agent-lsp (has it) |
|----------------|-----------------|-------------------|
| `qualified_name` | Yes | Partial (has name + file, not always qualified) |
| `kind` | Yes | Yes (from LSP SymbolKind) |
| `score` | Yes (relevance ranking) | **No** (LSP doesn't score results) |
| `provenance` | Yes (lsp_resolved, ast_inferred) | **No** (all results come from LSP) |
| `distance` | Yes (graph distance from query) | **No** (no graph distance model) |

**Bottom line:** The graph profile was designed for knowing's data model (scored, ranked, distance-grouped symbols). Forcing it onto agent-lsp would require either fake values or adding a scoring layer, both of which are wrong for this phase.

### Tabular profile (Section 6a): Fits naturally

The tabular profile encodes **arbitrary arrays of objects** with a field declaration header:

```
## symbols [12]{name,kind,file,line,test_callers,non_test_callers}
AuthMiddleware|fn|auth.go|45|2|5
NewServer|fn|server.go|12|1|8
HandleRequest|method|handler.go|78|3|12
```

This maps directly to every agent-lsp tool response:

| Tool | Current JSON shape | Tabular fit |
|------|-------------------|-------------|
| blast_radius | `{changed_symbols: [{name, file, line, callers}]}` | Direct: array of uniform objects |
| find_references | `[{file, line, character}]` | Direct: flat array |
| list_symbols | `[{name, kind, range, detail}]` | Direct: flat array |
| find_callers | `{items, incoming, outgoing}` | Direct: arrays + nested |
| get_diagnostics | `[{file, range, severity, message}]` | Direct: flat array |
| explore_symbol | `{hover, impls, calls, refs}` | Object with nested arrays |
| get_completions | `[{label, kind, detail}]` | Direct: flat array |

### Future: Graph profile as opt-in upgrade

If agent-lsp later adds relevance scoring (e.g., ranking blast_radius results by caller count, or scoring symbols by distance from query center), the graph profile becomes a natural fit. This is a separate feature decision, not a serialization concern.

## Current State

### Serialization Layer

All 66 tool handlers follow the same pattern:

```go
// internal/tools/*.go (53 json.Marshal call sites)
response := map[string]any{ ... }
data, err := json.Marshal(response)
return types.TextResult(string(data)), nil
```

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

### Phase 1: Format negotiation

Add format preference to the MCP session:

```go
// Option A: Per-tool parameter
{"tool": "blast_radius", "args": {"file": "auth.go", "format": "gcf"}}

// Option B: Session-level capability (preferred)
// Client sends during initialize:
{"capabilities": {"experimental": {"output_format": "gcf"}}}
```

Option B is cleaner: set once, applies to all tool responses. Agent-lsp reads the capability during session init and stores it.

### Phase 2: Encoding layer

New package: `internal/encoding/gcf/`

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

### Phase 3: Migration helper

Replace the 53 `json.Marshal` calls with a format-aware helper:

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

### Phase 4: Session deduplication

agent-lsp already has session state (`internal/session/`). Add GCF session tracking so that symbols sent in prior responses become bare references:

```go
// internal/session/gcf_state.go
type GCFState struct {
    mu      sync.Mutex
    session *gcfgo.Session
}

func (s *GCFState) Encode(data any) string {
    s.mu.Lock()
    defer s.mu.Unlock()
    return s.session.EncodeGeneric(data)
}
```

Session dedup is where the compounding savings happen. Multi-turn exploration (blast_radius then explore then find_callers then inspect) reuses records automatically. By the 5th call, 92.7% cumulative savings vs JSON.

### Phase 5: Delta encoding (future)

When a user re-queries after an edit, delta encoding sends only what changed. This depends on the tabular profile gaining delta support in GCF (currently only the graph profile has it).

**Decision:** Defer until tabular delta encoding is specified and implemented in gcf-go.

## Example: blast_radius Before and After

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

## Rollout Plan

### Milestone 1: Format negotiation (2 days)

- [ ] Add `output_format` field to session config
- [ ] Read format preference from MCP experimental capabilities
- [ ] Thread format through context to tool handlers
- [ ] Add `--format gcf` flag to CLI for testing

### Milestone 2: Tabular encoding for high-value tools (3 days)

- [ ] Add `internal/encoding/gcf/` package
- [ ] Add `encodeResult` helper to `internal/tools/helpers.go`
- [ ] Migrate blast_radius (change_impact.go)
- [ ] Migrate find_references, list_symbols, find_symbol (navigation.go, analysis.go)
- [ ] Migrate find_callers, call_hierarchy (callhierarchy.go)
- [ ] Migrate cross_repo (cross_repo.go)
- [ ] Integration tests: verify GCF output decodes to equivalent data

### Milestone 3: Tabular encoding for remaining tools (2 days)

- [ ] Migrate get_diagnostics, suggest_fixes
- [ ] Migrate explore_symbol, detect_changes
- [ ] Migrate simulation tools (simulate_edit, preview_edit, etc.)
- [ ] Migrate remaining tools (get_completions, semantic_tokens, etc.)
- [ ] Skip scalar/text tools (hover, format_document, etc.)

### Milestone 4: Session deduplication (2 days)

- [ ] Add GCFState to session manager
- [ ] Wire session encoding through tool handlers
- [ ] Test: multi-turn exploration shows dedup in action
- [ ] Benchmark: measure actual token savings across 5-call session

### Milestone 5: Benchmarks and documentation (1 day)

- [ ] Benchmark: JSON vs GCF token counts on real tool responses (all tool categories)
- [ ] Update agent-lsp README with measured savings ("Token-Optimized Output" section)
- [ ] Update GCF README to list agent-lsp as production consumer
- [ ] Publish results

## Dependencies

| Dependency | Version | Purpose |
|-----------|---------|---------|
| `github.com/blackwell-systems/gcf-go` | v0.1.0 | Tabular encoding via `EncodeGeneric`, session management |

Zero additional runtime dependencies (gcf-go is zero-dep itself).

## Risks

### 1. MCP clients that can't handle non-JSON text

**Risk:** Some MCP clients might parse the `text` field as JSON and break on GCF output.
**Mitigation:** GCF is opt-in via capabilities. Default remains JSON. Clients that don't negotiate get JSON.

### 2. Comprehension regression on tabular format

**Risk:** LLMs might understand JSON better than GCF tabular for small payloads.
**Mitigation:** GCF tabular has 100% comprehension parity with JSON for structured extraction. For payloads under 10 records, savings are minimal, so keep JSON for scalar/text tools. Test with real agent-lsp payloads before expanding.

### 3. Debugging difficulty

**Risk:** GCF output is less familiar than JSON for debugging.
**Mitigation:** GCF is human-readable by design. Keep JSON as default. Add `gcf decode` CLI command for debugging. Consider a `--verbose` mode that emits both formats.

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
| Session dedup (5-call session) | >80% cumulative | Benchmark multi-turn exploration |
| Comprehension accuracy | 100% on extraction tasks | Run extraction eval with real agent-lsp GCF output |
| Zero regressions | All existing tests pass | CI green after migration |

## Timeline

**Total: ~10 days of focused work**

| Week | Milestones | Deliverable |
|------|-----------|-------------|
| Week 1 | M1 + M2 | Format negotiation, high-value tool encoding |
| Week 2 | M3 + M4 + M5 | Remaining tools, session dedup, benchmarks |

## Future: Graph Profile Upgrade Path

If agent-lsp later adds relevance scoring (ranking symbols by caller count, blast radius, recency), the graph profile becomes viable:

```
GCF tool=blast_radius budget=5000 tokens=847 symbols=12
## targets
@0 fn auth.go:AuthMiddleware 0.95 lsp
@1 fn server.go:NewServer 0.72 lsp
## related
@2 fn main.go:main 0.40 lsp
@3 fn setup.go:Setup 0.35 lsp
## edges
@0<@2 called_by
@0<@3 called_by
@1<@2 called_by
```

This would unlock 79-84% savings on graph tools. The scoring model is a separate feature decision tracked in the roadmap; the encoding infrastructure built in this plan will support it when it arrives.

## Open Questions

1. **Should GCF be the default?** No. Opt-in via capabilities. Make default only after comprehension validation across Claude, GPT, Gemini.
2. **MCP spec alignment?** Is there an emerging standard for output format negotiation in MCP? Check with spec contributors (Sam Morrow is one).
3. **Per-tool opt-in?** Scalar/text tools don't benefit. Format negotiation should be global with per-tool fallback to JSON when GCF adds no value.
4. **Tabular delta encoding?** Currently only the graph profile supports delta. Should GCF spec add tabular deltas? If so, agent-lsp would benefit from re-query efficiency.
