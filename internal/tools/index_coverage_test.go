package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// --- probeWorkspaceCoverage (issue #42) ---

func openedSet(paths ...string) map[string]bool {
	out := make(map[string]bool, len(paths))
	for _, p := range paths {
		out[p] = true
	}
	return out
}

func TestProbeWorkspaceCoverage_EmptyRoot(t *testing.T) {
	root := t.TempDir()
	if got := probeWorkspaceCoverage(context.Background(), root, nil); got != coverageComplete {
		t.Fatalf("empty workspace should be complete, got %v", got)
	}
}

func TestProbeWorkspaceCoverage_EmptyRootPath(t *testing.T) {
	// No root (client without a workspace) — nothing to classify.
	if got := probeWorkspaceCoverage(context.Background(), "", nil); got != coverageComplete {
		t.Fatalf("empty root path should be complete, got %v", got)
	}
}

func TestProbeWorkspaceCoverage_AllOpened(t *testing.T) {
	root := t.TempDir()
	var opened []string
	for _, name := range []string{"a.go", "b.go"} {
		p := filepath.Join(root, name)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		opened = append(opened, p)
	}
	if got := probeWorkspaceCoverage(context.Background(), root, openedSet(opened...)); got != coverageComplete {
		t.Fatalf("all files opened should be complete, got %v", got)
	}
}

func TestProbeWorkspaceCoverage_MoreFilesThanOpened(t *testing.T) {
	root := t.TempDir()
	var opened []string
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		p := filepath.Join(root, name)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if name != "c.go" {
			opened = append(opened, p)
		}
	}
	if got := probeWorkspaceCoverage(context.Background(), root, openedSet(opened...)); got != coverageUnopened {
		t.Fatalf("3 files with 2 opened should be unopened, got %v", got)
	}
}

func TestProbeWorkspaceCoverage_OpenedOutsideEligibleSet(t *testing.T) {
	// Opens outside the eligible workspace set (e.g. node_modules) must not
	// mask unopened source files — the comparison is by path set, not by
	// open-document count. (CodeRabbit #59 thread 4)
	root := t.TempDir()
	src := filepath.Join(root, "a.go")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	vendored := filepath.Join(root, "node_modules", "x.js")
	if err := os.MkdirAll(filepath.Dir(vendored), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vendored, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The only opened document lives in a skipped directory.
	if got := probeWorkspaceCoverage(context.Background(), root, openedSet(vendored)); got != coverageUnopened {
		t.Fatalf("unopened source file masked by a node_modules open: got %v, want coverageUnopened", got)
	}
}

func TestProbeWorkspaceCoverage_SkipsDotAndSkipDirs(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "a.go")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	hidden := filepath.Join(root, ".hidden")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hidden, "b.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	vendored := filepath.Join(root, "node_modules")
	if err := os.MkdirAll(vendored, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vendored, "c.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Only the one real file counts, and it is opened → complete.
	if got := probeWorkspaceCoverage(context.Background(), root, openedSet(p)); got != coverageComplete {
		t.Fatalf("hidden and skipped dirs must not count as unopened files, got %v", got)
	}
}

func TestProbeWorkspaceCoverage_Recursive(t *testing.T) {
	root := t.TempDir()
	rootFile := filepath.Join(root, "a.go")
	if err := os.WriteFile(rootFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "src")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	subFile := filepath.Join(sub, "b.go")
	if err := os.WriteFile(subFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Only the sub file opened → the root-level file is unopened.
	if got := probeWorkspaceCoverage(context.Background(), root, openedSet(subFile)); got != coverageUnopened {
		t.Fatalf("files in subdirectories must count toward coverage, got %v", got)
	}
}

func TestProbeWorkspaceCoverage_CapYieldsUnknown(t *testing.T) {
	// Budget exhaustion must report unknown coverage — which still surfaces
	// the caveat — even when the opened set would outnumber the probe cap.
	// (CodeRabbit #59 thread 6)
	root := t.TempDir()
	opened := map[string]bool{}
	for i := 0; i < maxProbeEntries+50; i++ {
		p := filepath.Join(root, "f"+itoa(i)+".go")
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		opened[p] = true
	}
	if got := probeWorkspaceCoverage(context.Background(), root, opened); got != coverageUnknown {
		t.Fatalf("capped walk must report unknown coverage, got %v", got)
	}
}

func TestProbeWorkspaceCoverage_UnopenedBeforeCap(t *testing.T) {
	// The probe short-circuits on the first unopened file instead of walking
	// the whole budget: one unopened file among many is enough.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := probeWorkspaceCoverage(context.Background(), root, nil); got != coverageUnopened {
		t.Fatalf("one unopened file must yield unopened coverage, got %v", got)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
