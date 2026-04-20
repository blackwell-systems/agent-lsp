# CI Notes

Implementation details for contributors and maintainers about the language server CI test harness.

## Per-language quirks

**Java (jdtls):** Tier 2 tests are skipped when jdtls does not finish indexing within the CI timeout. This is a known jdtls cold-start characteristic, not a tool bug.

**Scala (metals):** Runs in a separate CI job with `continue-on-error: true` and a 30-minute timeout. metals requires sbt compilation on first start; results are informational.

**Swift (sourcekit-lsp):** Runs on a `macos-latest` runner since sourcekit-lsp ships with Xcode and is not available on Linux CI runners.

**Prisma:** Runs with `continue-on-error: true`. The language server works standalone after `prisma generate` initializes the client.

**SQL (sqls):** Requires a live PostgreSQL service container. The CI job provisions `postgres:16` automatically.

**Nix (nil):** Runs with `continue-on-error: true`. The Nix installer is slow in CI; nil installs via `nix profile install github:oxalica/nil`.

**MongoDB:** The language server is extracted from the `mongodb-js/vscode` VS Code extension VSIX at `dist/languageServer.js`. The CI job has `continue-on-error: true` since the extracted server may behave differently outside a VS Code extension host context. Requires a live `mongo:7` service container provisioned automatically.

**Clojure (clojure-lsp), Nix (nil), Dart (dart language-server), MongoDB (mongodb-language-server):** CI-verified as of the `ci-coverage-expansion` IMPL.

## Tool-specific notes

**`type_hierarchy`:** Tested on Java (jdtls) and TypeScript (typescript-language-server). TypeScript skips when the server does not return a hierarchy item at the configured position.

**Completions and workspace symbol search:** Not supported by some servers in the test harness; marked `—` in the conformance table.
