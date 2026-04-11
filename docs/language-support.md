# Language Support

## Install reference

| Language | Server | Install |
|----------|--------|---------|
| TypeScript / JavaScript | `typescript-language-server` | `npm i -g typescript-language-server typescript` |
| Python | `pyright-langserver` | `npm i -g pyright` |
| Go | `gopls` | `go install golang.org/x/tools/gopls@latest` |
| Rust | `rust-analyzer` | `rustup component add rust-analyzer` |
| C / C++ | `clangd` | `apt install clangd` / `brew install llvm` |
| Ruby | `solargraph` | `gem install solargraph` |
| PHP | `intelephense` | `npm i -g intelephense` |
| Java | `jdtls` | [eclipse.jdt.ls snapshots](https://download.eclipse.org/jdtls/snapshots/) |
| YAML | `yaml-language-server` | `npm i -g yaml-language-server` |
| JSON | `vscode-json-language-server` | `npm i -g vscode-langservers-extracted` |
| Dockerfile | `docker-langserver` | `npm i -g dockerfile-language-server-nodejs` |
| C# | `csharp-ls` | `dotnet tool install -g csharp-ls` |
| Kotlin | `kotlin-language-server` | [GitHub releases](https://github.com/fwcd/kotlin-language-server/releases) |
| Lua | `lua-language-server` | [GitHub releases](https://github.com/LuaLS/lua-language-server/releases) |
| Swift | `sourcekit-lsp` | Ships with Xcode / Swift toolchain |
| Zig | `zls` | [GitHub releases](https://github.com/zigtools/zls/releases) (match Zig version) |
| CSS | `vscode-css-language-server` | `npm i -g vscode-langservers-extracted` |
| HTML | `vscode-html-language-server` | `npm i -g vscode-langservers-extracted` |
| Terraform | `terraform-ls` | [releases.hashicorp.com](https://releases.hashicorp.com/terraform-ls/) |
| Scala | `metals` | `cs install metals` ([Coursier](https://get-coursier.io)) |
| Gleam | `gleam` (built-in) | [GitHub releases](https://github.com/gleam-lang/gleam/releases) |
| Elixir | `elixir-ls` | [GitHub releases](https://github.com/elixir-lsp/elixir-ls/releases) |
| Prisma | `prisma-language-server` | `npm i -g @prisma/language-server` |
| SQL | `sqls` | `go install github.com/sqls-server/sqls@latest` |
| Clojure | `clojure-lsp` | [GitHub releases](https://github.com/clojure-lsp/clojure-lsp/releases) |
| Nix | `nil` | [GitHub releases](https://github.com/oxalica/nil/releases) |
| Dart | `dart language-server` | Ships with Dart SDK (`brew install dart`) |
| MongoDB | `mongodb-language-server` | `npm i -g @mongodb-js/mongodb-language-server` |

---

## CI tool coverage matrix

Tier 1 (`start_lsp`, `open_document`, `get_diagnostics`, `get_info_on_location`) verified for all 30 languages. Tier 2: 34 additional tools.

| Language | Tier 1 | symbols | definition | references | completions | workspace | format | declaration | type_hierarchy | hover | call_hier | sem_tok | sig_help |
|----------|--------|---------|------------|------------|-------------|-----------|--------|-------------|----------------|-------|-----------|---------|----------|
| TypeScript | pass | pass | pass | pass | pass | pass | pass | pass | — | pass | pass | pass | pass |
| Python | pass | pass | pass | pass | pass | pass | — | — | — | pass | pass | pass | — |
| Go | pass | pass | pass | pass | pass | pass | pass | — | — | pass | pass | pass | pass |
| Rust | pass | pass | pass | pass | pass | pass | pass | — | — | pass | pass | pass | — |
| Java | pass | — | — | — | — | — | — | — | pass | pass | pass | — | — |
| C | pass | pass | pass | pass | pass | pass | pass | pass | — | pass | pass | pass | — |
| PHP | pass | pass | pass | pass | pass | pass | — | — | — | pass | pass | pass | pass |
| C++ | pass | pass | pass | pass | pass | pass | pass | pass | — | pass | pass | pass | — |
| JavaScript | pass | pass | pass | pass | pass | pass | pass | pass | — | pass | pass | pass | — |
| Ruby | pass | pass | pass | pass | pass | pass | pass | — | — | pass | pass | pass | pass |
| YAML | pass | — | — | — | pass | pass | pass | — | — | pass | — | — | — |
| JSON | pass | — | — | — | pass | pass | pass | — | — | pass | — | — | — |
| Dockerfile | pass | — | — | — | pass | pass | — | — | — | pass | — | — | — |
| C# | pass | pass | pass | pass | pass | pass | pass | — | — | pass | pass | pass | pass |
| Kotlin | pass | pass | pass | pass | pass | pass | pass | — | — | pass | pass | pass | pass |
| Lua | pass | pass | — | — | pass | pass | pass | — | — | pass | pass | pass | pass |
| Swift | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | pass | — |
| Zig | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | pass | — |
| CSS | pass | pass | — | — | pass | pass | pass | — | — | pass | — | — | — |
| HTML | pass | — | — | — | pass | pass | pass | — | — | pass | — | — | — |
| Terraform | pass | pass | pass | — | pass | pass | pass | — | — | pass | — | — | — |
| Scala | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | pass | — |
| Gleam | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | — | — |
| Elixir | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | — | — |
| Prisma | pass | pass | pass | pass | — | — | pass | — | — | pass | — | — | — |
| SQL | pass | pass | pass | pass | pass | pass | — | — | — | pass | — | — | — |
| Clojure | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | — | — |
| Nix | pass | pass | — | — | pass | pass | — | — | — | pass | — | — | — |
| Dart | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | — | — |
| MongoDB | pass | — | — | — | pass | pass | — | — | — | pass | — | — | — |

See [ci-notes.md](./ci-notes.md) for per-language CI quirks.

---

## Current (30 languages, CI-tested)

| Language | Language Server | Status |
|---|---|---|
| TypeScript | typescript-language-server | passing |
| Python | pyright-langserver | passing |
| Go | gopls | passing |
| Rust | rust-analyzer | passing |
| Java | jdtls | flaky (cold-start indexing) |
| C | clangd | passing |
| PHP | intelephense | passing |
| C++ | clangd | passing |
| JavaScript | typescript-language-server | passing |
| Ruby | solargraph | passing |
| YAML | yaml-language-server | passing |
| JSON | vscode-json-language-server | passing |
| Dockerfile | docker-langserver | passing |
| C# | csharp-ls | passing |
| Kotlin | kotlin-language-server | passing |
| Lua | lua-language-server | passing |
| Swift | sourcekit-lsp | passing (macos-latest runner) |
| Zig | zls | passing |
| CSS | vscode-css-language-server | passing |
| HTML | vscode-html-language-server | passing |
| Terraform | terraform-ls | passing |
| Scala | metals | best-effort (cold-start; continue-on-error CI job) |
| Gleam | gleam (built-in lsp) | passing |
| Elixir | elixir-ls | best-effort (continue-on-error CI job) |
| Prisma | prisma-language-server | investigating (continue-on-error; server requires VS Code extension host) |
| SQL | sqls | passing (postgres:16 service container) |
| Clojure | clojure-lsp | passing |
| Nix | nil | passing |
| Dart | dart language-server | passing |
| MongoDB | mongodb-language-server | investigating (continue-on-error; server bundled in VS Code extension, not published standalone) |

---

## CI job structure

| Job | Languages | Runner |
|---|---|---|
| `multi-lang-core` | Go, TypeScript, Python, Rust, Java, Kotlin | ubuntu-latest |
| `multi-lang-extended` | C, C++, JavaScript, PHP, Ruby, YAML, JSON, Dockerfile, C#, CSS, HTML | ubuntu-latest |
| `multi-lang-zig` | Zig | ubuntu-latest |
| `multi-lang-terraform` | Terraform | ubuntu-latest |
| `multi-lang-lua` | Lua | ubuntu-latest |
| `multi-lang-swift` | Swift | macos-latest |
| `multi-lang-scala` | Scala | ubuntu-latest (continue-on-error) |
| `multi-lang-gleam` | Gleam | ubuntu-latest |
| `multi-lang-elixir` | Elixir | ubuntu-latest (continue-on-error) |
| `multi-lang-prisma` | Prisma | ubuntu-latest (continue-on-error) |
| `multi-lang-sql` | SQL | ubuntu-latest (postgres:16 service) |
| `multi-lang-clojure` | Clojure | ubuntu-latest |
| `multi-lang-nix` | Nix | ubuntu-latest |
| `multi-lang-dart` | Dart | ubuntu-latest |
| `multi-lang-mongodb` | MongoDB | ubuntu-latest (continue-on-error) |
| `speculative-test` | session lifecycle (gopls) | ubuntu-latest |

---

## Adding a language: what's required

Each new language needs three things:

1. **`langConfig` entry** in `test/multi_lang_test.go` `buildLanguageConfigs()`:
   - `binary` (language server executable name)
   - `serverArgs` (e.g. `[]string{"--stdio"}`)
   - `fixture` directory path
   - `file` path (primary fixture file)
   - `hoverLine/hoverColumn` — position of a named symbol in the primary file
   - `definitionLine/definitionColumn` — position of a symbol whose definition is in secondFile
   - `referenceLine/referenceColumn` — position to query for references
   - `completionLine/completionColumn` — position inside a method call for completions
   - `workspaceSymbol` — a symbol name that workspace symbol search should return
   - `secondFile` — cross-file fixture (for definition + references across files)
   - `supportsFormatting` — whether the server formats documents
   - `declarationLine/declarationColumn` — optional, for C-style go_to_declaration
   - `highlightLine/highlightColumn` — position for document highlight testing
   - `inlayHintEndLine` — end line for inlay hint range
   - `renameSymbolLine/renameSymbolColumn/renameSymbolName` — position and new name for rename testing (set to 0 to skip)
   - `codeActionLine/codeActionEndLine` — line range for code action testing

2. **Fixture files** in `test/fixtures/<lang>/`:
   - A primary file with a `Person` class/struct (or similar named symbol)
   - A `greeter` cross-file that imports and calls `Person`
   - A build/project file if the language server requires one (e.g. `go.mod`, `build.zig`, `Package.swift`, `build.sbt`)
   - Follow the pattern of existing fixtures (hover target, definition cross-ref, completion context)

3. **CI install step** in the appropriate `.github/workflows/ci.yml` job:
   - JVM-based (Java, Kotlin) → `multi-lang-core` (Java already set up)
   - Lightweight npm/binary → `multi-lang-extended`
   - macOS-only → dedicated job with `runs-on: macos-latest`
   - Heavy/slow startup → dedicated job with `continue-on-error: true`
   - Everything else → dedicated job (keeps extended job install time bounded)

---

## Tier 3 — Next expansion candidates

### Bash (bash-language-server)
- **Install:** `npm install -g bash-language-server`
- **Binary:** `bash-language-server`, language ID `shellscript`
- **Fixture:** `test/fixtures/bash/` — simple script with functions
- **Notes:** Good hover and completions. Definition/references limited.

### Haskell (haskell-language-server)
- **Install:** `ghcup install hls` — slow and fragile in CI
- **Blocker:** ghcup setup adds 5+ minutes; GHC version matrix complexity

---

## Tier 4 — Complex / skip for now

| Language | Server | Blocker |
|---|---|---|
| Haskell | haskell-language-server | ghcup setup is slow and fragile in CI |
| OCaml | ocamllsp | opam setup nontrivial |
| Elm | elm-language-server | Niche; requires elm + elm-format |
| R | r-languageserver | Niche; R package install in CI adds complexity |

---

## Language expansion summary

| Tier | Languages | Count |
|---|---|---|
| Current | TypeScript, Python, Go, Rust, Java, C, PHP, C++, JavaScript, Ruby, YAML, JSON, Dockerfile, C#, Kotlin, Lua, Swift, Zig, CSS, HTML, Terraform, Scala, Gleam, Elixir, Prisma, SQL, Clojure, Nix, Dart, MongoDB | **30** |
| Tier 3 candidates | Bash | 1 |
| **Potential total** | | **31** |

The 30-language set covers systems, web, JVM, scripting, infrastructure, config, functional, schema, query, document-database, and Nix/functional-package-manager domains.
