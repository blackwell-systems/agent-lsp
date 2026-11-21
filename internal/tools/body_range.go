package tools

import (
	"strings"
	"unicode"

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

func startsWithSpace(s string) bool {
	for _, r := range s {
		return unicode.IsSpace(r)
	}
	return false
}

// docstringOpenerLine reports whether a trimmed line starts a new docstring, as
// opposed to closing one. A closing delimiter stands alone or follows content;
// a new docstring has text immediately after its opening delimiter.
func docstringOpenerLine(trimmed, delimiter string) bool {
	return strings.HasPrefix(trimmed, delimiter) && len(trimmed) > len(delimiter) &&
		!startsWithSpace(trimmed[len(delimiter):])
}

// literalInteriorLines marks lines strictly inside a closed multiline string
// or block comment. A declaration-looking line there is literal text, not a
// sibling definition, so the body clamp must ignore it.
//
// An unterminated opening delimiter marks nothing. A file corrupted by issue
// #80 leaves a docstring open, and the declaration after it is real code that
// must still end the body range.
func literalInteriorLines(lines []string) []bool {
	interior := make([]bool, len(lines))
	for i := 0; i < len(lines); i++ {
		if idx := strings.Index(lines[i], "/*"); idx >= 0 {
			if strings.Contains(lines[i][idx+2:], "*/") {
				continue
			}
			for k := i + 1; k < len(lines); k++ {
				if strings.Contains(lines[k], "*/") {
					break
				}
				interior[k] = true
			}
			continue
		}
		for _, delimiter := range []string{`"""`, `'''`} {
			if strings.Count(lines[i], delimiter)%2 == 0 {
				continue
			}
			for k := i + 1; k < len(lines); k++ {
				if docstringOpenerLine(strings.TrimSpace(lines[k]), delimiter) {
					continue
				}
				if !strings.Contains(lines[k], delimiter) {
					continue
				}
				for m := i + 1; m < k; m++ {
					interior[m] = true
				}
				i = k
				break
			}
			break
		}
	}
	return interior
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
// continuation line does not truncate the body. Lines inside a closed string
// or comment are ignored.
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
	interior := literalInteriorLines(lines)
	for i := bodyStartLine; i <= last; i++ {
		if i < 0 || i >= len(lines) || interior[i] {
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
