---
saw_name: '[SAW:wave2:agent-G] ## Task'
---

# Agent G Brief - Wave 2

**IMPL Doc:** /Users/dayna.blackwell/code/LSP-MCP-GO/docs/IMPL/IMPL-tier2-lang-expansion.yaml

## Files Owned

- `test/multi_lang_test.go`
- `.github/workflows/ci.yml`
- `README.md`
- `docs/language-support.md`


## Task

## Task
Wire all 6 new languages into test configuration and documentation.

## What to implement
Update 4 files:
1. test/multi_lang_test.go — Add 6 langConfig entries to buildLanguageConfigs()
2. .github/workflows/ci.yml — Add language server install steps  
3. README.md — Update language count badge (7 to 13) and table
4. docs/language-support.md — Move Tier 2 languages to Current section

## Interfaces consumed
All 6 fixture directories must exist before this agent runs (Wave 1 completion):
- test/fixtures/cpp/
- test/fixtures/javascript/
- test/fixtures/ruby/
- test/fixtures/yaml/
- test/fixtures/json/
- test/fixtures/dockerfile/

## langConfig specifications
Add 6 entries to buildLanguageConfigs() in test/multi_lang_test.go after PHP entry (line 276).

For each language, add a struct literal following the existing pattern. Key fields:
- C++: name="C++", id="cpp", binary="clangd", symbolName="Person"
- JavaScript: name="JavaScript", id="javascript", binary="typescript-language-server", serverArgs=["--stdio"]
- Ruby: name="Ruby", id="ruby", binary="solargraph", serverArgs=["stdio"]
- YAML: name="YAML", id="yaml", binary="yaml-language-server", definitionLine=0 (limited definition support)
- JSON: name="JSON", id="json", binary="vscode-json-language-server", definitionLine=0
- Dockerfile: name="Dockerfile", id="dockerfile", binary="docker-langserver", definitionLine=0

After adding entries, review the actual fixture files created by Wave 1 agents and
adjust hoverLine, definitionLine, callSiteLine, referenceLine, completionLine to
match the actual line numbers in those files. The estimates in this spec may not
match the final fixtures.

## CI install steps
Add 4 install steps to .github/workflows/ci.yml in multi-lang-test job after line 80
(after "sudo chmod +x /usr/local/bin/jdtls" and before "- name: Run multi-language tests"):

- name: Install solargraph (Ruby)
  run: gem install solargraph
- name: Install yaml-language-server  
  run: npm install -g yaml-language-server
- name: Install vscode-json-language-server
  run: npm install -g vscode-json-language-server
- name: Install dockerfile-language-server-nodejs
  run: npm install -g dockerfile-language-server-nodejs

## README.md updates
1. Line 6: Change badge from languages-7_verified to languages-13_verified
2. Line 12: Change "7 languages" to "13 languages"
3. Lines 87-96: Add 6 rows to language table after PHP row:
   C++, JavaScript, Ruby (full Tier 2), YAML, JSON, Dockerfile (Tier 1 only)

## docs/language-support.md updates
1. Line 3: Change heading from "Current (7 languages, CI-tested)" to "Current (13 languages, CI-tested)"
2. Lines 44-81: Move Tier 2 section content into Current section
3. Lines 127-133: Update target table to show 13 current languages and add Tier 3 row

## Testing strategy
After all updates, verify:
1. test/multi_lang_test.go compiles (no syntax errors)
2. CI workflow is valid YAML
3. README table has 13 language rows
4. docs/language-support.md Current section lists 13 languages

Postcondition checks:
- grep -c "gem install solargraph" .github/workflows/ci.yml should return 1
- grep -c "languages-13_verified" README.md should return 1
- grep -c "Current (13 languages" docs/language-support.md should return 1

## Verification gate
Build: GOWORK=off go build ./...
Lint: GOWORK=off go vet ./...
Postconditions: CI install check, README badge check, docs heading check (above)

## Constraints
- Do NOT modify fixture files (owned by Wave 1 agents)
- After adding langConfig entries, review each fixtures actual line numbers and adjust
- YAML, JSON, Dockerfile are Tier 1 only (set definitionLine/callSiteLine/referenceLine to 0)
- Do NOT run tests yet — CI will run full test suite post-merge

## Files owned
- test/multi_lang_test.go
- .github/workflows/ci.yml
- README.md
- docs/language-support.md



## Interface Contracts

### langConfig struct extension

Each new language must provide a langConfig entry compatible with the
existing struct in test/multi_lang_test.go. The struct is stable and
fully defined (lines 19-46).

```
type langConfig struct {
  name               string   // Display name (e.g., "C++")
  id                 string   // LSP language ID (e.g., "cpp")
  binary             string   // Language server binary name
  serverArgs         []string // Args to pass to server
  javaHome           string   // Optional, Java only
  fixture            string   // Absolute path to fixture dir
  file               string   // Absolute path to primary source file
  hoverLine          int      // Line number for hover test (1-based)
  hoverColumn        int      // Column number for hover test (1-based)
  definitionLine     int      // Expected definition line
  definitionColumn   int      // Expected definition column
  callSiteLine       int      // Line with cross-file call
  callSiteColumn     int      // Column of cross-file call
  callSiteFile       string   // File with cross-file call (optional)
  referenceLine      int      // Line to query references from
  referenceColumn    int      // Column for references
  completionLine     int      // Line for completion test
  completionColumn   int      // Column for completion test
  completionFile     string   // File for completion (optional)
  workspaceSymbol    string   // Symbol to search for (e.g., "Person")
  supportsFormatting bool     // Whether server formats
  secondFile         string   // Cross-file fixture path
  symbolName         string   // Symbol name for document symbols test
  declarationLine    int      // Optional, for go_to_declaration
  declarationColumn  int      // Optional, for go_to_declaration
}

```

### Fixture pattern

Every language fixture must follow the Person/Greeter cross-file pattern:
- Primary file defines a Person class/struct with name and age fields
- Primary file defines an add() function for completions
- Secondary file (greeter) imports Person and uses it
This ensures cross-file definition, references, and workspace symbol tests pass.

```
Primary file:
- Person type (struct/class/interface) with name and age
- add(x, y) function
- Hover target at Person declaration
- Completion target inside add() or similar

Secondary file (greeter):
- Import Person from primary file
- Use Person in a function or class
- Call site for go_to_definition test

```



## Quality Gates

Level: standard

- **build**: `GOWORK=off go build ./...` (required: true)
- **lint**: `GOWORK=off go vet ./...` (required: true)
- **test**: `GOWORK=off go test -v -run TestMultiLanguage ./test/ -timeout 15m` (required: true)

