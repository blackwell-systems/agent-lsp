# agent-lsp Audit 4

## Summary
- Audited: `internal/config/`, `cmd/agent-lsp/server.go`, `internal/tools/`, `internal/session/`, `internal/lsp/`, `internal/resources/`, `internal/logging/`, `internal/extensions/`
- Layer map: `cmd/agent-lsp` → `internal/tools`, `internal/session`, `internal/resources`, `internal/extensions` → `internal/lsp` → `internal/types`. `internal/logging` is used by all layers. No cycles detected.
- Highest severity: error
- Signal: Five findings of note — one data race on a package-level variable, one resource leak on partial server initialization, two dead public APIs with no production caller, and one wired parameter that is accepted but never used.

---

## cmd/agent-lsp/server.go

**dead_symbol** · error · confidence: reduced
`cmd/agent-lsp/server.go:119` · [LSP unavailable — Grep fallback, reduced confidence]
What: The `registry *extensions.ExtensionRegistry` parameter of `Run` is accepted in the function signature but never referenced anywhere in the function body. `registry.ToolHandlers()`, `registry.ResourceHandlers()`, `registry.SubscriptionHandlers()`, and `registry.PromptHandlers()` are never called from `Run`. The `ExtensionRegistry` returned by `extensions.NewRegistry()` in `main.go` is wired through, populated with language extensions, and then silently dropped. The extension system is fully registered but produces no runtime effect.
Fix: Either wire `registry` into the tool/resource dispatch loop inside `Run` (per the design intent of the extension system), or remove the parameter and the activation calls in `main.go` until the feature is ready. Carrying a populated but unused parameter is misleading and masks the integration gap.

---

## internal/logging/logging.go

**coverage_gap** · error · confidence: high
`internal/logging/logging.go:100–102` · [LSP unavailable — Grep fallback, reduced confidence]
What: `initWarning` is a package-level `var string` that is read and written in `Log()` outside of the `mu` mutex. The read (`if w := initWarning; w != "" {`) and the clearing write (`initWarning = ""`) at lines 100–101 are not protected by the lock that guards the rest of logging state. `Log()` is called concurrently (from goroutines in the LSP client's `readLoop`, `drainStderr`, and progress monitor). Under Go's race detector this is a data race. In the common case `initWarning` is set exactly once during `init()` and cleared on the first `Log()` call, so the window is narrow but real in test environments that stress concurrent logging startup.
Fix: Either read and clear `initWarning` under `mu.Lock()`, or use a `sync.Once` to flush it exactly once before the first call to the main logging path.

---

## internal/lsp/manager.go

**silent_failure** · error · confidence: high
`internal/lsp/manager.go:79–94` (function `StartAll`) · [LSP unavailable — Grep fallback, reduced confidence]
What: `StartAll` iterates over all configured entries and starts each LSP client. On the first `Initialize` error it returns immediately, leaving all previously successfully initialized clients running (spawned subprocess, open stdin/stdout pipes) but with no reference stored on their respective entries (`e.client` is not set) and no cleanup call. These goroutines and file descriptors are leaked. For example, if a user configures `go:gopls typescript:tsserver` and gopls initializes but tsserver fails, gopls is now running with no way to shut it down — `Shutdown` only shuts down clients where `e.client != nil`.
Fix: Before returning the error, iterate the already-started entries and call their `Shutdown` method, or store the client before checking the error so `Shutdown` can reach it.

---

## internal/resources/resources.go · internal/resources/subscriptions.go

**dead_symbol** · warning · confidence: reduced
`internal/resources/resources.go:24–31` (`ResourceEntry` type) · [LSP unavailable — Grep fallback, reduced confidence]
What: `ResourceEntry` is an exported struct with four fields. Grep across the entire repository (including all non-test Go files) shows zero usages outside the file that defines it. No production code constructs or reads this type. It has no test coverage either.
Fix: Remove the type if it is not part of an upcoming feature, or add a `// TODO` comment with intent if it is reserved for a planned API.

**dead_symbol** · warning · confidence: reduced
`internal/resources/subscriptions.go:22` (`HandleSubscribeDiagnostics`), `internal/resources/subscriptions.go:55` (`HandleUnsubscribeDiagnostics`), `internal/resources/subscriptions.go:13` (`NotifyFunc`) · [LSP unavailable — Grep fallback, reduced confidence]
What: All three exported symbols are defined in `subscriptions.go` and called only from `resources_test.go`. They are never imported or called from `cmd/agent-lsp/server.go` or any other production package. The subscription infrastructure exists but is not wired into the MCP server's `resources/subscribe` flow — the resource templates are registered, but the `server.AddResourceSubscribeHandler`-equivalent callback is absent from `server.go`.
Fix: Either wire `HandleSubscribeDiagnostics`/`HandleUnsubscribeDiagnostics` into `server.go`'s resource subscription handling, or document the symbols as "pending integration" with a `TODO` comment.

---

## internal/lsp/client.go

**context_propagation** · warning · confidence: high
`internal/lsp/client.go:290` · [LSP unavailable — Grep fallback, reduced confidence]
What: Inside `dispatch()`, which handles the server-initiated `workspace/applyEdit` request, the code creates a fresh `context.WithTimeout(context.Background(), defaultTimeout)` to pass to `ApplyWorkspaceEdit`. The `dispatch()` function has no context parameter (it is called from `readLoop()`, which also has none), so there is no caller context to propagate. This is therefore a legitimate root context and not a violation of context propagation rules. However, the indentation of lines 291–292 is misaligned (they are indented one level less than the surrounding `if` block), suggesting a formatting artifact.
Fix: Run `gofmt` on the file to correct the indentation at lines 291–292. This is cosmetic but makes the intent harder to read.

---

## All Findings

| Severity | Confidence | Check Type | Finding | Location |
|----------|------------|------------|---------|----------|
| error | reduced | dead_symbol | `registry` parameter accepted by `Run` but never used — entire extension system is wired but produces no runtime effect | `cmd/agent-lsp/server.go:119` |
| error | high | coverage_gap | `initWarning` read/write in `Log()` is not protected by `mu` — data race under concurrent goroutines | `internal/logging/logging.go:100` |
| error | high | silent_failure | `StartAll` leaks already-started LSP subprocess clients when a later entry fails to initialize | `internal/lsp/manager.go:89` |
| warning | reduced | dead_symbol | `ResourceEntry` type defined but never used in any production code | `internal/resources/resources.go:25` |
| warning | reduced | dead_symbol | `HandleSubscribeDiagnostics`, `HandleUnsubscribeDiagnostics`, `NotifyFunc` defined but not wired into MCP server | `internal/resources/subscriptions.go:22,55,13` |
| warning | high | doc_drift (cosmetic) | `workspace/applyEdit` handler has misaligned indentation on lines 291–292 | `internal/lsp/client.go:291` |

---

## Not Checked — Out of Scope

- Prior audit findings (Audits 1–3) confirmed as fixed; not re-examined.
- `internal/lsp/framing.go`, `internal/types/` — not in the specified focus areas.
- Integration tests in `integration_test.go` — not in scope.
- `extensions/haskell/` — only checked that `init()` registers a factory (no side effects beyond registration); no issues found.

## Not Checked — Tooling Constraints

- LSP MCP tools (`mcp__lsp__start_lsp`, `mcp__lsp__find_references`, `mcp__lsp__inspect_symbol`) could not be invoked via Bash or the available tool interface in this environment. All dead_symbol findings used Grep across the full repository as fallback. These carry `confidence: reduced` and should be re-verified with `gopls` references before acting on them. The primary findings (logging race, StartAll leak, unused registry parameter) do not depend on LSP — they are visible from static code reading.
