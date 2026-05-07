package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateScopeConfig_Python(t *testing.T) {
	rootDir := t.TempDir()
	scopePaths := []string{"src/core", "src/utils"}

	sc, err := GenerateScopeConfig(rootDir, "python", scopePaths)
	if err != nil {
		t.Fatalf("GenerateScopeConfig: %v", err)
	}
	if sc == nil {
		t.Fatal("expected non-nil ScopeConfig for python")
	}

	configPath := filepath.Join(rootDir, "pyrightconfig.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read pyrightconfig.json: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to parse pyrightconfig.json: %v", err)
	}

	includes, ok := config["include"].([]interface{})
	if !ok {
		t.Fatal("pyrightconfig.json missing 'include' array")
	}
	if len(includes) != 2 {
		t.Errorf("expected 2 include paths, got %d", len(includes))
	}
	if includes[0] != "src/core" {
		t.Errorf("include[0] = %q, want %q", includes[0], "src/core")
	}
	if includes[1] != "src/utils" {
		t.Errorf("include[1] = %q, want %q", includes[1], "src/utils")
	}

	// Cleanup.
	RemoveScopeConfig(sc)
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Error("pyrightconfig.json should have been removed after RemoveScopeConfig")
	}
}

func TestGenerateScopeConfig_TypeScript(t *testing.T) {
	rootDir := t.TempDir()
	scopePaths := []string{"src/components"}

	sc, err := GenerateScopeConfig(rootDir, "typescript", scopePaths)
	if err != nil {
		t.Fatalf("GenerateScopeConfig: %v", err)
	}
	if sc == nil {
		t.Fatal("expected non-nil ScopeConfig for typescript")
	}

	configPath := filepath.Join(rootDir, "tsconfig.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read tsconfig.json: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to parse tsconfig.json: %v", err)
	}

	includes, ok := config["include"].([]interface{})
	if !ok {
		t.Fatal("tsconfig.json missing 'include' array")
	}
	if len(includes) != 1 {
		t.Fatalf("expected 1 include path, got %d", len(includes))
	}
	// Directory paths get /**/* suffix for TypeScript.
	if includes[0] != "src/components/**/*" {
		t.Errorf("include[0] = %q, want %q", includes[0], "src/components/**/*")
	}

	// Verify compilerOptions exists.
	if _, ok := config["compilerOptions"]; !ok {
		t.Error("tsconfig.json missing 'compilerOptions'")
	}

	// Cleanup.
	RemoveScopeConfig(sc)
}

func TestGenerateScopeConfig_Go_NoOp(t *testing.T) {
	rootDir := t.TempDir()
	sc, err := GenerateScopeConfig(rootDir, "go", []string{"pkg/api"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sc != nil {
		t.Errorf("expected nil ScopeConfig for go, got %+v", sc)
	}

	// Verify no config files were created.
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty rootDir, found %d entries", len(entries))
	}
}

func TestRemoveScopeConfig_Cleanup(t *testing.T) {
	rootDir := t.TempDir()

	sc, err := GenerateScopeConfig(rootDir, "python", []string{"src"})
	if err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(rootDir, "pyrightconfig.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("pyrightconfig.json should exist before removal")
	}

	RemoveScopeConfig(sc)

	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Error("pyrightconfig.json should not exist after RemoveScopeConfig")
	}
}

func TestGenerateScopeConfig_BackupExisting(t *testing.T) {
	rootDir := t.TempDir()

	// Create an existing pyrightconfig.json with custom content.
	configPath := filepath.Join(rootDir, "pyrightconfig.json")
	originalContent := []byte(`{"pythonVersion": "3.9", "custom": true}`)
	if err := os.WriteFile(configPath, originalContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Generate scope config, which should back up the existing file.
	sc, err := GenerateScopeConfig(rootDir, "python", []string{"lib"})
	if err != nil {
		t.Fatalf("GenerateScopeConfig: %v", err)
	}

	// Verify backup was created.
	backupPath := configPath + ".agent-lsp-backup"
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("backup file should exist: %v", err)
	}
	if string(backupData) != string(originalContent) {
		t.Errorf("backup content = %q, want %q", string(backupData), string(originalContent))
	}

	// Verify the generated config has the scope paths, not the original content.
	genData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var genConfig map[string]interface{}
	if err := json.Unmarshal(genData, &genConfig); err != nil {
		t.Fatal(err)
	}
	if _, ok := genConfig["custom"]; ok {
		t.Error("generated config should not contain 'custom' key from original")
	}

	// RemoveScopeConfig should restore the original.
	RemoveScopeConfig(sc)

	restoredData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("original config should be restored: %v", err)
	}
	if string(restoredData) != string(originalContent) {
		t.Errorf("restored content = %q, want %q", string(restoredData), string(originalContent))
	}

	// Backup file should be gone.
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Error("backup file should have been removed after restore")
	}
}
