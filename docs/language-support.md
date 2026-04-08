# Language Support

## Current (18 languages, CI-tested)

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
| Scala | metals | ⚠️ best-effort (cold-start; continue-on-error CI job) |

---

## CI job structure

| Job | Languages | Runner |
|---|---|---|
| `multi-lang-core` | Go, TypeScript, Python, Rust, Java, Kotlin | ubuntu-latest |
| `multi-lang-extended` | C, C++, JavaScript, PHP, Ruby, YAML, JSON, Dockerfile, CSharp | ubuntu-latest |
| `multi-lang-lua` | Lua | ubuntu-latest |
| `multi-lang-swift` | Swift | macos-latest |
| `multi-lang-scala` | Scala | ubuntu-latest (continue-on-error) |

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
   - `declarationLine/declarationColumn` + `supportsDeclaration` — optional, for C-style go_to_declaration
   - `highlightLine/highlightColumn` — position for document highlight testing
   - `inlayHintEndLine` — end line for inlay hint range
   - `renameSymbolLine/renameSymbolColumn/renameSymbolName` — position and new name for rename testing
   - `codeActionLine/codeActionEndLine` — line range for code action testing

2. **Fixture files** in `test/fixtures/<lang>/`:
   - A primary file with a `Person` class/struct (or similar named symbol)
   - A `greeter` cross-file that imports and calls `Person`
   - Follow the pattern of existing fixtures (each has a hover target, a definition cross-ref, and a completion context)

3. **CI install step** in the appropriate `.github/workflows/ci.yml` job based on language runtime:
   - JVM-based languages (Java, Kotlin, Scala) → `multi-lang-core` or the scala job
   - Lightweight npm/binary-installed servers → `multi-lang-extended`
   - Special runtime requirements → dedicated job (e.g. Swift requires macOS runner, Lua has its own job)

---

## Tier 3 — Next expansion candidates

### Zig (zls)
- **Install:** `sudo snap install zls --classic` or download binary from GitHub releases
- **Binary:** `zls`, language ID `zig`
- **Fixture:** `test/fixtures/zig/src/` — `person.zig`, `greeter.zig`, `build.zig`
- **Notes:** Full Tier 1+2. zls has excellent Go-to-definition support.

### CSS (vscode-css-language-server)
- **Install:** `npm install -g vscode-css-language-server`
- **Binary:** `vscode-css-language-server --stdio`, language ID `css`
- **Fixture:** `test/fixtures/css/` — a stylesheet with named custom properties and classes
- **Notes:** Hover and completions work well. Definition/references limited.

### HTML (vscode-html-language-server)
- **Install:** `npm install -g vscode-html-language-server`
- **Binary:** `vscode-html-language-server --stdio`, language ID `html`
- **Notes:** Embedded language handling (CSS/JS in HTML). Tier 1 only initially.

---

## Tier 4 — Complex / skip for now

| Language | Server | Blocker |
|---|---|---|
| Haskell | haskell-language-server | ghcup setup is slow and fragile in CI |
| OCaml | ocamllsp | opam setup nontrivial |
| Elm | elm-language-server | Niche; requires elm + elm-format |

---

## Language expansion summary

| Tier | Languages | Count |
|---|---|---|
| Current | TypeScript, Python, Go, Rust, Java, C, PHP, C++, JavaScript, Ruby, YAML, JSON, Dockerfile, C#, Kotlin, Lua, Swift, Scala | **18** |
| Tier 3 candidates | Zig, CSS, HTML | 3 |
| **Potential total** | | **21** |

The current 18-language set covers the most common development scenarios. Tier 3 expansion would add 3 more languages with one additional binary install each.
