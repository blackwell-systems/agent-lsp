package phase

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// skillMDPermissions is the tool_permissions block of a SKILL.md frontmatter.
type skillMDPermissions struct {
	phaseOrder      []string
	allowed         map[string][]string
	forbidden       map[string][]string
	globalForbidden []string
}

// parseSkillMDPermissions reads the tool_permissions YAML from a SKILL.md
// frontmatter. The block has a fixed shape (phases: name: allowed/forbidden
// lists, then global_forbidden), so an indentation reader is enough.
func parseSkillMDPermissions(t *testing.T, path string) skillMDPermissions {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	p := skillMDPermissions{allowed: map[string][]string{}, forbidden: map[string][]string{}}
	in, inPhases := false, false
	var phase, list string
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if i := strings.Index(trimmed, " #"); i >= 0 {
			trimmed = strings.TrimSpace(trimmed[:i])
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		switch {
		case trimmed == "tool_permissions:":
			in = true
			continue
		case !in:
			continue
		case trimmed == "---" || (indent <= 2 && trimmed != "" && !strings.HasPrefix(trimmed, "-")):
			if trimmed != "phases:" && trimmed != "global_forbidden:" && indent <= 2 {
				return p
			}
		}
		switch {
		case trimmed == "phases:":
			inPhases = true
		case trimmed == "global_forbidden:":
			inPhases, phase, list = false, "", "global"
		case inPhases && indent == 6 && strings.HasSuffix(trimmed, ":"):
			phase = strings.TrimSuffix(trimmed, ":")
			p.phaseOrder = append(p.phaseOrder, phase)
		case strings.HasPrefix(trimmed, "allowed:") || strings.HasPrefix(trimmed, "forbidden:"):
			key, rest, _ := strings.Cut(trimmed, ":")
			list = key
			// Inline flow list: allowed: ["mcp__agent-lsp__start_lsp"]
			if rest = strings.TrimSpace(rest); strings.HasPrefix(rest, "[") {
				for _, item := range strings.Split(strings.Trim(rest, "[]"), ",") {
					if tool := strings.TrimPrefix(strings.Trim(strings.TrimSpace(item), `"'`), "mcp__agent-lsp__"); tool != "" {
						if key == "allowed" {
							p.allowed[phase] = append(p.allowed[phase], tool)
						} else {
							p.forbidden[phase] = append(p.forbidden[phase], tool)
						}
					}
				}
			}
		case strings.HasPrefix(trimmed, "- "):
			tool := strings.Trim(strings.TrimPrefix(trimmed, "- "), `"'`)
			tool = strings.TrimPrefix(tool, "mcp__agent-lsp__")
			switch {
			case list == "global":
				p.globalForbidden = append(p.globalForbidden, tool)
			case list == "allowed":
				p.allowed[phase] = append(p.allowed[phase], tool)
			case list == "forbidden":
				p.forbidden[phase] = append(p.forbidden[phase], tool)
			}
		}
	}
	return p
}

func sorted(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

// The phase rules are written twice: in each SKILL.md's tool_permissions
// (what agents read) and in BuiltinSkills (what is enforced). They drifted
// (preview_edit forbidden in code but not in SKILL.md); this keeps them equal.
func TestBuiltinSkillsMatchSkillMD(t *testing.T) {
	for _, cfg := range BuiltinSkills() {
		t.Run(cfg.SkillName, func(t *testing.T) {
			md := parseSkillMDPermissions(t, filepath.Join("..", "..", "skills", cfg.SkillName, "SKILL.md"))
			var order []string
			for _, ph := range cfg.Phases {
				order = append(order, ph.Name)
				if got, want := sorted(md.allowed[ph.Name]), sorted(ph.Allowed); !reflect.DeepEqual(got, want) {
					t.Errorf("phase %s allowed: SKILL.md %v, enforced %v", ph.Name, got, want)
				}
				if got, want := sorted(md.forbidden[ph.Name]), sorted(ph.Forbidden); !reflect.DeepEqual(got, want) {
					t.Errorf("phase %s forbidden: SKILL.md %v, enforced %v", ph.Name, got, want)
				}
			}
			if !reflect.DeepEqual(md.phaseOrder, order) {
				t.Errorf("phase order: SKILL.md %v, enforced %v", md.phaseOrder, order)
			}
			if got, want := sorted(md.globalForbidden), sorted(cfg.GlobalForbidden); !reflect.DeepEqual(got, want) {
				t.Errorf("global_forbidden: SKILL.md %v, enforced %v", got, want)
			}
		})
	}
}
