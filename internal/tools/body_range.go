package tools

import (
	"strings"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

// siblingDeclarationPrefixes lists line starts that introduce a new definition
// at the same indentation as the symbol whose body is being replaced.
var siblingDeclarationPrefixes = []string{
	"def ", "class ", "async def ", "func ", "function ", "fn ",
	"pub fn ", "const ", "let ", "var ", "type ", "struct ",
	"enum ", "impl ", "interface ", "export ", "@",
}

func isSiblingDeclaration(trimmed string) bool {
	for _, prefix := range siblingDeclarationPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func leadingWhitespace(line string) int {
	n := 0
	for _, r := range line {
		if r == ' ' || r == '\t' {
			n++
			continue
		}
		break
	}
	return n
}

// clampBodyEndToNextSibling shortens a body end position so that a following
// definition is never swallowed. A malformed symbol range can extend past the
// target function when an earlier edit left the file unparsable; the text scan
// is independent of the language server's view, which is unreliable in that
// state.
//
// declLine is the line of the symbol declaration, bodyStartLine the first line
// of its body. A non-blank line at or before the declaration's indentation ends
// the body: at strictly less indentation it is a dedent out of the scope, at
// equal indentation it must look like a new declaration so a closing brace or
// continuation line does not truncate the body.
func clampBodyEndToNextSibling(content string, declLine, bodyStartLine int, bodyEnd types.Position) types.Position {
	lines := strings.Split(content, "\n")
	if declLine < 0 || declLine >= len(lines) {
		return bodyEnd
	}
	declIndent := leadingWhitespace(lines[declLine])
	last := bodyEnd.Line
	if last >= len(lines) {
		last = len(lines) - 1
	}
	for i := bodyStartLine; i <= last; i++ {
		if i < 0 || i >= len(lines) {
			continue
		}
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		indent := leadingWhitespace(lines[i])
		if indent > declIndent {
			continue
		}
		if indent == declIndent && !isSiblingDeclaration(trimmed) {
			continue
		}
		return types.Position{Line: i, Character: 0}
	}
	return bodyEnd
}
