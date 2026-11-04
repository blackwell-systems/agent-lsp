# Language Support

## Install reference

| Language | Server | Install |
|----------|--------|---------|
| TypeScript / JavaScript | `typescript-language-server` | `npm i -g typescript-language-server typescript@6` |
| Python | `pyright-langserver` | `npm i -g pyright` |
| Go | `gopls` | `go install golang.org/x/tools/gopls@latest` |
| Rust | `rust-analyzer` | `rustup component add rust-analyzer` |
| C / C++ | `clangd` | `apt install clangd` / `brew install llvm` |
| Ruby | `solargraph` | `gem install solargraph` |
| PHP | `intelephense` | `npm i -g intelephense` |
| Java | `jdtls` | [eclipse.jdt.ls snapshots](https://download.eclipse.org/jdtls/snapshots/) (current snapshots need Java 25 to run) |
| YAML | `yaml-language-server` | `npm i -g yaml-language-server` |
| JSON | `vscode-json-language-server` | `npm i -g vscode-langservers-extracted` |
| Dockerfile | `docker-langserver` | `npm i -g dockerfile-language-server-nodejs` |
| C# | `csharp-ls` | `dotnet tool install -g csharp-ls` |
| Kotlin | `kotlin-language-server` | [GitHub releases](https://github.com/fwcd/kotlin-language-server/releases) (1.3.13 runs on Java 21) |
| Lua | `lua-language-server` | [GitHub releases](https://github.com/LuaLS/lua-language-server/releases) |
| MQL4 / MQL5 | `mql-lsp-server` | [GitHub releases](https://github.com/davalillo/mql-language-server/releases) |
| Luau | `luau-lsp` | [GitHub releases](https://github.com/JohnnyMorganz/luau-lsp/releases) |
| Swift | `sourcekit-lsp` | Ships with Xcode / Swift toolchain |
| Zig | `zls` | [GitHub releases](https://github.com/zigtools/zls/releases) (match Zig version) |
| CSS | `vscode-css-language-server` | `npm i -g vscode-langservers-extracted` |
| HTML | `vscode-html-language-server` | `npm i -g vscode-langservers-extracted` |
| Terraform | `terraform-ls` | [releases.hashicorp.com](https://releases.hashicorp.com/terraform-ls/) |
| Scala | `metals` | `cs install metals` ([Coursier](https://get-coursier.io)); importing an sbt project also needs `sbt` |
| Gleam | `gleam` (built-in) | [GitHub releases](https://github.com/gleam-lang/gleam/releases) |
| Elixir | `elixir-ls` | [GitHub releases](https://github.com/elixir-lsp/elixir-ls/releases) |
| Prisma | `prisma-language-server` | `npm i -g @prisma/language-server` |
| SQL | `sqls` | `go install github.com/sqls-server/sqls@latest` |
| Clojure | `clojure-lsp` | [GitHub releases](https://github.com/clojure-lsp/clojure-lsp/releases) |
| Nix | `nil` | [GitHub releases](https://github.com/oxalica/nil/releases) |
| Dart | `dart language-server` | Ships with Dart SDK (`brew install dart`) |
| MongoDB | `mongodb-language-server` | Extract from the MongoDB VS Code extension VSIX (not published on npm); see [MongoDB](#mongodb-mongodb-language-server) below |

TypeScript 7 ships without `tsserver`, which typescript-language-server needs, so install TypeScript 6 (`typescript@6`) until that changes. A project with TypeScript 6 in its own `node_modules` works either way.

### Auto-detection coverage

Running agent-lsp with no arguments (or `--auto`) auto-detects only the servers listed in `internal/config/autodetect.go`: `gopls` (Go), `typescript-language-server` (TypeScript, JavaScript), `pyright-langserver` (Python), `rust-analyzer` (Rust), `clangd` (C, C++), `solargraph` (Ruby), `yaml-language-server` (YAML), `vscode-json-language-server` (JSON), `docker-langserver` (Dockerfile), `csharp-ls` (C#), `jdtls` (Java), `kotlin-language-server` (Kotlin), `intelephense` (PHP), and `mql-lsp-server` (MQL4/MQL5).

Every other language in the table above needs an explicit server: pass `language:binary` arguments (for example `agent-lsp lua:lua-language-server`, with extra server args after commas: `css:vscode-css-language-server,--stdio`), or a config file with `--config /path/to/lsp-mcp.json` (or `--merge-config` to overlay it on the auto-detected servers).

### MongoDB (mongodb-language-server)

`@mongodb-js/mongodb-language-server` is not on npm. The server ships inside the [mongodb-js/vscode](https://github.com/mongodb-js/vscode/releases) extension; CI extracts it from the VSIX and wraps it in a `mongodb-language-server` script:

```bash
MDBVSC_VERSION=1.18.0   # CI pins this version and verifies the VSIX sha256
wget -q "https://github.com/mongodb-js/vscode/releases/download/v${MDBVSC_VERSION}/mongodb-vscode-${MDBVSC_VERSION}.vsix" -O /tmp/mongodb-vscode.vsix
sudo mkdir -p /opt/mongodb-vscode
sudo unzip -q /tmp/mongodb-vscode.vsix -d /opt/mongodb-vscode
printf '#!/bin/bash\nexec node /opt/mongodb-vscode/extension/dist/languageServer.js "$@"\n' \
  | sudo tee /usr/local/bin/mongodb-language-server > /dev/null
sudo chmod +x /usr/local/bin/mongodb-language-server
```

Requires Node.js on `PATH`. MongoDB is not auto-detected, so pass it explicitly, for example `agent-lsp mongodb:mongodb-language-server,--stdio` (see [Auto-detection coverage](#auto-detection-coverage)).

### MQL (mql-lsp-server)

agent-lsp auto-detects servers on `PATH`, so installing MQL support is just
placing the self-contained binary there — no .NET runtime required:

```bash
curl -fsSL https://github.com/davalillo/mql-language-server/releases/latest/download/mql-lsp-server-linux-x64 \
  -o ~/.local/bin/mql-lsp-server
chmod +x ~/.local/bin/mql-lsp-server
mql-lsp-server --version   # verify: "MQL Language Server 2.4.2" or newer
```

Assets for other platforms are in the same release: `mql-lsp-server-win-x64.exe`
(rename to `mql-lsp-server.exe`), `mql-lsp-server-osx-x64`,
`mql-lsp-server-osx-arm64`. If you already have the .NET 10 SDK,
`dotnet tool install -g mql-language-server` installs the same `mql-lsp-server`
command. Verify with `agent-lsp doctor`.

---

## CI tool coverage matrix

Tier 1 (`start_lsp`, `open_document`, `get_diagnostics`, `inspect_symbol`) verified for all 32 languages (MongoDB is checked through completion, the only feature its server offers). Tier 2: 34 additional tools.

The cells below are generated from CI's own results (the summary table `TestMultiLanguage` prints), not written by hand: `pass` passed, `—` was skipped because the server does not support the feature or the test has no fixture position for it, `fail` failed. Regenerate after a CI run with `gh run view <run-id> --log > ci.log && python3 scripts/update-coverage-matrix.py ci.log`.

| Language | Tier 1 | symbols | definition | references | completions | workspace | format | declaration | type_hierarchy | hover | call_hier | sem_tok | sig_help |
|----------|--------|---------|------------|------------|-------------|-----------|--------|-------------|----------------|-------|-----------|---------|----------|
| TypeScript | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | pass | — |
| Python | pass | pass | pass | pass | pass | pass | — | — | — | pass | pass | — | pass |
| Go | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | — | pass |
| Rust | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | pass | — |
| Java | pass | pass | — | pass | pass | pass | pass | — | — | pass | pass | pass | — |
| C | pass | pass | — | pass | pass | pass | pass | pass | — | pass | — | pass | — |
| PHP | pass | pass | pass | pass | pass | pass | — | — | — | pass | — | — | — |
| C++ | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | pass | — |
| JavaScript | pass | pass | pass | pass | pass | pass | pass | — | — | pass | pass | pass | — |
| Ruby | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | — | — |
| YAML | pass | pass | — | — | pass | — | pass | — | — | pass | — | — | — |
| JSON | pass | pass | — | — | pass | — | pass | — | — | pass | — | — | — |
| Dockerfile | pass | pass | — | — | pass | — | — | — | — | pass | — | pass | — |
| C# | pass | — | pass | pass | pass | pass | pass | — | — | pass | — | pass | — |
| Kotlin | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | pass | — |
| Lua | pass | pass | — | pass | pass | pass | pass | — | — | pass | — | pass | — |
| Luau | pass | pass | pass | pass | pass | pass | — | — | — | pass | — | pass | — |
| Swift | pass | pass | — | pass | pass | pass | pass | — | — | pass | pass | pass | — |
| Zig | pass | pass | pass | pass | pass | — | pass | — | — | pass | — | pass | pass |
| CSS | pass | pass | pass | pass | pass | — | pass | — | — | pass | — | — | — |
| HTML | pass | — | — | — | pass | — | pass | — | — | pass | — | — | — |
| Terraform | pass | pass | — | pass | pass | pass | — | — | — | pass | — | — | — |
| Scala | pass | — | pass | pass | pass | pass | pass | — | — | pass | pass | pass | — |
| Gleam | pass | pass | pass | pass | pass | — | pass | — | — | pass | — | — | — |
| Elixir | pass | pass | pass | pass | pass | pass | pass | — | — | pass | pass | — | pass |
| Prisma | pass | pass | pass | pass | pass | — | pass | — | — | pass | — | — | — |
| SQL | pass | — | — | — | pass | — | — | — | — | pass | — | — | — |
| Clojure | pass | pass | pass | pass | pass | pass | pass | — | — | pass | pass | pass | pass |
| Nix | pass | — | — | — | pass | — | — | — | — | pass | — | pass | — |
| Dart | pass | pass | pass | pass | pass | pass | pass | — | — | pass | — | pass | pass |
| MongoDB | pass | — | — | — | pass | — | — | — | — | — | — | — | — |
| MQL | pass | pass | pass | pass | pass | pass | — | — | — | pass | pass | — | pass |

Note: MQL references and rename via `mql-lsp-server` are name-based (not type-resolved). This limitation is tracked upstream ([davalillo/mql-language-server#45](https://github.com/davalillo/mql-language-server/issues/45)).

See [ci-notes.md](../architecture/ci-notes.md) for per-language CI quirks.

---

## Current (32 languages, CI-verified)

**stable** = all Tier 1 tools pass CI. **experimental** = server works but CI results are informational.

| Language | Language Server | Status |
|---|---|---|
| TypeScript | typescript-language-server | stable |
| Python | pyright-langserver | stable |
| Go | gopls | stable |
| Rust | rust-analyzer | stable |
| Java | jdtls | stable (Java 25; cold-start indexing is slow) |
| C | clangd | stable |
| PHP | intelephense | stable |
| C++ | clangd | stable |
| JavaScript | typescript-language-server | stable |
| Ruby | solargraph | stable |
| YAML | yaml-language-server | stable |
| JSON | vscode-json-language-server | stable (hover and diagnostics need a JSON schema; the fixture names one with `$schema`) |
| Dockerfile | docker-langserver | stable |
| C# | csharp-ls | stable |
| Kotlin | kotlin-language-server | stable |
| Lua | lua-language-server | stable |
| Luau | luau-lsp | stable |
| Swift | sourcekit-lsp | stable (macos-latest runner) |
| Zig | zls | stable |
| CSS | vscode-css-language-server | stable |
| HTML | vscode-html-language-server | stable |
| Terraform | terraform-ls | stable |
| Scala | metals | stable (agent-lsp sets Metals `autoImportBuilds`, so the sbt build is imported on first open; the import takes a minute or more) |
| Gleam | gleam (built-in lsp) | stable |
| Elixir | elixir-ls | experimental |
| Prisma | prisma-language-server | experimental |
| SQL | sqls | stable (postgres:16 service container) |
| Clojure | clojure-lsp | stable |
| Nix | nil | experimental |
| Dart | dart language-server | stable |
| MongoDB | mongodb-language-server | stable (completion only: the server offers no hover or diagnostics) |
| MQL | mql-lsp-server | experimental (name-based references/rename) |

---

## CI job structure

| Job | Languages | Runner |
|---|---|---|
| `multi-lang-core` | Go, TypeScript, Python, Rust, Kotlin | ubuntu-latest |
| `multi-lang-java` | Java | ubuntu-latest (continue-on-error) |
| `multi-lang-extended` | C, C++, JavaScript, PHP, Ruby, YAML, JSON, Dockerfile, C#, CSS, HTML | ubuntu-latest |
| `multi-lang-zig` | Zig | ubuntu-latest |
| `multi-lang-terraform` | Terraform | ubuntu-latest |
| `multi-lang-lua` | Lua | ubuntu-latest |
| `multi-lang-mql` | MQL | ubuntu-latest |
| `multi-lang-luau` | Luau | ubuntu-latest |
| `multi-lang-swift` | Swift | macos-latest |
| `multi-lang-scala` | Scala | ubuntu-latest (continue-on-error) |
| `multi-lang-gleam` | Gleam | ubuntu-latest |
| `multi-lang-elixir` | Elixir | ubuntu-latest (continue-on-error) |
| `multi-lang-prisma` | Prisma | ubuntu-latest (continue-on-error) |
| `multi-lang-sql` | SQL | ubuntu-latest (postgres:16 service) |
| `multi-lang-clojure` | Clojure | ubuntu-latest |
| `multi-lang-nix` | Nix | ubuntu-latest (continue-on-error) |
| `multi-lang-dart` | Dart | ubuntu-latest |
| `multi-lang-mongodb` | MongoDB | ubuntu-latest (continue-on-error) |
| `speculative-test` | Go, TypeScript, Python, Rust, C++, C#, Dart, Java (speculative sessions) | ubuntu-latest |

---

## Adding a language: what's required

Each new language needs three things:

1. **`langConfig` entry** in `buildLanguageConfigs()` in `test/lang_configs_test.go` (the `langConfig` type itself is defined in `test/multi_lang_test.go`):
   - `binary` (language server executable name)
   - `serverArgs` (e.g. `[]string{"--stdio"}`)
   - `fixture` directory path
   - `file` path (primary fixture file)
   - `hoverLine/hoverColumn`: position of a named symbol in the primary file
   - `definitionLine/definitionColumn`: position of a symbol whose definition is in secondFile
   - `referenceLine/referenceColumn`: position to query for references
   - `completionLine/completionColumn`: position inside a method call for completions
   - `workspaceSymbol`: a symbol name that workspace symbol search should return
   - `secondFile`: cross-file fixture (for definition + references across files)
   - `supportsFormatting`: whether the server formats documents
   - `declarationLine/declarationColumn`: optional, for C-style go_to_declaration
   - `highlightLine/highlightColumn`: position for document highlight testing
   - `inlayHintEndLine`: end line for inlay hint range
   - `renameSymbolLine/renameSymbolColumn/renameSymbolName`: position and new name for rename testing (set to 0 to skip)
   - `codeActionLine/codeActionEndLine`: line range for code action testing

2. **Fixture files** in `test/fixtures/<lang>/`:
   - A primary file with a `Person` class/struct (or similar named symbol)
   - A `greeter` cross-file that imports and calls `Person`
   - A build/project file if the language server requires one (e.g. `go.mod`, `build.zig`, `Package.swift`, `build.sbt`)
   - Follow the pattern of existing fixtures (hover target, definition cross-ref, completion context)

3. **CI install step** in the appropriate `.github/workflows/ci.yml` job:
   - JVM-based: Java → `multi-lang-java`, Kotlin → `multi-lang-core`
   - Lightweight npm/binary → `multi-lang-extended`
   - macOS-only → dedicated job with `runs-on: macos-latest`
   - Heavy/slow startup → dedicated job with `continue-on-error: true`
   - Everything else → dedicated job (keeps extended job install time bounded)

---

## Tier 3: next expansion candidates

### Bash (bash-language-server)
- **Install:** `npm install -g bash-language-server`
- **Binary:** `bash-language-server`, language ID `shellscript`
- **Fixture:** `test/fixtures/bash/`, simple script with functions
- **Notes:** Good hover and completions. Definition/references limited.

### Haskell (haskell-language-server)
- **Install:** `ghcup install hls` (slow and fragile in CI)
- **Blocker:** ghcup setup adds 5+ minutes; GHC version matrix complexity

---

## Tier 4: complex / skip for now

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
| Current | TypeScript, Python, Go, Rust, Java, C, PHP, C++, JavaScript, Ruby, YAML, JSON, Dockerfile, C#, Kotlin, Lua, Luau, Swift, Zig, CSS, HTML, Terraform, Scala, Gleam, Elixir, Prisma, SQL, Clojure, Nix, Dart, MongoDB, MQL | **32** |
| Tier 3 candidates | Bash | 1 |
| **Potential total** | | **33** |

The 32-language set covers systems, web, JVM, scripting, infrastructure, config, functional, schema, query, document-database, Nix/functional-package-manager, and MQL (MetaTrader) domains.
