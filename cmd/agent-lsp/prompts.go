// prompts.go registers MCP prompts for each agent-lsp skill.
//
// Skills are defined as SKILL.md files under skills/. Each file has YAML
// frontmatter (name, description, argument-hint) and a Markdown body with
// the full workflow instructions. Prompts expose the same skills via the
// MCP protocol so any MCP client can discover them through prompts/list
// and retrieve the full instructions via prompts/get.
//
// Context budget: prompts/list returns only the short description from
// frontmatter. The full SKILL.md body is returned only when prompts/get
// is called for a specific prompt, keeping the listing lightweight.
package main

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/agent-lsp/skills"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// skillMeta holds parsed frontmatter from a SKILL.md file.
type skillMeta struct {
	Name         string
	Description  string
	ArgumentHint string
	Body         string // Markdown body after frontmatter
}

// parseSkillMD splits a SKILL.md into frontmatter fields and body.
func parseSkillMD(content string) (skillMeta, bool) {
	if !strings.HasPrefix(content, "---") {
		return skillMeta{}, false
	}
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return skillMeta{}, false
	}
	fm := rest[:idx]
	body := strings.TrimSpace(rest[idx+4:])

	var meta skillMeta
	meta.Body = body
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		v = strings.Trim(v, "\"")
		switch k {
		case "name":
			meta.Name = v
		case "description":
			meta.Description = v
		case "argument-hint":
			meta.ArgumentHint = v
		}
	}
	if meta.Name == "" {
		return skillMeta{}, false
	}
	return meta, true
}

// parseArgumentHint converts an argument-hint string like "[old-name] [new-name]"
// into MCP PromptArgument definitions. Bracket groups are split first so
// "[optional: start_line-end_line]" is treated as one argument.
func parseArgumentHint(hint string) []*mcp.PromptArgument {
	if hint == "" {
		return nil
	}
	var args []*mcp.PromptArgument
	// Split on bracket groups rather than whitespace so multi-word groups
	// like "[optional: range]" stay together.
	rest := hint
	for rest != "" {
		open := strings.IndexByte(rest, '[')
		if open < 0 {
			break
		}
		close := strings.IndexByte(rest[open:], ']')
		if close < 0 {
			break
		}
		group := rest[open+1 : open+close]
		rest = rest[open+close+1:]

		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}

		// Skip pure connectors like "symbol-name | file-path" alternatives
		// by splitting on | and creating one arg per alternative.
		for _, alt := range strings.Split(group, "|") {
			alt = strings.TrimSpace(alt)
			if alt == "" {
				continue
			}
			required := true
			if strings.HasPrefix(alt, "optional:") || strings.HasPrefix(alt, "optional ") {
				required = false
				alt = strings.TrimPrefix(alt, "optional:")
				alt = strings.TrimPrefix(alt, "optional ")
				alt = strings.TrimSpace(alt)
			}
			if alt == "" {
				continue
			}
			// Collapse whitespace to hyphens for the name
			name := strings.Join(strings.Fields(alt), "-")
			args = append(args, &mcp.PromptArgument{
				Name:     name,
				Required: required,
			})
		}
	}
	return args
}

// loadSkills parses all embedded SKILL.md files and returns their metadata.
func loadSkills() []skillMeta {
	var result []skillMeta
	_ = fs.WalkDir(skills.Files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Base(path) != "SKILL.md" {
			return nil
		}
		data, readErr := skills.Files.ReadFile(path)
		if readErr != nil {
			return nil
		}
		meta, ok := parseSkillMD(string(data))
		if !ok {
			return nil
		}
		result = append(result, meta)
		return nil
	})
	return result
}

// registerPrompts loads all embedded SKILL.md files and registers each as
// an MCP prompt on the server.
func registerPrompts(server *mcp.Server) {
	for _, meta := range loadSkills() {
		prompt := &mcp.Prompt{
			Name:        meta.Name,
			Description: meta.Description,
			Arguments:   parseArgumentHint(meta.ArgumentHint),
		}

		body := meta.Body
		desc := meta.Description
		server.AddPrompt(prompt, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			instruction := body
			if len(req.Params.Arguments) > 0 {
				var parts []string
				for k, v := range req.Params.Arguments {
					parts = append(parts, fmt.Sprintf("%s: %s", k, v))
				}
				instruction = fmt.Sprintf("Arguments: %s\n\n%s", strings.Join(parts, ", "), body)
			}
			return &mcp.GetPromptResult{
				Description: desc,
				Messages: []*mcp.PromptMessage{
					{
						Role:    "user",
						Content: &mcp.TextContent{Text: instruction},
					},
				},
			}, nil
		})
	}
}

// registerPromptsWithCallback is a test helper that runs the same parsing
// as registerPrompts but calls cb instead of registering on a server.
func registerPromptsWithCallback(cb func(name, description string)) {
	for _, meta := range loadSkills() {
		cb(meta.Name, meta.Description)
	}
}
