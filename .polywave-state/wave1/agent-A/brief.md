---
polywave_name: '[polywave:wave1:agent-A] Create test/gcf_decode_test.go (package main_test) with the shared GCF-decode'
---

# Agent A Brief - Wave 1

**IMPL Doc:** /Users/dayna/code/agent-lsp/docs/IMPL/IMPL-multilang-tier2-verification.yaml

## Files Owned

- `test/gcf_decode_test.go`


## Task

Create test/gcf_decode_test.go (package main_test) with the shared GCF-decode
helpers that Wave 2 will consume. This is a NEW file with no dependencies.

Implement exactly these unexported helpers (they live in package main_test
alongside multi_lang_test.go, so lowercase names are fine and shared):

  import gcfgo "github.com/blackwell-systems/gcf-go"

  func decodeGraph(text string) (*gcfgo.Payload, error)
    // return gcfgo.Decode(text)

  func decodeGeneric(text string) (any, error)
    // return gcfgo.DecodeGeneric(text)

  func graphSymbolNames(p *gcfgo.Payload) []string
    // For each s in p.Symbols, take the segment after the last '.' in
    // s.QualifiedName (or the whole string if no '.'), return the slice.
    // Handle nil p -> empty slice.

  func graphEdgeCount(p *gcfgo.Payload) int
    // return len(p.Edges), 0 for nil p.

The server-side encode contract these invert lives in
internal/tools/helpers.go (EncodeResult) and internal/encoding/gcf/ (Encode,
EncodeGraph). decodeGraph is the inverse of the *gcfgo.Payload branch;
decodeGeneric is the inverse of the tabular branch. Do NOT parse GCF text by
hand; use only the gcf-go exported decoders.

Add a small self-test in the same file, e.g. TestGcfDecodeHelpers, that
round-trips a tiny payload: build a *gcfgo.Payload with one Symbol
(QualifiedName "pkg/foo.Person", Kind "type") and one Edge, encode it via
gcfgo.Encode, decode via decodeGraph, and assert graphSymbolNames contains
"Person" and graphEdgeCount==1. This proves the helper works without needing a
language server installed.

### Constraints
- Do NOT modify test/multi_lang_test.go (owned by Agent B in Wave 2).
- Do NOT hand-roll GCF parsing; use gcfgo.Decode / gcfgo.DecodeGeneric only.
- Do NOT modify files not in your ownership list.

### Verification gate
- GOWORK=off go build ./...
- GOWORK=off go vet ./test/...
- gofmt -l test/gcf_decode_test.go   # expect empty output
- GOWORK=off go test -run TestGcfDecodeHelpers ./test/   # must PASS (no server needed)
- Postcondition: grep -c "func decodeGraph\|func decodeGeneric\|func graphSymbolNames\|func graphEdgeCount" test/gcf_decode_test.go  # expect 4



## Interface Contracts

### decodeGraph

Decode a GCF graph-profile tool response into a *gcfgo.Payload. This is the true
inverse of the server's EncodeResult path for *gcfgo.Payload data
(internal/tools/helpers.go EncodeResult -> gcf.EncodeGraph -> gcfgo.Encode). Used by
graph-shaped tools: list_symbols, find_symbol, go_to_definition, find_references,
go_to_declaration, find_callers, go_to_symbol, type_hierarchy.


```
// decodeGraph decodes a GCF graph-profile response (as produced by
// gcf.EncodeGraph on the server) into a Payload. Returns an error if the
// text is not valid GCF graph format.
func decodeGraph(text string) (*gcfgo.Payload, error)
// Implementation: return gcfgo.Decode(text)

```

### decodeGeneric

Decode a GCF generic/tabular-profile tool response. Inverse of the server's
gcf.Encode path (EncodeGenericChecked). Used by generic-shaped tools:
get_completions, get_document_highlights, get_inlay_hints, get_semantic_tokens,
get_server_capabilities, workspace folders, detect_lsp_servers, apply_edit,
run_build, run_tests, get_tests_for_file, get_symbol_source.


```
// decodeGeneric decodes a GCF generic-profile response into a Go value
// (typically a []any of maps or a map[string]any). Returns an error if the
// text is not valid GCF generic format.
func decodeGeneric(text string) (any, error)
// Implementation: return gcfgo.DecodeGeneric(text)
// If the caller needs the raw remainder / ordered form, DecodeGenericFull is
// available: func DecodeGenericFull(text string) (GenericSet, string, error)

```

### graphSymbolNames

Extract the list of symbol names (unqualified, last dot segment of QualifiedName)
from a decoded graph Payload, so tests can assert "symbol Person present" without
each test reimplementing traversal.


```
// graphSymbolNames returns the trailing name segment of every symbol's
// QualifiedName in the payload (e.g. "tools/foo.Person" -> "Person").
func graphSymbolNames(p *gcfgo.Payload) []string

```

### graphEdgeCount

Return len(p.Edges) for a decoded graph Payload (used by find_references /
find_callers style tests that assert a minimum count).


```
func graphEdgeCount(p *gcfgo.Payload) int

```

### Tier2Baseline

Per-language expected-status map for Tier-2 tools. Models each tool as one of
three expected statuses: "pass", "allowed-skip", "allowed-fail". TestMultiLanguage
HARD-FAILS when an actual result is worse than its baseline entry (a "pass" that
regressed to skip/fail, or an "allowed-skip" that regressed to fail). Actual
results BETTER than baseline are fine (never fail). CRITICAL: the baseline MUST be
DERIVED FROM REAL POST-PARSER-FIX RESULTS produced by Wave 2, not from the stale
docs matrix, and MUST preserve legitimate per-language skips/gaps (see constraints).


```
// expectedTier2Status is "pass", "allowed-skip", or "allowed-fail".
// tier2Baseline maps language name -> tool name -> expectedTier2Status.
// A tool absent from a language's map defaults to "pass" (strict).
var tier2Baseline map[string]map[string]string
// assertTier2Baseline fails the subtest when any actual result is strictly
// worse than its baseline entry. severity order: pass > skip > fail.
func assertTier2Baseline(t *testing.T, langName string, results []toolResult)

```



## Quality Gates

Level: standard

- **build**: `GOWORK=off go build ./...` (required: true)
- **lint**: `GOWORK=off go vet ./test/...` (required: true)
- **format**: `gofmt -l test/` (required: true)
- **custom**: `GOWORK=off go test -c -o /dev/null ./test/` (required: true)

