# LSP 3.17 Spec Compliance Audit

**Audit date:** 2026-04-06
**Inspector version:** 0.2.0
**Repo root:** `/Users/dayna.blackwell/code/LSP-MCP-GO`
**Checks applied:** `cross_field_consistency`, `coverage_gap`, `silent_failure`, `doc_drift`
**Scope:** Spec compliance gaps against LSP 3.17 — divergences from what LSP 3.17 requires or specifies.
**Previously fixed issues excluded per audit brief.**

---

## Summary

- **Audited:** `internal/lsp/client.go`, `internal/types/types.go`, `internal/tools/*.go`, `docs/lsp-conformance.md`
- **Layer map:** `cmd → internal/tools → internal/lsp → internal/types`. `internal/lsp` imports only `internal/types`. Extensions import `internal/tools`.
- **Highest severity:** error
- **Signal:** Six spec compliance gaps found across four categories; the most critical is a declared server capability (`workspace.applyEdit: true`) with no server-initiated handler, meaning any server that sends `workspace/applyEdit` requests receives a null response that reports the edit as failed.

---

## Architectural context

Architectural docs found: `docs/architecture.md`, `docs/lsp-conformance.md`. No `CLAUDE.md` or `CONTRIBUTING` at repo root. Patterns inferred from both docs and code. The lsp-conformance.md document explicitly tracks previously fixed issues, so only new gaps are reported here.

---

## Findings

---

### Finding 1: workspace/applyEdit server-initiated request unhandled despite declared capability

**coverage_gap** · error · confidence: high (Grep confirmed, LSP tool unavailable — fallback)

`internal/lsp/client.go:468` and `internal/lsp/client.go:237–274`

**What:** The client declares `workspace.applyEdit: true` in `Initialize` params (line 468), which per LSP 3.17 §workspace_applyEdit signals to the server that it may send `workspace/applyEdit` requests to the client side. The dispatch switch (lines 237–274) has no `case "workspace/applyEdit":` handler. The unrecognised request falls to the `default:` branch (line 270) which sends a null response. However, the spec defines the response as `ApplyWorkspaceEditResult { applied: boolean; failureReason?: string; failedChange?: uinteger }` — null is not a conformant response. Servers like gopls that send `workspace/applyEdit` for code actions requiring file creation or renaming will silently receive a null and interpret the edit as failed.

**Spec reference:** LSP 3.17 §workspace_applyEdit — "The workspace/applyEdit request is sent from the server to the client to modify resource on the client side."

**Fix:** Either add a `case "workspace/applyEdit":` handler in `dispatch` that applies the edit and responds with `ApplyWorkspaceEditResult{applied: true}`, or remove `applyEdit: true` from the declared capabilities to stop servers from sending this request.

---

### Finding 2: documentChanges resource operations silently discarded

**coverage_gap** · error · confidence: high (Grep confirmed)

`internal/lsp/client.go:1244–1259`

**What:** The spec defines `WorkspaceEdit.documentChanges` as `TextDocumentEdit[] | (TextDocumentEdit | CreateFile | RenameFile | DeleteFile)[]`. The implementation decodes `documentChanges` only into the `TextDocumentEdit` shape (struct with `textDocument.URI` and `edits`). When `documentChanges` contains `CreateFile`, `RenameFile`, or `DeleteFile` entries (which have a `kind` discriminant field and no `edits` array), the struct unmarshal either silently ignores those entries (because the anonymous struct has no `kind` field) or fails the entire unmarshal depending on field ordering. If unmarshal fails, the code falls through to the legacy `changes` map — which also won't contain the file operations. The net result: rename operations that produce file-level resource changes (e.g. renaming a file itself, not just symbol occurrences) are silently dropped.

**Spec reference:** LSP 3.17 §workspaceEdit — "documentChanges?: (TextDocumentEdit[] | (TextDocumentEdit | CreateFile | RenameFile | DeleteFile)[])"

**Fix:** Add a `Kind` field discriminant to the documentChanges decode struct. Dispatch on `kind == "create"` / `"rename"` / `"delete"` to perform the corresponding OS-level file operation before applying text edits.

---

### Finding 3: rootPath sent in initialize despite conformance doc stating it was removed

**doc_drift** · warning · confidence: high (Grep confirmed)

`docs/lsp-conformance.md:176` vs `internal/lsp/client.go:459`

**What:** `docs/lsp-conformance.md` line 176 lists under "Previously Non-Conformant (Fixed)": `rootPath sent in initialize params | §3.15.1 | Removed (deprecated, superseded by rootUri)`. The `Initialize` method at `client.go:459` still sends `"rootPath": rootDir` in the params. The spec marks `rootPath` as `@deprecated in favour of rootUri`. The documentation claims this was fixed and removed; the code did not receive the corresponding change. Callers consulting the conformance doc would believe rootPath is no longer sent.

**Note:** Sending `rootPath` alongside `rootUri` is not spec-incorrect since the spec says "if both are set, rootUri wins." The harm is the false documentation claim, not a runtime failure. The spec also deprecates `rootUri` itself in favour of `workspaceFolders`, which the code correctly sends; the conformance doc does not mention that `rootUri` is deprecated.

**Fix:** Either remove `"rootPath": rootDir` from the initialize params (making the doc accurate) or update `lsp-conformance.md` to reflect that rootPath is intentionally retained for server compatibility.

---

### Finding 4: $/progress "report" kind not logged despite conformance doc claiming it is

**doc_drift** · warning · confidence: high (Grep confirmed)

`docs/lsp-conformance.md:82` vs `internal/lsp/client.go:325–330`

**What:** `lsp-conformance.md` line 80–83 states: "$/progress begin/report/end — all three WorkDoneProgress kinds are handled: begin: token added to active set, title logged; report: intermediate progress logged; end: token removed." The `handleProgress` function has a switch with only two cases: `"begin"` and `"end"`. There is no `case "report":`. A `report` notification falls through the switch silently — no logging occurs and no state is modified. The doc claim that `report` is "handled" and "intermediate progress logged" is inaccurate.

**Spec reference:** LSP 3.17 §progress — `WorkDoneProgressReport` kind `"report"` carries `message` and `percentage` fields that describe intermediate task state.

**Fix:** Either add `case "report":` with a log call, or update the conformance doc to remove the claim that report notifications are logged.

---

### Finding 5: PrepareRename sent when renameProvider is true (no prepareProvider)

**coverage_gap** · warning · confidence: high (code read confirmed)

`internal/lsp/client.go:1167–1177`

**What:** The `PrepareRename` method checks the `renameProvider` capability with a type switch. The switch handles `map[string]interface{}` (checks `prepareProvider` flag) and `nil` (returns early). If `renameProvider` is a plain `bool` (`true`), neither case matches and execution falls through to send the `textDocument/prepareRename` request. Per LSP 3.17 §textDocument_prepareRename, the server only supports `prepareRename` if `RenameOptions.prepareProvider == true`. When `renameProvider` is `true` (not an options object), `prepareProvider` is not declared, and sending the request will result in an error response from the server.

**Spec reference:** LSP 3.17 §textDocument_prepareRename — "Server capability: property path (optional): renameProvider property type: boolean | RenameOptions"

**Fix:** Add `case bool:` to the switch. When `renameProvider` is `true` (not an options object), log that `prepareRename` is not supported and return nil without sending the request.

---

### Finding 6: uriToPath uses string stripping, not URL decoding — diverges from spec URI handling

**cross_field_consistency** · warning · confidence: high (code read + Python verification)

`internal/lsp/client.go:1472–1478` vs `internal/tools/helpers.go:49–58`

**What:** Two URI-to-path conversion functions exist with different semantics. `tools.URIToFilePath` uses `url.Parse(uri).Path` which correctly decodes percent-encoded characters (e.g. `%20` → space). `lsp.uriToPath` uses simple string slicing (`uri[len("file://"):]`), which leaves percent-encoding intact (e.g. `file:///Users/foo%20bar` → `/Users/foo%20bar` as a literal string, not `/Users/foo bar`). The `uriToPath` function is called in `applyEditsToFile` (line 1285), `OpenDocument` metadata storage (line 629), and `ReopenDocument` (line 742). The LSP spec requires URIs to be valid RFC 3986 URIs, which permit percent-encoding. A workspace containing files with spaces or special characters in their paths will produce incorrect file reads and writes when `uriToPath` is used.

**Spec reference:** LSP 3.17 §uri — "URI: a string that follows RFC 3986."

**Fix:** Replace `lsp.uriToPath` with a call to `url.Parse(uri).Path` (matching the behaviour of `tools.URIToFilePath`), or export `tools.URIToFilePath` and import it in the `lsp` package. Note: the `lsp` package must not import `internal/tools` per the layer rules; the fix should move the correct implementation into `internal/types` or add it directly in `lsp`.

---

## All Findings

| Severity | Confidence | Check Type | Finding | Location |
|----------|------------|------------|---------|----------|
| error | high | coverage_gap | `workspace/applyEdit` server-initiated request unhandled; null response returned despite `applyEdit: true` declared | `internal/lsp/client.go:468,270` |
| error | high | coverage_gap | `documentChanges` resource operations (CreateFile/RenameFile/DeleteFile) silently discarded | `internal/lsp/client.go:1244` |
| warning | high | doc_drift | `rootPath` still sent in initialize; conformance doc incorrectly states it was removed | `internal/lsp/client.go:459` / `docs/lsp-conformance.md:176` |
| warning | high | doc_drift | `$/progress report` kind not logged or handled; conformance doc claims it is handled | `internal/lsp/client.go:325` / `docs/lsp-conformance.md:82` |
| warning | high | coverage_gap | `PrepareRename` sent when `renameProvider == true` (bool); no `prepareProvider` declared | `internal/lsp/client.go:1167` |
| warning | high | cross_field_consistency | Two URI-to-path functions with different percent-encoding semantics; `uriToPath` in lsp package is incorrect for paths with spaces | `internal/lsp/client.go:1472` vs `internal/tools/helpers.go:49` |

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
