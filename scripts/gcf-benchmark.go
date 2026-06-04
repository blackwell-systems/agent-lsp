// +build ignore

// Quick benchmark comparing JSON vs GCF token counts on representative tool responses.
// Run: go run scripts/gcf-benchmark.go
package main

import (
	"encoding/json"
	"fmt"
	"strings"

	gcf "github.com/blackwell-systems/agent-lsp/internal/encoding/gcf"
)

// Representative blast_radius response
var blastRadiusResponse = map[string]any{
	"changed_symbols": []map[string]any{
		{"name": "AuthMiddleware", "file": "internal/auth/middleware.go", "line": 45},
		{"name": "NewServer", "file": "internal/server/server.go", "line": 12},
		{"name": "HandleRequest", "file": "internal/handler/request.go", "line": 78},
		{"name": "ValidateToken", "file": "internal/auth/token.go", "line": 23},
		{"name": "NewRouter", "file": "internal/router/router.go", "line": 15},
	},
	"affected_symbols": []map[string]any{
		{"name": "AuthMiddleware", "file": "internal/auth/middleware.go", "line": 45, "test_callers": 3, "non_test_callers": 8, "sync_guarded": false},
		{"name": "NewServer", "file": "internal/server/server.go", "line": 12, "test_callers": 2, "non_test_callers": 5, "sync_guarded": false},
		{"name": "HandleRequest", "file": "internal/handler/request.go", "line": 78, "test_callers": 4, "non_test_callers": 12, "sync_guarded": false},
		{"name": "ValidateToken", "file": "internal/auth/token.go", "line": 23, "test_callers": 5, "non_test_callers": 3, "sync_guarded": false},
		{"name": "NewRouter", "file": "internal/router/router.go", "line": 15, "test_callers": 1, "non_test_callers": 6, "sync_guarded": false},
	},
	"test_files":     []string{"internal/auth/middleware_test.go", "internal/server/server_test.go", "internal/handler/request_test.go", "internal/auth/token_test.go", "internal/router/router_test.go"},
	"test_functions":  []string{"TestAuthMiddleware", "TestNewServer", "TestHandleRequest", "TestValidateToken", "TestNewRouter", "TestAuthFlow", "TestServerStart"},
	"non_test_callers": []map[string]any{
		{"name": "main", "file": "cmd/server/main.go", "line": 15},
		{"name": "Setup", "file": "internal/setup/setup.go", "line": 30},
		{"name": "InitApp", "file": "internal/app/init.go", "line": 8},
		{"name": "RegisterRoutes", "file": "internal/router/register.go", "line": 42},
		{"name": "StartHTTP", "file": "internal/http/start.go", "line": 19},
		{"name": "LoadConfig", "file": "internal/config/load.go", "line": 55},
	},
	"summary":  "Found 5 changed symbols with 15 test references across 5 test files.",
	"warnings": []string{},
}

// Representative find_references response
var findReferencesResponse = []map[string]any{
	{"file": "internal/auth/middleware.go", "line": 45, "character": 5},
	{"file": "internal/server/server.go", "line": 88, "character": 12},
	{"file": "internal/handler/request.go", "line": 15, "character": 8},
	{"file": "internal/router/router.go", "line": 33, "character": 20},
	{"file": "cmd/server/main.go", "line": 22, "character": 4},
	{"file": "internal/setup/setup.go", "line": 67, "character": 15},
	{"file": "internal/auth/middleware_test.go", "line": 12, "character": 8},
	{"file": "internal/auth/middleware_test.go", "line": 45, "character": 8},
	{"file": "internal/server/server_test.go", "line": 30, "character": 12},
	{"file": "internal/handler/request_test.go", "line": 18, "character": 5},
	{"file": "internal/handler/request_test.go", "line": 55, "character": 5},
	{"file": "internal/handler/request_test.go", "line": 92, "character": 5},
}

// Representative list_symbols response
var listSymbolsResponse = []map[string]any{
	{"name": "AuthMiddleware", "kind": "Function", "detail": "func(http.Handler) http.Handler", "file": "middleware.go", "start_line": 45, "end_line": 78},
	{"name": "ValidateToken", "kind": "Function", "detail": "func(string) (*Claims, error)", "file": "token.go", "start_line": 23, "end_line": 56},
	{"name": "Claims", "kind": "Struct", "detail": "struct", "file": "token.go", "start_line": 10, "end_line": 18},
	{"name": "TokenExpiredError", "kind": "Variable", "detail": "var error", "file": "errors.go", "start_line": 5, "end_line": 5},
	{"name": "NewAuthService", "kind": "Function", "detail": "func(Config) *AuthService", "file": "service.go", "start_line": 30, "end_line": 45},
	{"name": "AuthService", "kind": "Struct", "detail": "struct", "file": "service.go", "start_line": 12, "end_line": 28},
	{"name": "Config", "kind": "Struct", "detail": "struct", "file": "config.go", "start_line": 8, "end_line": 15},
	{"name": "DefaultConfig", "kind": "Function", "detail": "func() Config", "file": "config.go", "start_line": 17, "end_line": 25},
	{"name": "ErrUnauthorized", "kind": "Variable", "detail": "var error", "file": "errors.go", "start_line": 6, "end_line": 6},
	{"name": "ErrTokenMalformed", "kind": "Variable", "detail": "var error", "file": "errors.go", "start_line": 7, "end_line": 7},
}

// Representative get_diagnostics response
var diagnosticsResponse = []map[string]any{
	{"file": "internal/auth/middleware.go", "line": 52, "character": 8, "severity": "error", "message": "undefined: jwt.Parse", "source": "gopls"},
	{"file": "internal/auth/middleware.go", "line": 67, "character": 15, "severity": "warning", "message": "error return value not checked", "source": "gopls"},
	{"file": "internal/auth/token.go", "line": 34, "character": 2, "severity": "error", "message": "cannot use claims (variable of type *Claims) as jwt.Claims", "source": "gopls"},
	{"file": "internal/server/server.go", "line": 90, "character": 12, "severity": "information", "message": "unused parameter: ctx", "source": "gopls"},
	{"file": "internal/handler/request.go", "line": 15, "character": 1, "severity": "warning", "message": "exported function HandleRequest should have comment", "source": "gopls"},
}

// estimateTokens gives a rough token count (byte length / 3.5 for ASCII-dominant text)
func estimateTokens(s string) int {
	return int(float64(len(s)) / 3.5)
}

func benchmark(name string, data any) {
	jsonBytes, _ := json.Marshal(data)
	jsonStr := string(jsonBytes)

	gcfStr, _ := gcf.Encode(data)

	jsonTokens := estimateTokens(jsonStr)
	gcfTokens := estimateTokens(gcfStr)
	savings := 0.0
	if jsonTokens > 0 {
		savings = float64(jsonTokens-gcfTokens) / float64(jsonTokens) * 100
	}

	fmt.Printf("\n## %s\n", name)
	fmt.Printf("  JSON:   %6d bytes  ~%4d tokens\n", len(jsonStr), jsonTokens)
	fmt.Printf("  GCF:    %6d bytes  ~%4d tokens\n", len(gcfStr), gcfTokens)
	fmt.Printf("  Saving: %5.1f%%\n", savings)

	// Show first 3 lines of GCF output
	lines := strings.Split(gcfStr, "\n")
	preview := lines
	if len(preview) > 5 {
		preview = preview[:5]
	}
	fmt.Printf("  GCF preview:\n")
	for _, l := range preview {
		fmt.Printf("    %s\n", l)
	}
}

func main() {
	fmt.Println("# GCF vs JSON Token Comparison")
	fmt.Println("# Byte/3.5 token estimate (validated: 0.97 correlation with o200k_base)")

	benchmark("blast_radius (5 symbols, 6 callers)", blastRadiusResponse)
	benchmark("find_references (12 locations)", findReferencesResponse)
	benchmark("list_symbols (10 symbols)", listSymbolsResponse)
	benchmark("get_diagnostics (5 diagnostics)", diagnosticsResponse)

	// Scale test: 50 references
	var bigRefs []map[string]any
	for i := range 50 {
		bigRefs = append(bigRefs, map[string]any{
			"file":      fmt.Sprintf("internal/pkg%d/file%d.go", i/5, i),
			"line":      i*10 + 5,
			"character": i%20 + 1,
		})
	}
	benchmark("find_references (50 locations)", bigRefs)

	// Scale test: 30 symbols
	var bigSymbols []map[string]any
	for i := range 30 {
		bigSymbols = append(bigSymbols, map[string]any{
			"name":       fmt.Sprintf("Function%d", i),
			"kind":       "Function",
			"detail":     fmt.Sprintf("func(arg%d Type%d) Result%d", i, i, i),
			"file":       fmt.Sprintf("pkg%d.go", i/3),
			"start_line": i*20 + 1,
			"end_line":   i*20 + 15,
		})
	}
	benchmark("list_symbols (30 symbols)", bigSymbols)
}
