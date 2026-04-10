# Language Support

## Current (25 languages, CI-tested)

| Language | Language Server | Status |
|---|---|---|
| TypeScript | typescript-language-server | ✅ passing |
| Python | pyright-langserver | ✅ passing |
| Go | gopls | ✅ passing |
| Rust | rust-analyzer | ✅ passing |
| Java | jdtls | ⚠️ flaky (cold-start indexing) |
| C | clangd | ✅ passing |
| PHP | intelephense | ✅ passing |
| C++ | clangd | ✅ passing |
| JavaScript | typescript-language-server | ✅ passing |
| Ruby | solargraph | ✅ passing |
| YAML | yaml-language-server | ✅ passing |
| JSON | vscode-json-language-server | ✅ passing |
| Dockerfile | docker-langserver | ✅ passing |
| C# | csharp-ls | ✅ passing |
| Kotlin | kotlin-language-server | ✅ passing |
| Lua | lua-language-server | ✅ passing |
| Swift | sourcekit-lsp | ✅ passing (macos-latest runner) |
| Zig | zls | ✅ passing |
| CSS | vscode-css-language-server | ✅ passing |
| HTML | vscode-html-language-server | ✅ passing |
| Terraform | terraform-ls | ✅ passing |
| Scala | metals | ⚠️ best-effort (cold-start; continue-on-error CI job) |
| Gleam | gleam (built-in lsp) | ✅ passing |
| Elixir | elixir-ls | ⚠️ best-effort (continue-on-error CI job) |
| Prisma | prisma-language-server | ✅ passing |

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
| `multi-lang-prisma` | Prisma | ubuntu-latest |
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
| Current | TypeScript, Python, Go, Rust, Java, C, PHP, C++, JavaScript, Ruby, YAML, JSON, Dockerfile, C#, Kotlin, Lua, Swift, Zig, CSS, HTML, Terraform, Scala, Gleam, Elixir, Prisma | **25** |
| Tier 3 candidates | SQL (sqls + live DB), Bash, R | 1–3 |
| **Potential total** | | **26–28** |

The 25-language set covers systems, web, JVM, scripting, infrastructure, config, functional, and schema domains.
