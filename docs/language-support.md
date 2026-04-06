# Language Support Roadmap

## Current (7 languages, CI-tested)

| Language | Language Server | Status |
|---|---|---|
| TypeScript | typescript-language-server | ✅ passing |
| Python | pyright-langserver | ✅ passing |
| Go | gopls | ✅ passing |
| Rust | rust-analyzer | ✅ passing |
| Java | jdtls | ⚠️ flaky (jdtls indexing timing) |
| C | clangd | ✅ passing |
| PHP | intelephense | ✅ passing |

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

2. **Fixture files** in `test/fixtures/<lang>/`:
   - A primary file with a `Person` class/struct (or similar named symbol)
   - A `greeter` cross-file that imports and calls `Person`
   - Follow the pattern of existing fixtures (each has a hover target, a definition cross-ref, and a completion context)

3. **CI install step** in `.github/workflows/ci.yml` `multi-lang-test` job.

---

## Tier 2 — Zero or one new install (next to add)

### C++ (clangd — already installed for C)
- **Install:** none — clangd is already in CI
- **Binary:** `clangd`, language ID `cpp`
- **Fixture:** `test/fixtures/cpp/` — `person.cpp`, `person.h`, `greeter.cpp`
- **Notes:** Add `compile_commands.json` or `build/compile_commands.json` so clangd resolves includes. Same fixture pattern as C.

### JavaScript (typescript-language-server — already installed)
- **Install:** none — typescript-language-server handles `.js` natively
- **Binary:** `typescript-language-server --stdio`, language ID `javascript`
- **Fixture:** `test/fixtures/javascript/src/` — plain `.js` files, no tsconfig needed
- **Notes:** Set `supportsFormatting: false` initially; prettier integration varies.

### Ruby (solargraph)
- **Install:** `gem install solargraph` (Ruby is on ubuntu-latest)
- **Binary:** `solargraph stdio`, language ID `ruby`
- **Fixture:** `test/fixtures/ruby/` — `person.rb`, `greeter.rb`
- **Notes:** `workspaceSymbol` search in solargraph requires the gems to be indexed; set `supportsFormatting: false`.

### YAML (yaml-language-server)
- **Install:** `npm install -g yaml-language-server`
- **Binary:** `yaml-language-server --stdio`, language ID `yaml`
- **Fixture:** `test/fixtures/yaml/` — a schema-validated YAML file with named keys
- **Notes:** Hover and completions require a JSON Schema; use the GitHub Actions workflow schema as a known fixture. Definition/references may not apply — set `supportsDeclaration: false`.

### JSON (vscode-json-language-server)
- **Install:** `npm install -g vscode-json-language-server`
- **Binary:** `vscode-json-language-server --stdio`, language ID `json`
- **Fixture:** `test/fixtures/json/` — `package.json` (schema already known to the server)
- **Notes:** Completions and hover work well; definition/references are limited. Tier 1 only.

### Dockerfile (dockerfile-language-server-nodejs)
- **Install:** `npm install -g dockerfile-language-server-nodejs`
- **Binary:** `docker-langserver --stdio`, language ID `dockerfile`
- **Fixture:** `test/fixtures/dockerfile/` — a multi-stage Dockerfile with named stages
- **Notes:** Tier 1 only (hover, diagnostics). References/symbols are limited.

---

## Tier 3 — One new binary (medium effort)

### Lua (lua-language-server)
- **Install:** `sudo apt-get install -y lua-language-server` or `sudo snap install lua-language-server`
- **Binary:** `lua-language-server`, language ID `lua`
- **Fixture:** `test/fixtures/lua/` — `person.lua`, `greeter.lua`
- **Notes:** Full Tier 1+2 support. Good hover and completion coverage.

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
| C# | omnisharp | Needs .NET runtime; multi-component install |
| Swift | sourcekit-lsp | macOS-only; CI runs Linux |
| Kotlin | kotlin-language-server | Needs coursier + JDK setup on top of Java |
| Scala | metals | Needs coursier; slow workspace indexing |
| Haskell | haskell-language-server | ghcup setup is slow and fragile in CI |
| OCaml | ocamllsp | opam setup nontrivial |
| Elm | elm-language-server | Niche; requires elm + elm-format |

---

## Target: 12 confirmed languages

After Tier 2 additions (C++, JavaScript, Ruby, YAML, JSON, Dockerfile):

| Tier | Languages | Count |
|---|---|---|
| Current | TypeScript, Python, Go, Rust, Java, C, PHP | 7 |
| +Tier 2 | C++, JavaScript, Ruby, YAML, JSON, Dockerfile | +6 |
| **Total** | | **13** |

This doubles the confirmed language count with approximately 6 new CI install lines and 6 fixture directories.
