package uri

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func canon(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// A create/rename to a not-yet-existing file THROUGH an in-repo symlinked parent
// directory pointing outside the root must be rejected (issue: symlinked-parent
// escape on the create path, which lexical-fallback validation missed).
func TestValidatePath_SymlinkedParentEscapeRejected(t *testing.T) {
	root := canon(t, t.TempDir())
	outside := canon(t, t.TempDir())
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	target := filepath.Join(root, "link", "newfile.txt") // does not exist yet
	if got, err := ValidatePath(target, root); err == nil {
		t.Fatalf("symlinked-parent escape NOT rejected: allowed %q (resolves outside root)", got)
	}
}

// A legitimate not-yet-existing file directly in the root must still be allowed
// (no false-reject of normal create).
func TestValidatePath_NewFileInRootAllowed(t *testing.T) {
	root := canon(t, t.TempDir())
	if _, err := ValidatePath(filepath.Join(root, "sub", "newfile.txt"), root); err != nil {
		t.Fatalf("legit new file in root rejected: %v", err)
	}
}

// Classic lexical traversal and absolute-outside must still be rejected.
func TestValidatePath_LexicalTraversalRejected(t *testing.T) {
	root := canon(t, t.TempDir())
	for _, p := range []string{filepath.Join(root, "..", "..", "etc", "passwd"), "/etc/passwd"} {
		if _, err := ValidatePath(p, root); err == nil {
			t.Errorf("traversal not rejected: %q", p)
		}
	}
}

// A symlinked leaf (existing) pointing outside must be rejected too.
func TestValidatePath_SymlinkedLeafEscapeRejected(t *testing.T) {
	root := canon(t, t.TempDir())
	outside := canon(t, t.TempDir())
	secret := filepath.Join(outside, "secret")
	_ = os.WriteFile(secret, []byte("x"), 0o600)
	link := filepath.Join(root, "leak")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if got, err := ValidatePath(link, root); err == nil && !strings.HasPrefix(got, root) {
		t.Fatalf("symlinked leaf escape not rejected: %q", got)
	}
}
