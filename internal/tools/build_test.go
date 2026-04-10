package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHandleRunBuild_MissingWorkspaceDir verifies that an empty args map
// returns an error result requiring workspace_dir.
func TestHandleRunBuild_MissingWorkspaceDir(t *testing.T) {
	r, err := HandleRunBuild(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true for missing workspace_dir")
	}
}

// TestHandleRunTests_MissingWorkspaceDir verifies that an empty args map
// returns an error result requiring workspace_dir.
func TestHandleRunTests_MissingWorkspaceDir(t *testing.T) {
	r, err := HandleRunTests(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true for missing workspace_dir")
	}
}

// TestHandleGetTestsForFile_MissingFilePath verifies that an empty args map
// returns an error result requiring file_path.
func TestHandleGetTestsForFile_MissingFilePath(t *testing.T) {
	r, err := HandleGetTestsForFile(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true for missing file_path")
	}
}

// TestHandleRunBuild_UnsupportedLanguage verifies that specifying an unknown
// language returns an error result.
func TestHandleRunBuild_UnsupportedLanguage(t *testing.T) {
	dir := t.TempDir()
	r, err := HandleRunBuild(context.Background(), map[string]interface{}{
		"workspace_dir": dir,
		"language":      "cobol",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true for unsupported language")
	}
	if !strings.Contains(r.Content[0].Text, "cobol") {
		t.Errorf("expected error message to contain 'cobol', got: %s", r.Content[0].Text)
	}
}

// TestHandleRunTests_UnsupportedLanguage verifies that specifying an unknown
// language returns an error result.
func TestHandleRunTests_UnsupportedLanguage(t *testing.T) {
	dir := t.TempDir()
	r, err := HandleRunTests(context.Background(), map[string]interface{}{
		"workspace_dir": dir,
		"language":      "cobol",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected IsError=true for unsupported language")
	}
	if !strings.Contains(r.Content[0].Text, "cobol") {
		t.Errorf("expected error message to contain 'cobol', got: %s", r.Content[0].Text)
	}
}

// TestFindTestFiles_Go verifies that FindTestFiles finds *_test.go in the same
// directory as the source file.
func TestFindTestFiles_Go(t *testing.T) {
	dir := t.TempDir()

	// Create main.go and main_test.go.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testFilePath := filepath.Join(dir, "main_test.go")
	if err := os.WriteFile(testFilePath, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := FindTestFiles(context.Background(), dir, "go", filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, tf := range result.TestFiles {
		if tf == testFilePath {
			found = true
		}
	}
	if !found {
		t.Errorf("expected main_test.go in TestFiles, got: %v", result.TestFiles)
	}
}

// TestFindTestFiles_Go_NoTests verifies that FindTestFiles returns an empty
// list when no *_test.go files exist.
func TestFindTestFiles_Go_NoTests(t *testing.T) {
	dir := t.TempDir()

	// Create only main.go.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := FindTestFiles(context.Background(), dir, "go", filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.TestFiles) != 0 {
		t.Errorf("expected empty TestFiles, got: %v", result.TestFiles)
	}
}

// TestLocationNormalization verifies that a TestFailure with File and Line set
// gets a correctly normalized LSP Location after normalizeTestFailureLocation.
func TestLocationNormalization(t *testing.T) {
	root := t.TempDir()

	tf := &TestFailure{
		TestName: "TestSomething",
		File:     "pkg_test.go",
		Line:     42,
		Message:  "assertion failed",
	}

	normalizeTestFailureLocation(root, tf)

	if tf.Location == nil {
		t.Fatal("expected Location to be set")
	}
	if !strings.HasPrefix(tf.Location.URI, "file://") {
		t.Errorf("expected URI to start with 'file://', got: %s", tf.Location.URI)
	}
	if tf.Location.Range.Start.Line != 41 {
		t.Errorf("expected Start.Line=41 (0-indexed from line 42), got: %d", tf.Location.Range.Start.Line)
	}
}

// TestBuildResultShape verifies that RunBuild returns a BuildResult with the
// expected shape when run against a valid Go module directory.
func TestBuildResultShape(t *testing.T) {
	dir := t.TempDir()

	// Create a minimal Go module.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := RunBuild(context.Background(), dir, "go", "./...")
	if err != nil {
		t.Fatalf("unexpected error from RunBuild: %v", err)
	}

	if !result.Success {
		t.Errorf("expected Success=true for valid Go module, got false. Raw: %s", result.Raw)
	}

	// Verify Raw is a string (may be empty).
	_ = result.Raw // always a string in Go

	// Verify the shape can be marshaled and unmarshaled.
	data, mErr := json.Marshal(result)
	if mErr != nil {
		t.Fatalf("marshal error: %v", mErr)
	}
	var roundTrip BuildResult
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if roundTrip.Success != result.Success {
		t.Errorf("Success mismatch after round-trip")
	}
}

// TestRunners_NewLanguages verifies that the four new language entries
// are present in the runners dispatch table and have non-empty commands.
func TestRunners_NewLanguages(t *testing.T) {
	languages := []string{"csharp", "swift", "zig", "kotlin"}
	for _, lang := range languages {
		t.Run(lang, func(t *testing.T) {
			runner, ok := runners[lang]
			if !ok {
				t.Fatalf("language %q not found in runners map", lang)
			}
			if runner.buildCmd == "" {
				t.Errorf("language %q has empty buildCmd", lang)
			}
			if runner.testCmd == "" {
				t.Errorf("language %q has empty testCmd", lang)
			}
		})
	}
}

// TestParseBuildErrors_CSharp verifies MSBuild-style error parsing.
func TestParseBuildErrors_CSharp(t *testing.T) {
	input := []byte("Program.cs(10,5): error CS0246: The type or namespace name 'Foo' could not be found\n")
	errors := parseBuildErrors("csharp", input)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}
	if errors[0].File != "Program.cs" {
		t.Errorf("expected File=Program.cs, got %q", errors[0].File)
	}
	if errors[0].Line != 10 {
		t.Errorf("expected Line=10, got %d", errors[0].Line)
	}
	if errors[0].Column != 5 {
		t.Errorf("expected Column=5, got %d", errors[0].Column)
	}
}

// TestParseBuildErrors_Swift verifies Swift compiler error parsing.
func TestParseBuildErrors_Swift(t *testing.T) {
	input := []byte("Sources/main.swift:3:10: error: use of unresolved identifier 'Foo'\n")
	errors := parseBuildErrors("swift", input)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}
	if errors[0].File != "Sources/main.swift" {
		t.Errorf("expected File=Sources/main.swift, got %q", errors[0].File)
	}
	if errors[0].Line != 3 {
		t.Errorf("expected Line=3, got %d", errors[0].Line)
	}
	if errors[0].Column != 10 {
		t.Errorf("expected Column=10, got %d", errors[0].Column)
	}
}

// TestParseBuildErrors_Zig verifies Zig compiler error parsing.
func TestParseBuildErrors_Zig(t *testing.T) {
	input := []byte("src/main.zig:7:3: error: expected ';' after statement\n")
	errors := parseBuildErrors("zig", input)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}
	if errors[0].File != "src/main.zig" {
		t.Errorf("expected File=src/main.zig, got %q", errors[0].File)
	}
	if errors[0].Line != 7 {
		t.Errorf("expected Line=7, got %d", errors[0].Line)
	}
	if errors[0].Column != 3 {
		t.Errorf("expected Column=3, got %d", errors[0].Column)
	}
}

// TestParseBuildErrors_Kotlin verifies Gradle Kotlin error parsing.
func TestParseBuildErrors_Kotlin(t *testing.T) {
	input := []byte("e: Greeter.kt: (12, 8): error: unresolved reference: Person\n")
	errors := parseBuildErrors("kotlin", input)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}
	if errors[0].File != "Greeter.kt" {
		t.Errorf("expected File=Greeter.kt, got %q", errors[0].File)
	}
	if errors[0].Line != 12 {
		t.Errorf("expected Line=12, got %d", errors[0].Line)
	}
	if errors[0].Column != 8 {
		t.Errorf("expected Column=8, got %d", errors[0].Column)
	}
}

// TestParseBuildErrors_NewLanguages_NoErrors verifies that clean output yields empty slice
// for each new language.
func TestParseBuildErrors_NewLanguages_NoErrors(t *testing.T) {
	languages := []string{"csharp", "swift", "zig", "kotlin"}
	for _, lang := range languages {
		t.Run(lang, func(t *testing.T) {
			result := parseBuildErrors(lang, []byte("Build succeeded.\n"))
			if len(result) != 0 {
				t.Errorf("expected 0 errors for clean output, got %d", len(result))
			}
		})
	}
}

// TestParseBuildErrors_TypeScript verifies TypeScript compiler error parsing.
func TestParseBuildErrors_TypeScript(t *testing.T) {
	input := []byte("src/index.ts(5,3): error TS2322: Type 'string' is not assignable to type 'number'\n")
	errors := parseBuildErrors("typescript", input)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}
	if errors[0].File != "src/index.ts" {
		t.Errorf("expected File=src/index.ts, got %q", errors[0].File)
	}
	if errors[0].Line != 5 {
		t.Errorf("expected Line=5, got %d", errors[0].Line)
	}
	if errors[0].Column != 3 {
		t.Errorf("expected Column=3, got %d", errors[0].Column)
	}
}

// TestParseBuildErrors_Rust verifies Rust compiler error parsing.
func TestParseBuildErrors_Rust(t *testing.T) {
	input := []byte("error[E0308]: mismatched types\n --> src/main.rs:10:5\n")
	errors := parseBuildErrors("rust", input)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}
	if errors[0].File != "src/main.rs" {
		t.Errorf("expected File=src/main.rs, got %q", errors[0].File)
	}
	if errors[0].Line != 10 {
		t.Errorf("expected Line=10, got %d", errors[0].Line)
	}
	if errors[0].Column != 5 {
		t.Errorf("expected Column=5, got %d", errors[0].Column)
	}
}

// TestParseBuildErrors_Python verifies mypy/pyflakes-style error parsing.
func TestParseBuildErrors_Python(t *testing.T) {
	input := []byte("app/models.py:23: error: Argument 1 to \"open\" has incompatible type \"int\"\n")
	errors := parseBuildErrors("python", input)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}
	if errors[0].File != "app/models.py" {
		t.Errorf("expected File=app/models.py, got %q", errors[0].File)
	}
	if errors[0].Line != 23 {
		t.Errorf("expected Line=23, got %d", errors[0].Line)
	}
}

// TestParseBuildErrors_NoErrors_TypeScript_Rust_Python verifies clean output.
func TestParseBuildErrors_NoErrors_TypeScript_Rust_Python(t *testing.T) {
	languages := []string{"typescript", "rust", "python"}
	for _, lang := range languages {
		t.Run(lang, func(t *testing.T) {
			result := parseBuildErrors(lang, []byte("Build succeeded.\n"))
			if len(result) != 0 {
				t.Errorf("expected 0 errors for clean output, got %d", len(result))
			}
		})
	}
}

// TestFindTestFiles_CSharp verifies that FindTestFiles finds *Test*.cs files.
func TestFindTestFiles_CSharp(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Greeter.cs"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(dir, "GreeterTests.cs")
	if err := os.WriteFile(testFile, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := FindTestFiles(context.Background(), dir, "csharp", filepath.Join(dir, "Greeter.cs"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, tf := range result.TestFiles {
		if tf == testFile {
			found = true
		}
	}
	if !found {
		t.Errorf("expected GreeterTests.cs in TestFiles, got: %v", result.TestFiles)
	}
}

// TestFindTestFiles_Swift verifies that FindTestFiles finds *Tests.swift files.
func TestFindTestFiles_Swift(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Greeter.swift"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(dir, "GreeterTests.swift")
	if err := os.WriteFile(testFile, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := FindTestFiles(context.Background(), dir, "swift", filepath.Join(dir, "Greeter.swift"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, tf := range result.TestFiles {
		if tf == testFile {
			found = true
		}
	}
	if !found {
		t.Errorf("expected GreeterTests.swift in TestFiles, got: %v", result.TestFiles)
	}
}

// TestFindTestFiles_Zig verifies that FindTestFiles returns the source file
// itself (inline tests, same pattern as Rust).
func TestFindTestFiles_Zig(t *testing.T) {
	dir := t.TempDir()
	srcFile := filepath.Join(dir, "greeter.zig")
	if err := os.WriteFile(srcFile, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := FindTestFiles(context.Background(), dir, "zig", srcFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.TestFiles) != 1 {
		t.Fatalf("expected 1 test file (source itself), got %d: %v", len(result.TestFiles), result.TestFiles)
	}
	if result.TestFiles[0] != srcFile {
		t.Errorf("expected source file %q, got %q", srcFile, result.TestFiles[0])
	}
}

// TestFindTestFiles_Kotlin verifies that FindTestFiles finds *Test.kt files.
func TestFindTestFiles_Kotlin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Greeter.kt"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(dir, "GreeterTest.kt")
	if err := os.WriteFile(testFile, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := FindTestFiles(context.Background(), dir, "kotlin", filepath.Join(dir, "Greeter.kt"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, tf := range result.TestFiles {
		if tf == testFile {
			found = true
		}
	}
	if !found {
		t.Errorf("expected GreeterTest.kt in TestFiles, got: %v", result.TestFiles)
	}
}
