package tools

import (
	"testing"

	"github.com/blackwell-systems/agent-lsp/internal/types"
)

func TestClampBodyEndToNextSiblingBrokenPython(t *testing.T) {
	content := `def sigma_pool(frame):
    """按域聚合。
    使用 prefix_columns 的结果。
    cols = prefix_columns(frame)
    return cols

def cluster_members(frame):
    """返回每个簇的成员。"""
    return frame
`
	got := clampBodyEndToNextSibling(content, 0, 1, types.Position{Line: 8, Character: 16})
	if want := (types.Position{Line: 6, Character: 0}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestClampBodyEndToNextSiblingWellFormed(t *testing.T) {
	content := `def sigma_pool(frame):
    """按域聚合。"""
    cols = prefix_columns(frame)
    return cols


def cluster_members(frame):
    return frame
`
	end := types.Position{Line: 3, Character: 19}
	got := clampBodyEndToNextSibling(content, 0, 1, end)
	if got != end {
		t.Fatalf("got %+v, want unchanged %+v", got, end)
	}
}

func TestClampBodyEndToNextSiblingNestedChild(t *testing.T) {
	content := `def outer():
    def inner():
        pass
    return inner
`
	end := types.Position{Line: 3, Character: 16}
	got := clampBodyEndToNextSibling(content, 0, 1, end)
	if got != end {
		t.Fatalf("nested child was mistaken for a sibling: got %+v, want %+v", got, end)
	}
}

func TestClampBodyEndToNextSiblingClosesBraceNotSibling(t *testing.T) {
	content := "func f() {\n\tif true {\n\t\treturn\n\t}\n}\n"
	end := types.Position{Line: 4, Character: 1}
	got := clampBodyEndToNextSibling(content, 0, 1, end)
	if got != end {
		t.Fatalf("closing brace was mistaken for a sibling: got %+v, want %+v", got, end)
	}
}

func TestClampBodyEndToNextSiblingDedent(t *testing.T) {
	content := `class C:
    def m(self):
        return 1

x = 2
`
	got := clampBodyEndToNextSibling(content, 1, 2, types.Position{Line: 4, Character: 5})
	if want := (types.Position{Line: 4, Character: 0}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestClampBodyEndToNextSiblingIgnoresDeclarationInString(t *testing.T) {
	content := `def render():
    template = """
def helper():
    pass
"""
    return template

def real_next():
    return 1
`
	end := types.Position{Line: 5, Character: 19}
	got := clampBodyEndToNextSibling(content, 0, 1, end)
	if got != end {
		t.Fatalf("declaration inside a closed multiline string truncated the body: got %+v, want %+v", got, end)
	}
}

func TestClampBodyEndToNextSiblingIgnoresDeclarationInBlockComment(t *testing.T) {
	content := "func f() {\n    x := /*\nfunc g() {\n*/\n    1\n}\n"
	end := types.Position{Line: 5, Character: 1}
	got := clampBodyEndToNextSibling(content, 0, 1, end)
	if got != end {
		t.Fatalf("declaration inside a block comment truncated the body: got %+v, want %+v", got, end)
	}
}
