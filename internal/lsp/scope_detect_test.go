package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// --- Python package detection ---

func TestDetectPackageScope_Python_WithInitPy(t *testing.T) {
	root := t.TempDir()

	// Create a Python package hierarchy: myapp/core/__init__.py
	mkDir(t, root, "myapp")
	mkFile(t, root, "myapp/__init__.py", "")
	mkDir(t, root, "myapp/core")
	mkFile(t, root, "myapp/core/__init__.py", "")
	mkFile(t, root, "myapp/core/handler.py", "import os\nfrom myapp.utils import helper\n")

	// Also create the utils package that handler.py imports.
	mkDir(t, root, "myapp/utils")
	mkFile(t, root, "myapp/utils/__init__.py", "")

	filePath := filepath.Join(root, "myapp/core/handler.py")
	scope, err := DetectPackageScope(filePath, root, "python")
	if err != nil {
		t.Fatalf("DetectPackageScope: %v", err)
	}

	// Should include the package directory ("myapp") since both myapp/ and
	// myapp/core/ have __init__.py, the walk-up should find "myapp" as the
	// top-level package.
	if len(scope) == 0 {
		t.Fatal("expected non-empty scope")
	}

	// The package root should be "myapp" (the top-level __init__.py package).
	if scope[0] != "myapp" {
		t.Errorf("scope[0] = %q, want %q", scope[0], "myapp")
	}
}

func TestDetectPackageScope_Python_NoInitPy(t *testing.T) {
	root := t.TempDir()

	// A plain directory with no __init__.py: use the file's directory.
	mkDir(t, root, "scripts")
	mkFile(t, root, "scripts/run.py", "import sys\n")

	filePath := filepath.Join(root, "scripts/run.py")
	scope, err := DetectPackageScope(filePath, root, "python")
	if err != nil {
		t.Fatalf("DetectPackageScope: %v", err)
	}

	if len(scope) == 0 {
		t.Fatal("expected non-empty scope")
	}
	if scope[0] != "scripts" {
		t.Errorf("scope[0] = %q, want %q", scope[0], "scripts")
	}
}

func TestDetectPackageScope_Python_ImportResolution(t *testing.T) {
	root := t.TempDir()

	// Package "api" imports from "models" and "external_lib".
	mkDir(t, root, "api")
	mkFile(t, root, "api/__init__.py", "")
	mkFile(t, root, "api/views.py", "from models import User\nimport utils.helpers\n")

	// "models" exists as a local package.
	mkDir(t, root, "models")
	mkFile(t, root, "models/__init__.py", "")

	// "utils" exists as a local package.
	mkDir(t, root, "utils")
	mkFile(t, root, "utils/__init__.py", "")

	filePath := filepath.Join(root, "api/views.py")
	scope, err := DetectPackageScope(filePath, root, "python")
	if err != nil {
		t.Fatalf("DetectPackageScope: %v", err)
	}

	// Should contain: "api" (the package), "models", "utils".
	sort.Strings(scope)
	want := []string{"api", "models", "utils"}
	sort.Strings(want)

	if len(scope) != len(want) {
		t.Fatalf("scope = %v, want %v", scope, want)
	}
	for i := range want {
		if scope[i] != want[i] {
			t.Errorf("scope[%d] = %q, want %q", i, scope[i], want[i])
		}
	}
}

// --- Python import parsing ---

func TestParsePythonImportsFile(t *testing.T) {
	root := t.TempDir()
	content := `import os
import sys
from pathlib import Path
from mypackage.submod import thing
from . import relative
import json
from ..parent import foo
`
	path := filepath.Join(root, "test.py")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	imports := parsePythonImportsFile(path)

	// Should include: os, sys, pathlib, mypackage.submod, json
	// Should exclude: . (relative), ..parent (relative)
	wantSet := map[string]bool{
		"os":                true,
		"sys":               true,
		"pathlib":           true,
		"mypackage.submod":  true,
		"json":              true,
	}

	for _, imp := range imports {
		if !wantSet[imp] {
			t.Errorf("unexpected import: %q", imp)
		}
		delete(wantSet, imp)
	}
	for missing := range wantSet {
		t.Errorf("missing import: %q", missing)
	}
}

// --- TypeScript package detection ---

func TestDetectPackageScope_TypeScript_WithPackageJSON(t *testing.T) {
	root := t.TempDir()

	// Create a TS package: packages/core/package.json
	mkDir(t, root, "packages/core")
	mkFile(t, root, "packages/core/package.json", `{"name": "core"}`)
	mkDir(t, root, "packages/core/src")
	mkFile(t, root, "packages/core/src/index.ts", `import { helper } from '../utils/helper'`)

	// Create the utils sibling.
	mkDir(t, root, "packages/core/utils")
	mkFile(t, root, "packages/core/utils/helper.ts", "export function helper() {}")

	filePath := filepath.Join(root, "packages/core/src/index.ts")
	scope, err := DetectPackageScope(filePath, root, "typescript")
	if err != nil {
		t.Fatalf("DetectPackageScope: %v", err)
	}

	if len(scope) == 0 {
		t.Fatal("expected non-empty scope")
	}
	// Should find "packages/core" as the package root (contains package.json).
	if scope[0] != filepath.Join("packages", "core") {
		t.Errorf("scope[0] = %q, want %q", scope[0], filepath.Join("packages", "core"))
	}
}

func TestDetectPackageScope_TypeScript_NoPackageJSON(t *testing.T) {
	root := t.TempDir()

	// No package.json anywhere: fall back to file's directory.
	mkDir(t, root, "src")
	mkFile(t, root, "src/app.ts", "const x = 1")

	filePath := filepath.Join(root, "src/app.ts")
	scope, err := DetectPackageScope(filePath, root, "typescript")
	if err != nil {
		t.Fatalf("DetectPackageScope: %v", err)
	}

	if len(scope) == 0 {
		t.Fatal("expected non-empty scope")
	}
	if scope[0] != "src" {
		t.Errorf("scope[0] = %q, want %q", scope[0], "src")
	}
}

// --- TypeScript import parsing ---

func TestParseTSImportsFile(t *testing.T) {
	root := t.TempDir()
	content := `import { foo } from './utils/foo'
import bar from '../bar'
const x = require('./helpers/baz')
import { something } from 'external-package'
import type { Type } from './types'
`
	path := filepath.Join(root, "test.ts")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	imports := parseTSImportsFile(path)

	// Should include relative imports only: ./utils/foo, ../bar, ./helpers/baz, ./types
	// Should NOT include: external-package
	wantSet := map[string]bool{
		"./utils/foo":   true,
		"../bar":        true,
		"./helpers/baz": true,
		"./types":       true,
	}

	for _, imp := range imports {
		if !wantSet[imp] {
			t.Errorf("unexpected import: %q", imp)
		}
		delete(wantSet, imp)
	}
	for missing := range wantSet {
		t.Errorf("missing import: %q", missing)
	}
}

// --- Go/Rust returns nil ---

func TestDetectPackageScope_Go_ReturnsNil(t *testing.T) {
	scope, err := DetectPackageScope("/some/file.go", "/some", "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scope != nil {
		t.Errorf("expected nil scope for Go, got %v", scope)
	}
}

func TestDetectPackageScope_Rust_ReturnsNil(t *testing.T) {
	scope, err := DetectPackageScope("/some/file.rs", "/some", "rust")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scope != nil {
		t.Errorf("expected nil scope for Rust, got %v", scope)
	}
}

// --- countSourceFiles ---

func TestCountSourceFiles_BelowThreshold(t *testing.T) {
	root := t.TempDir()
	mkDir(t, root, "src")
	// Create 10 .py files, well below threshold.
	for i := 0; i < 10; i++ {
		mkFile(t, root, filepath.Join("src", "file"+scopeItoa(i)+".py"), "")
	}
	if countSourceFiles(root, "python") {
		t.Error("expected false (below threshold), got true")
	}
}

func TestCountSourceFiles_AboveThreshold(t *testing.T) {
	root := t.TempDir()
	mkDir(t, root, "src")
	// Create 501 .py files, above the 500 threshold.
	for i := 0; i < 501; i++ {
		mkFile(t, root, filepath.Join("src", "file"+scopeItoa(i)+".py"), "")
	}
	if !countSourceFiles(root, "python") {
		t.Error("expected true (above threshold), got false")
	}
}

func TestCountSourceFiles_SkipsNodeModules(t *testing.T) {
	root := t.TempDir()
	// Put 600 files inside node_modules; they should be skipped.
	mkDir(t, root, "node_modules/dep/src")
	for i := 0; i < 600; i++ {
		mkFile(t, root, filepath.Join("node_modules/dep/src", "file"+scopeItoa(i)+".ts"), "")
	}
	// Only 5 real files.
	mkDir(t, root, "src")
	for i := 0; i < 5; i++ {
		mkFile(t, root, filepath.Join("src", "app"+scopeItoa(i)+".ts"), "")
	}
	if countSourceFiles(root, "typescript") {
		t.Error("expected false (node_modules should be skipped), got true")
	}
}

// --- Scope shift detection ---

func TestScopeShift_DifferentPackage(t *testing.T) {
	root := t.TempDir()

	// Two separate Python packages.
	mkDir(t, root, "pkg_a")
	mkFile(t, root, "pkg_a/__init__.py", "")
	mkFile(t, root, "pkg_a/mod.py", "")

	mkDir(t, root, "pkg_b")
	mkFile(t, root, "pkg_b/__init__.py", "")
	mkFile(t, root, "pkg_b/mod.py", "")

	scopeA, err := DetectPackageScope(filepath.Join(root, "pkg_a/mod.py"), root, "python")
	if err != nil {
		t.Fatalf("DetectPackageScope pkg_a: %v", err)
	}
	scopeB, err := DetectPackageScope(filepath.Join(root, "pkg_b/mod.py"), root, "python")
	if err != nil {
		t.Fatalf("DetectPackageScope pkg_b: %v", err)
	}

	if pathsEqual(scopeA, scopeB) {
		t.Errorf("expected different scopes for different packages, both = %v", scopeA)
	}
}

func TestPathsEqual(t *testing.T) {
	if !pathsEqual([]string{"a", "b"}, []string{"a", "b"}) {
		t.Error("expected equal")
	}
	if pathsEqual([]string{"a", "b"}, []string{"a", "c"}) {
		t.Error("expected not equal")
	}
	if pathsEqual([]string{"a"}, []string{"a", "b"}) {
		t.Error("expected not equal (different lengths)")
	}
	if !pathsEqual(nil, nil) {
		t.Error("expected nil == nil")
	}
}

// --- ShouldAutoScope ---

func TestShouldAutoScope_UnsupportedLanguage(t *testing.T) {
	root := t.TempDir()
	if ShouldAutoScope(root, "go") {
		t.Error("Go should not trigger auto-scope")
	}
	if ShouldAutoScope(root, "rust") {
		t.Error("Rust should not trigger auto-scope")
	}
}

// --- helpers ---

func mkDir(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, rel), 0755); err != nil {
		t.Fatal(err)
	}
}

func mkFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func scopeItoa(i int) string {
	return fmt.Sprintf("%d", i)
}
