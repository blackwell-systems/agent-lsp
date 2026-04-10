# LSP 3.17 Spec Compliance Audit

**Audit date:** 2026-04-06 (updated 2026-04-10: Findings 1–6 all resolved)
**Inspector version:** 0.2.0
**Repo root:** `/Users/dayna.blackwell/code/agent-lsp`
**Checks applied:** `cross_field_consistency`, `coverage_gap`, `silent_failure`, `doc_drift`
**Scope:** Spec compliance gaps against LSP 3.17 — divergences from what LSP 3.17 requires or specifies.
**Previously fixed issues excluded per audit brief.**

---

## Summary

- **Audited:** `internal/lsp/client.go`, `internal/types/types.go`, `internal/tools/*.go`, `docs/lsp-conformance.md`
- **Layer map:** `cmd → internal/tools → internal/lsp → internal/types/uri`. `internal/lsp` imports `internal/types`, `internal/logging`, `internal/uri`. Extensions import `internal/tools`.
- **Status (updated 2026-04-10):** All six original findings have been resolved. See resolution notes on each finding below.
- **Original highest severity:** error
- **Original signal:** Six spec compliance gaps found across four categories.

---

## Architectural context

Architectural docs found: `docs/architecture.md`, `docs/lsp-conformance.md`. No `CLAUDE.md` or `CONTRIBUTING` at repo root. Patterns inferred from both docs and code. The lsp-conformance.md document explicitly tracks previously fixed issues, so only new gaps are reported here.

---

## Findings

---

### Finding 1: workspace/applyEdit server-initiated request unhandled despite declared capability

**coverage_gap** · error · confidence: high (Grep confirmed, LSP tool unavailable — fallback)

**Status: RESOLVED** — `case "workspace/applyEdit":` handler added to dispatch in `internal/lsp/client.go`. The handler applies the edit and responds with `ApplyWorkspaceEditResult{applied: true}`.

`internal/lsp/client.go` (dispatch switch)

**What:** The client declares `workspace.applyEdit: true` in `Initialize` params, which per LSP 3.17 §workspace_applyEdit signals to the server that it may send `workspace/applyEdit` requests to the client side. The dispatch switch had no `case "workspace/applyEdit":` handler. The unrecognised request fell to the `default:` branch which sends a null response — not a conformant `ApplyWorkspaceEditResult`. Servers like gopls that send `workspace/applyEdit` for code actions requiring file creation or renaming would silently receive a null and interpret the edit as failed.

**Spec reference:** LSP 3.17 §workspace_applyEdit — "The workspace/applyEdit request is sent from the server to the client to modify resource on the client side."

---

### Finding 2: documentChanges resource operations silently discarded

**coverage_gap** · error · confidence: high (Grep confirmed)

**Status: RESOLVED** — `applyDocumentChanges` in `internal/lsp/client.go` now dispatches on a `kind` discriminant field. `CreateFile`, `RenameFile`, and `DeleteFile` entries are handled with corresponding OS-level operations before text edits are applied.

`internal/lsp/client.go` (applyDocumentChanges)

**What:** The spec defines `WorkspaceEdit.documentChanges` as `TextDocumentEdit[] | (TextDocumentEdit | CreateFile | RenameFile | DeleteFile)[]`. The implementation decoded `documentChanges` only into the `TextDocumentEdit` shape. When `documentChanges` contained `CreateFile`, `RenameFile`, or `DeleteFile` entries (which have a `kind` discriminant field and no `edits` array), the struct unmarshal silently ignored those entries. The net result: rename operations that produce file-level resource changes (e.g. renaming a file itself) were silently dropped.

**Spec reference:** LSP 3.17 §workspaceEdit — "documentChanges?: (TextDocumentEdit[] | (TextDocumentEdit | CreateFile | RenameFile | DeleteFile)[])"

---

### Finding 3: rootPath sent in initialize despite conformance doc stating it was removed

**doc_drift** · warning · confidence: high (Grep confirmed)

**Status: RESOLVED** — `rootPath` removed from `Initialize` params in `internal/lsp/client.go`. A comment at the relevant line now reads: "rootPath is deprecated in favour of rootUri; omitted per LSP 3.17." The conformance doc claim is now accurate.

`internal/lsp/client.go` (Initialize params)

**What:** `docs/lsp-conformance.md` listed under "Previously Non-Conformant (Fixed)": `rootPath sent in initialize params | §3.15.1 | Removed`. The `Initialize` method still sent `"rootPath": rootDir`. The doc claim was correct in intent but the code had not received the change yet. Now both are in sync.

---

### Finding 4: $/progress "report" kind not logged despite conformance doc claiming it is

**doc_drift** · warning · confidence: high (Grep confirmed)

**Status: RESOLVED** — `case "report":` added to `handleProgress` in `internal/lsp/client.go` with a `logging.Log(LevelDebug, ...)` call. The conformance doc claim is now accurate.

`internal/lsp/client.go` (handleProgress switch)

**What:** `lsp-conformance.md` stated that all three `WorkDoneProgress` kinds (begin/report/end) are handled. The `handleProgress` switch had only `"begin"` and `"end"` cases — no `"report"` case. A report notification fell through silently. The code and doc were out of sync.

---

### Finding 5: PrepareRename sent when renameProvider is true (no prepareProvider)

**coverage_gap** · warning · confidence: high (code read confirmed)

**Status: RESOLVED** — `case bool:` added to the `renameProvider` type switch in `PrepareRename`. When `renameProvider` is `true` (not an options object, so no `prepareProvider` declared), the method now logs at debug level and returns nil without sending the request.

`internal/lsp/client.go` (PrepareRename)

**What:** The `PrepareRename` method's type switch over `renameProvider` had no `case bool:`. A plain `true` value fell through to send `textDocument/prepareRename` even when `prepareProvider` was not declared, causing a server error response. Per LSP 3.17 §textDocument_prepareRename, `prepareRename` is only valid when `RenameOptions.prepareProvider == true`.

---

### Finding 6: uriToPath uses string stripping, not URL decoding — diverges from spec URI handling

**cross_field_consistency** · warning · confidence: high (code read + Python verification)

**Status: RESOLVED** — A new `internal/uri` package provides `URIToPath(uri string) string` using `url.Parse(uri).Path` for correct RFC 3986 percent-decoding. Both `internal/lsp` and `internal/session` now use `uri.URIToPath` exclusively. The old string-slicing `uriToPath` in `internal/lsp/client.go` is gone. Layer rules are maintained: `internal/uri` imports only `internal/types`, so `internal/lsp` can import it without creating an upward dependency.

`internal/uri/uri.go` (URIToPath)

**What:** Two URI-to-path conversion functions existed with different semantics. `tools.URIToFilePath` used `url.Parse(uri).Path` (correct). `lsp.uriToPath` used simple string slicing, leaving percent-encoding intact. The slicing variant was called in `applyEditsToFile`, `OpenDocument` metadata storage, and `ReopenDocument`, causing incorrect file reads/writes on paths with spaces or special characters.

**Spec reference:** LSP 3.17 §uri — "URI: a string that follows RFC 3986."

---

## All Findings (updated 2026-04-10)

All six original findings have been resolved.

| Severity | Confidence | Check Type | Finding | Status |
|----------|------------|------------|---------|--------|
| error | high | coverage_gap | `workspace/applyEdit` server-initiated request unhandled; null response returned | **RESOLVED** — `case "workspace/applyEdit":` handler added |
| error | high | coverage_gap | `documentChanges` resource operations (CreateFile/RenameFile/DeleteFile) silently discarded | **RESOLVED** — `applyDocumentChanges` dispatches on `kind` discriminant |
| warning | high | doc_drift | `rootPath` still sent in initialize; conformance doc incorrectly states it was removed | **RESOLVED** — `rootPath` removed from `Initialize` params |
| warning | high | doc_drift | `$/progress report` kind not logged; conformance doc claims it is handled | **RESOLVED** — `case "report":` with debug log added to `handleProgress` |
| warning | high | coverage_gap | `PrepareRename` sent when `renameProvider == true` (bool); no `prepareProvider` declared | **RESOLVED** — `case bool:` returns nil without sending request |
| warning | high | cross_field_consistency | Two URI-to-path functions with different percent-encoding semantics | **RESOLVED** — canonical `internal/uri.URIToPath` uses `url.Parse`; string-slicing variant removed |

---

## Not Checked — Out of Scope

- **Position encoding (UTF-16 vs UTF-8):** The client does not declare `general.positionEncodings` in client capabilities (LSP 3.17 §3 added this). Without this declaration, the server defaults to UTF-16. The Go implementation passes character offsets from tool inputs directly as integers without any encoding translation. For ASCII-only or pure-ASCII paths this has no effect; for non-ASCII code the correctness depends entirely on what the server expects. This was noted but not flagged because the spec states "the only mandatory encoding is UTF-16" and servers default to UTF-16 when the client omits `general.positionEncodings` — making the current behaviour spec-compliant for the common case. A full audit of this would require verifying that all position arithmetic (1-based → 0-based conversion) matches UTF-16 semantics.
- **Partial result tokens:** The client sends no `partialResultToken` in any requests. This is spec-compliant; partial results are optional. Servers that stream partial results for long-running requests (e.g. `workspace/symbol`) will not use this path. Not a gap — omission is allowed.
- **`$/cancelRequest` notification:** When a Go context is cancelled (lines 422–426), the pending request is removed but no `$/cancelRequest` notification is sent to the server. The spec says clients "MAY" send this notification, so not sending it is conformant. Server-side this means the server may continue processing a request after the client has abandoned it, wasting resources. Not flagged as a violation.
- **`completionItem` capability declarations:** `completionItem: {}` is declared as empty. This means the client does not announce `snippetSupport`, `insertReplaceSupport`, or `labelDetailsSupport`. Servers will not return snippets. This is a capability negotiation gap in terms of feature completeness, not a spec violation.
- **CodeAction `triggerKind` (3.17.0):** `CodeActionTriggerKind` is a new field in `CodeActionContext` as of 3.17.0. The implementation does not send `triggerKind` in code action requests. This is not flagged because the field is optional per spec.
- **`textDocument/didChange` sync kind:** The client always sends full-document content in `didChange` (line 621: `{"text": text}`). This is correct for `TextDocumentSyncKind.Full`; the client has not declared a sync kind preference so the server chooses. Not a violation, but an efficiency concern for large files if the server supports incremental sync.
- **Checks not applied:** `dead_symbol`, `scope_analysis`, `silent_failure`, `error_wrapping`, `duplicate_semantics`, `test_coverage`, `interface_saturation`, `panic_not_recovered`, `context_propagation`, `init_side_effects`, `layer_violation` — excluded per `--checks` constraint.

## Not Checked — Tooling Constraints

- **mcp__lsp-mcp__ tools:** The `mcp__lsp-mcp__start_lsp` and related tools were not available in this environment (`No such tool available`). The built-in `LSP` tool was used as specified fallback per audit protocol. All symbol-level findings were verified via direct code reading and Grep; confidence is marked accordingly.
- **LSP built-in tool hover/findReferences:** Attempted for type verification on `jsonrpcError`, `ApplyWorkspaceEdit`, and `PrepareRename` symbols. Tool was available but not exercised for hover because the findings were fully determinable from source code reading alone; using it for confirmatory hover would not change the findings or their confidence level.
