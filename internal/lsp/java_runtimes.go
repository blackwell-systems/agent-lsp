package lsp

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// javaRuntime represents a JDK installation for jdtls configuration.
type javaRuntime struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// detectJavaRuntimes scans standard locations for installed JDKs and returns
// entries suitable for jdtls's java.configuration.runtimes setting.
func detectJavaRuntimes() []map[string]any {
	seen := map[string]bool{}
	var runtimes []javaRuntime

	add := func(path string) {
		path = filepath.Clean(path)
		if seen[path] {
			return
		}
		// Verify it's a real JDK by checking for bin/java
		javabin := filepath.Join(path, "bin", "java")
		if _, err := os.Stat(javabin); err != nil {
			return
		}
		version := detectJavaVersion(path)
		if version == "" {
			return
		}
		seen[path] = true
		runtimes = append(runtimes, javaRuntime{Name: version, Path: path})
	}

	// $JAVA_HOME
	if jh := os.Getenv("JAVA_HOME"); jh != "" {
		add(jh)
	}

	if runtime.GOOS == "darwin" {
		// Homebrew: /opt/homebrew/opt/openjdk*, /usr/local/opt/openjdk*
		for _, prefix := range []string{"/opt/homebrew/opt", "/usr/local/opt"} {
			entries, _ := filepath.Glob(prefix + "/openjdk*")
			for _, e := range entries {
				add(e)
				// Homebrew often has a libexec/openjdk.jdk/Contents/Home symlink
				add(filepath.Join(e, "libexec", "openjdk.jdk", "Contents", "Home"))
			}
		}
		// /Library/Java/JavaVirtualMachines/*/Contents/Home
		entries, _ := filepath.Glob("/Library/Java/JavaVirtualMachines/*/Contents/Home")
		for _, e := range entries {
			add(e)
		}
	} else {
		// Linux: /usr/lib/jvm/java-*
		entries, _ := filepath.Glob("/usr/lib/jvm/java-*")
		for _, e := range entries {
			add(e)
		}
	}

	// SDKMAN
	if sdkman := os.Getenv("SDKMAN_DIR"); sdkman != "" {
		entries, _ := filepath.Glob(filepath.Join(sdkman, "candidates", "java", "*"))
		for _, e := range entries {
			if filepath.Base(e) != "current" {
				add(e)
			}
		}
	}

	// Sort by version name for deterministic output
	sort.Slice(runtimes, func(i, j int) bool {
		return runtimes[i].Name < runtimes[j].Name
	})

	result := make([]map[string]any, len(runtimes))
	for i, r := range runtimes {
		result[i] = map[string]any{"name": r.Name, "path": r.Path}
	}
	return result
}

var javaVersionRe = regexp.MustCompile(`"(\d+)`)

// detectJavaVersion runs java -version and returns a JavaSE name like "JavaSE-17".
func detectJavaVersion(jdkPath string) string {
	javabin := filepath.Join(jdkPath, "bin", "java")
	out, err := exec.Command(javabin, "-version").CombinedOutput()
	if err != nil {
		return ""
	}
	s := string(out)
	// java -version outputs something like: openjdk version "17.0.2" or "21.0.1"
	m := javaVersionRe.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	major := m[1]
	// JDK 8 and below use 1.x naming
	if strings.HasPrefix(major, "1") {
		return ""
	}
	return "JavaSE-" + major
}
