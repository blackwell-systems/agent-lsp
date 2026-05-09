package config

import (
	"path/filepath"
	"testing"
)

func TestInferWorkspaceRoot_Swift(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "Package.swift"))
	subdir := filepath.Join(root, "Sources")
	mkdirAll(t, subdir)
	filePath := filepath.Join(subdir, "main.swift")
	touch(t, filePath)

	gotRoot, gotLang, err := inferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root: got %q, want %q", gotRoot, root)
	}
	if gotLang != "swift" {
		t.Errorf("languageID: got %q, want %q", gotLang, "swift")
	}
}

func TestInferWorkspaceRoot_Zig(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "build.zig"))
	filePath := filepath.Join(root, "main.zig")
	touch(t, filePath)

	gotRoot, gotLang, err := inferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root: got %q, want %q", gotRoot, root)
	}
	if gotLang != "zig" {
		t.Errorf("languageID: got %q, want %q", gotLang, "zig")
	}
}

func TestInferWorkspaceRoot_KotlinGradleKts(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "build.gradle.kts"))
	subdir := filepath.Join(root, "src", "main", "kotlin")
	mkdirAll(t, subdir)
	filePath := filepath.Join(subdir, "Main.kt")
	touch(t, filePath)

	gotRoot, gotLang, err := inferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root: got %q, want %q", gotRoot, root)
	}
	if gotLang != "kotlin" {
		t.Errorf("languageID: got %q, want %q", gotLang, "kotlin")
	}
}

func TestInferWorkspaceRoot_JavaGradle(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "build.gradle"))
	filePath := filepath.Join(root, "App.java")
	touch(t, filePath)

	gotRoot, gotLang, err := inferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root: got %q, want %q", gotRoot, root)
	}
	if gotLang != "java" {
		t.Errorf("languageID: got %q, want %q", gotLang, "java")
	}
}

func TestInferWorkspaceRoot_JavaMaven(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "pom.xml"))
	filePath := filepath.Join(root, "App.java")
	touch(t, filePath)

	gotRoot, gotLang, err := inferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root: got %q, want %q", gotRoot, root)
	}
	if gotLang != "java" {
		t.Errorf("languageID: got %q, want %q", gotLang, "java")
	}
}

func TestInferWorkspaceRoot_Scala(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "build.sbt"))
	filePath := filepath.Join(root, "Main.scala")
	touch(t, filePath)

	gotRoot, gotLang, err := inferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root: got %q, want %q", gotRoot, root)
	}
	if gotLang != "scala" {
		t.Errorf("languageID: got %q, want %q", gotLang, "scala")
	}
}

func TestInferWorkspaceRoot_Terraform(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, ".terraform.lock.hcl"))
	filePath := filepath.Join(root, "main.tf")
	touch(t, filePath)

	gotRoot, gotLang, err := inferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root: got %q, want %q", gotRoot, root)
	}
	if gotLang != "terraform" {
		t.Errorf("languageID: got %q, want %q", gotLang, "terraform")
	}
}

func TestInferWorkspaceRoot_Ruby(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "Gemfile"))
	filePath := filepath.Join(root, "app.rb")
	touch(t, filePath)

	gotRoot, gotLang, err := inferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root: got %q, want %q", gotRoot, root)
	}
	if gotLang != "ruby" {
		t.Errorf("languageID: got %q, want %q", gotLang, "ruby")
	}
}

func TestInferWorkspaceRoot_PHP(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "composer.json"))
	filePath := filepath.Join(root, "index.php")
	touch(t, filePath)

	gotRoot, gotLang, err := inferWorkspaceRoot(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root: got %q, want %q", gotRoot, root)
	}
	if gotLang != "php" {
		t.Errorf("languageID: got %q, want %q", gotLang, "php")
	}
}

func TestInferWorkspaceRoot_DirectoryInput(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "go.mod"))

	gotRoot, gotLang, err := inferWorkspaceRoot(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root: got %q, want %q", gotRoot, root)
	}
	if gotLang != "go" {
		t.Errorf("languageID: got %q, want %q", gotLang, "go")
	}
}
