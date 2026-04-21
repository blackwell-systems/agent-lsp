# CI Notes

Implementation details for contributors and maintainers about the language server CI test harness.

## Per-language quirks

**Java (jdtls):** Runs in a dedicated `multi-lang-java` job with `continue-on-error: true` and a 15-minute timeout. Isolated from other language servers to avoid OOM kills — jdtls under memory contention with other servers receives SIGTERM (exit status 15) from the runner. The dedicated job allocates `-Xmx2G` and the full runner memory budget. Tier 2 tools that require workspace indexing (go_to_definition, references, completions, format, semantic tokens, signature_help) need the full 240s initWait before they return results.

**Scala (metals):** Runs in a separate CI job with `continue-on-error: true` and a 30-minute timeout. metals requires sbt compilation on first start; results are informational.

**Swift (sourcekit-lsp):** Runs on a `macos-latest` runner since sourcekit-lsp ships with Xcode and is not available on Linux CI runners.

**Prisma:** Runs with `continue-on-error: true`. The language server works standalone after `prisma generate` initializes the client.

**SQL (sqls):** Requires a live PostgreSQL service container. The CI job provisions `postgres:16` automatically.

**Nix (nil):** Runs with `continue-on-error: true`. The Nix installer is slow in CI; nil installs via `nix profile install github:oxalica/nil`.

**MongoDB:** The language server is extracted from the `mongodb-js/vscode` VS Code extension VSIX at `dist/languageServer.js`. The CI job has `continue-on-error: true` since the extracted server may behave differently outside a VS Code extension host context. Requires a live `mongo:7` service container provisioned automatically.

**Clojure (clojure-lsp), Nix (nil), Dart (dart language-server), MongoDB (mongodb-language-server):** CI-verified as of the `ci-coverage-expansion` IMPL.

## Speculative session test job

`speculative-test` runs `TestSpeculativeSessions` across 8 languages in parallel. Each language subtest gets its own MCP process; subtests within a language run sequentially.

| Language | LSP binary | Error edit target | initWait | Timeout |
|---|---|---|---|---|
| Go | gopls | `return 42` in `string` method | 8s | 120s |
| TypeScript | typescript-language-server | `return "wrong"` in `number` function | 8s | 120s |
| Python | pyright-langserver | `return "wrong"` in `int` function | 8s | 120s |
| Rust | rust-analyzer | `"wrong"` where `i32` expected | 15s | 120s |
| C++ | clangd | `return "wrong"` in `int` function | 10s | 120s |
| C# | csharp-ls | `return 42` in `string` method | 10s | 120s |
| Dart | dart (language-server) | `return 42` in `String` method | 8s | 120s |
| Java | jdtls | `return "wrong"` in `int` method | 120s | 300s |

**Java quirk:** jdtls JVM cold-start requires a 120s `initWait` and a 300s per-language timeout. The CI job timeout is set to 20m. The jdtls workspace data dir (`/tmp/jdtls-workspace-speculative-test`) is separate from the one used by `multi-lang-core` (`/tmp/jdtls-workspace-lsp-mcp-test`) to prevent state collisions if both jobs run on the same runner.

**C++ quirk:** clangd provides single translation-unit (TU) diagnostics only. Cross-file propagation requires a rebuild step not available in the session model. `error_detection` is still reliable for intra-file type errors.

## Tool-specific notes

**`type_hierarchy`:** Tested on Java (jdtls) and TypeScript (typescript-language-server). TypeScript skips when the server does not return a hierarchy item at the configured position.

**Completions and workspace symbol search:** Not supported by some servers in the test harness; marked `—` in the conformance table.
