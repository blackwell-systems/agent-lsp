package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// repoFile reads a file relative to the repository root (two levels up from
// cmd/agent-lsp).
func repoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	return string(b)
}

// The tool reference must document exactly the registered tools: a tool
// without a section is invisible to users, and a section for a tool that does
// not exist (get_editing_context was documented for months) misleads them.
func TestDocsToolReferenceMatchesRegistry(t *testing.T) {
	registered := map[string]bool{}
	for _, name := range registeredTools(t) {
		registered[name] = true
	}
	doc := repoFile(t, "docs/reference/tools.md")
	documented := map[string]bool{}
	for _, m := range regexp.MustCompile("(?m)^### `([a-z_]+)`").FindAllStringSubmatch(doc, -1) {
		documented[m[1]] = true
	}
	var missing, unknown []string
	for name := range registered {
		if !documented[name] {
			missing = append(missing, name)
		}
	}
	for name := range documented {
		if !registered[name] {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(unknown)
	if len(missing) > 0 {
		t.Errorf("registered tools with no section in docs/reference/tools.md: %v", missing)
	}
	if len(unknown) > 0 {
		t.Errorf("docs/reference/tools.md documents tools that are not registered: %v", unknown)
	}
}

// Headline tool and skill counts on the front-door pages must match the code.
// Historical mentions (CHANGELOG, adopter quotes) are not checked.
func TestDocsHeadlineCounts(t *testing.T) {
	skills := len(loadSkills())
	// Totals only: "66 tools", "66 code intelligence tools", the CI badge
	// ("tools-66%2F66") and the README capability row. Subset counts ("**8
	// tools**, simulate ...") are not matched.
	toolRe := regexp.MustCompile(`(?:^|[^*\w])(\d+) (?:code intelligence )?tools\b|tools-(\d+)%2F(\d+)|\| Tools \| \*\*(\d+)\*\* \|`)
	skillRe := regexp.MustCompile(`\b(\d+) (?:workflow skills|agent workflows|multi-step workflow skills)\b`)
	files := []string{
		"README.md",
		"docs/index.md",
		"docs/reference/tools.md",
		"docs/getting-started/quickstart.md",
		"docs/architecture/architecture.md",
		"pypi/README.md",
		"npm/agent-lsp/README.md",
		"CLAUDE.md",
	}
	for _, f := range files {
		text := repoFile(t, f)
		for _, m := range toolRe.FindAllStringSubmatch(text, -1) {
			for _, g := range m[1:] {
				if g == "" {
					continue
				}
				if n, _ := strconv.Atoi(g); n != registeredToolCount {
					t.Errorf("%s: %q, but %d tools are registered", f, strings.TrimSpace(m[0]), registeredToolCount)
				}
			}
		}
		for _, m := range skillRe.FindAllStringSubmatch(text, -1) {
			if n, _ := strconv.Atoi(m[1]); n != skills {
				t.Errorf("%s: %q, but there are %d skills", f, m[0], skills)
			}
		}
	}
}

// notYetVerified lists the configured test languages that do not pass end to
// end in CI. Remove a language when it does, and update the docs' counts.
var notYetVerified = map[string]bool{"Scala": true}

// Headline "N CI-verified languages" claims must match the languages that run
// and pass in CI: the configured test languages minus notYetVerified. The
// docs said 30, 31 and 32 in different places while 25 actually ran.
func TestDocsLanguageCounts(t *testing.T) {
	configs := regexp.MustCompile(`(?m)^\t\t\tname: +"([^"]+)"`).FindAllStringSubmatch(repoFile(t, "test/lang_configs_test.go"), -1)
	if len(configs) == 0 {
		t.Fatal("no language configs found in test/lang_configs_test.go")
	}
	verified := 0
	for _, c := range configs {
		if !notYetVerified[c[1]] {
			verified++
		}
	}
	re := regexp.MustCompile(`\b(\d+) CI-verified languages\b|languages-(\d+)_CI--verified|\| Languages \(CI-verified\) \| \*\*(\d+)\*\*`)
	for _, f := range []string{"README.md", "docs/index.md", "pypi/README.md", "npm/agent-lsp/README.md", "server.json", "pypi/pyproject.toml"} {
		for _, m := range re.FindAllStringSubmatch(repoFile(t, f), -1) {
			for _, g := range m[1:] {
				if g == "" {
					continue
				}
				if n, _ := strconv.Atoi(g); n != verified {
					t.Errorf("%s: %q, but %d of %d configured languages are verified in CI", f, m[0], verified, len(configs))
				}
			}
		}
	}
}
