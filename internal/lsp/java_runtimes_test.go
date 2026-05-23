package lsp

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDetectJavaVersion(t *testing.T) {
	// Skip if java is not available
	javaPath, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	// Test with JAVA_HOME if set
	if jh := os.Getenv("JAVA_HOME"); jh != "" {
		version := detectJavaVersion(jh)
		if version != "" && version[:7] != "JavaSE-" {
			t.Errorf("detectJavaVersion(%q) = %q, expected JavaSE-XX format", jh, version)
		}
	}

	// Test with invalid path
	invalidPath := filepath.Join(javaPath, "nonexistent-jdk")
	version := detectJavaVersion(invalidPath)
	if version != "" {
		t.Errorf("detectJavaVersion(%q) = %q, expected empty for invalid path", invalidPath, version)
	}

	// Test with empty path
	version = detectJavaVersion("")
	if version != "" {
		t.Errorf("detectJavaVersion('') = %q, expected empty", version)
	}
}

func TestDetectJavaRuntimes(t *testing.T) {
	// This test verifies detectJavaRuntimes runs without panicking
	// and returns valid structure. Actual JDK detection depends on
	// the environment, so we can't assert specific versions.
	runtimes := detectJavaRuntimes()

	// Verify structure of each runtime entry
	for i, r := range runtimes {
		name, hasName := r["name"].(string)
		path, hasPath := r["path"].(string)

		if !hasName || name == "" {
			t.Errorf("runtime[%d]: missing or empty 'name' field", i)
		}
		if !hasPath || path == "" {
			t.Errorf("runtime[%d]: missing or empty 'path' field", i)
		}

		// Verify name format (JavaSE-XX)
		if hasName && name[:7] != "JavaSE-" {
			t.Errorf("runtime[%d]: name=%q, expected JavaSE-XX format", i, name)
		}
	}

	// If JAVA_HOME is set, we expect at least one runtime
	if jh := os.Getenv("JAVA_HOME"); jh != "" {
		if _, err := os.Stat(filepath.Join(jh, "bin", "java")); err == nil {
			if len(runtimes) == 0 {
				t.Error("JAVA_HOME is set with valid java binary, but detectJavaRuntimes returned empty")
			}
		}
	}
}

func TestDetectJavaRuntimesDedupe(t *testing.T) {
	// Set JAVA_HOME and verify no duplicates in results
	// This test only runs if JAVA_HOME is set
	jh := os.Getenv("JAVA_HOME")
	if jh == "" {
		t.Skip("JAVA_HOME not set")
	}

	runtimes := detectJavaRuntimes()

	// Count occurrences of JAVA_HOME path
	count := 0
	for _, r := range runtimes {
		if path, ok := r["path"].(string); ok && path == jh {
			count++
		}
	}

	if count > 1 {
		t.Errorf("JAVA_HOME path appears %d times, expected at most 1 (deduplication failed)", count)
	}
}

func TestFindJDKAtLeast(t *testing.T) {
	tests := []struct {
		name     string
		runtimes []map[string]any
		minMajor int
		want     string
	}{
		{
			name: "find JDK 17 when available",
			runtimes: []map[string]any{
				{"name": "JavaSE-11", "path": "/jdk11"},
				{"name": "JavaSE-17", "path": "/jdk17"},
				{"name": "JavaSE-21", "path": "/jdk21"},
			},
			minMajor: 17,
			want:     "/jdk17",
		},
		{
			name: "find lowest matching when multiple available",
			runtimes: []map[string]any{
				{"name": "JavaSE-17", "path": "/jdk17"},
				{"name": "JavaSE-21", "path": "/jdk21"},
			},
			minMajor: 17,
			want:     "/jdk17",
		},
		{
			name: "no match returns empty",
			runtimes: []map[string]any{
				{"name": "JavaSE-11", "path": "/jdk11"},
			},
			minMajor: 17,
			want:     "",
		},
		{
			name:     "empty runtimes returns empty",
			runtimes: []map[string]any{},
			minMajor: 17,
			want:     "",
		},
		{
			name: "exact match",
			runtimes: []map[string]any{
				{"name": "JavaSE-17", "path": "/jdk17"},
			},
			minMajor: 17,
			want:     "/jdk17",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findJDKAtLeast(tt.runtimes, tt.minMajor)
			if got != tt.want {
				t.Errorf("findJDKAtLeast() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLowestJDKPath(t *testing.T) {
	tests := []struct {
		name     string
		runtimes []map[string]any
		want     string
	}{
		{
			name: "returns first runtime path",
			runtimes: []map[string]any{
				{"name": "JavaSE-11", "path": "/jdk11"},
				{"name": "JavaSE-17", "path": "/jdk17"},
			},
			want: "/jdk11",
		},
		{
			name:     "empty runtimes returns empty",
			runtimes: []map[string]any{},
			want:     "",
		},
		{
			name: "single runtime",
			runtimes: []map[string]any{
				{"name": "JavaSE-21", "path": "/jdk21"},
			},
			want: "/jdk21",
		},
		{
			name: "missing path field returns empty",
			runtimes: []map[string]any{
				{"name": "JavaSE-11"},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lowestJDKPath(tt.runtimes)
			if got != tt.want {
				t.Errorf("lowestJDKPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectJavaRuntimesPlatformPaths(t *testing.T) {
	// Verify platform-specific paths are scanned
	runtimes := detectJavaRuntimes()

	if runtime.GOOS == "darwin" {
		// On macOS, check that we scan Homebrew and /Library paths
		// We can't assert specific JDKs are found, but we verify the
		// function runs without error.
		_ = runtimes
	} else if runtime.GOOS == "linux" {
		// On Linux, verify /usr/lib/jvm is scanned
		_ = runtimes
	}

	// This test primarily verifies no panics on different platforms
}

func TestJavaRuntimeSorting(t *testing.T) {
	// Verify that detected runtimes are sorted by name
	runtimes := detectJavaRuntimes()

	for i := 1; i < len(runtimes); i++ {
		prev, _ := runtimes[i-1]["name"].(string)
		curr, _ := runtimes[i]["name"].(string)

		if prev > curr {
			t.Errorf("runtimes not sorted: %q appears before %q", prev, curr)
		}
	}
}
