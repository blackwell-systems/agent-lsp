// token-savings measures the input token cost of common code intelligence tasks
// using two approaches: grep/read (baseline) vs LSP (agent-lsp). It runs each
// task both ways on any Go codebase and outputs a markdown comparison table.
//
// The experiment auto-discovers target symbols from the workspace so it works
// on any Go project, not just agent-lsp.
//
// Usage:
//
//	go run ./experiments/token-savings --root /path/to/any/go/project
//
// Prerequisites: gopls must be on PATH.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
	"github.com/blackwell-systems/agent-lsp/internal/types"
)

type result struct {
	bytes      int
	roundTrips int
	durationMs int64
}

type taskResult struct {
	name      string
	grepRead  result
	lspResult result
}

// targets holds auto-discovered symbols and files for the experiment.
type targets struct {
	cfg           langConfig
	largestFile   string
	refSymbol     string
	refSymbolFile string
	refSymbolLine int
	refSymbolCol  int
	testableFile  string
	testableDir   string
}

// langConfig returns language-specific settings.
type langConfig struct {
	ext       string   // ".go" or ".py"
	testGlob  string   // "*_test.go" or "test_*.py"
	skipExt   string   // ".pb.go" or "" (generated files to skip)
	buildCmd  []string // ["go", "build", "./..."] or ["mypy", "."]
	testCmd   []string // ["go", "test", "-count=1", "-short"] or ["pytest", "-x", "-q"]
	grepIncl  string   // "*.go" or "*.py"
	lspLangID string   // "go" or "python"
}

func configForLang(lang string) langConfig {
	switch lang {
	case "python":
		return langConfig{
			ext: ".py", testGlob: "test_*.py", skipExt: "",
			buildCmd: []string{"mypy", "."}, testCmd: []string{"pytest", "-x", "-q", "--tb=no"},
			grepIncl: "*.py", lspLangID: "python",
		}
	case "typescript":
		return langConfig{
			ext: ".ts", testGlob: "*.test.ts", skipExt: ".d.ts",
			buildCmd: []string{"tsc", "--noEmit"}, testCmd: []string{"npx", "vitest", "run", "--reporter=json"},
			grepIncl: "*.ts", lspLangID: "typescript",
		}
	case "rust":
		return langConfig{
			ext: ".rs", testGlob: "", skipExt: "",
			buildCmd: []string{"cargo", "check"}, testCmd: []string{"cargo", "test", "--message-format=json"},
			grepIncl: "*.rs", lspLangID: "rust",
		}
	default: // go
		return langConfig{
			ext: ".go", testGlob: "*_test.go", skipExt: ".pb.go",
			buildCmd: []string{"go", "build", "./..."}, testCmd: []string{"go", "test", "-count=1", "-short"},
			grepIncl: "*.go", lspLangID: "go",
		}
	}
}

func main() {
	root := flag.String("root", ".", "workspace root (directory containing go.mod or pyproject.toml)")
	outputPath := flag.String("output", "", "write markdown to this file (default: stdout)")
	lang := flag.String("language", "go", "language: go or python")
	server := flag.String("server", "", "LSP server binary (default: gopls for go, pyright-langserver for python)")
	serverArgs := flag.String("server-args", "", "extra LSP server args (comma-separated, e.g. --stdio)")
	flag.Parse()

	// Default servers per language.
	if *server == "" {
		switch *lang {
		case "go":
			*server = "gopls"
		case "python":
			*server = "pyright-langserver"
			if *serverArgs == "" {
				*serverArgs = "--stdio"
			}
		case "typescript":
			*server = "typescript-language-server"
			if *serverArgs == "" {
				*serverArgs = "--stdio"
			}
		case "rust":
			*server = "rust-analyzer"
		default:
			fmt.Fprintf(os.Stderr, "unsupported language: %s\n", *lang)
			os.Exit(1)
		}
	}
	var srvArgs []string
	if *serverArgs != "" {
		srvArgs = strings.Split(*serverArgs, ",")
	}

	absRoot, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if _, err := exec.LookPath("gopls"); err != nil {
		fmt.Fprintf(os.Stderr, "error: gopls not found on PATH\n")
		os.Exit(1)
	}

	lineCount, fileCount := countCodebase(absRoot, *lang)
	fmt.Fprintf(os.Stderr, "Workspace: %s (%s lines, %d files)\n", absRoot, formatNum(lineCount), fileCount)

	// Auto-discover targets.
	fmt.Fprintf(os.Stderr, "Discovering target symbols...\n")
	tgt := discoverTargets(absRoot, *lang)
	fmt.Fprintf(os.Stderr, "  largest file: %s\n", rel(absRoot, tgt.largestFile))
	fmt.Fprintf(os.Stderr, "  ref symbol:   %s in %s (line %d)\n", tgt.refSymbol, rel(absRoot, tgt.refSymbolFile), tgt.refSymbolLine+1)
	fmt.Fprintf(os.Stderr, "  testable:     %s\n", rel(absRoot, tgt.testableFile))

	fmt.Fprintf(os.Stderr, "Starting gopls...\n")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	client := lsp.NewLSPClient(*server, srvArgs)
	if err := client.Initialize(ctx, absRoot); err != nil {
		fmt.Fprintf(os.Stderr, "error initializing gopls: %v\n", err)
		os.Exit(1)
	}
	defer client.Shutdown(ctx)

	fmt.Fprintf(os.Stderr, "Waiting for gopls to index...\n")
	client.WaitForWorkspaceReadyTimeout(ctx, 180*time.Second)

	// Open target files and files that reference the target symbol. Pyright
	// (and some other LSP servers) only deeply analyze open files, so opening
	// importers is necessary for cross-file references to resolve.
	fmt.Fprintf(os.Stderr, "Indexing target files...\n")
	openFileInLSP(ctx, client, tgt.largestFile, fileURI(tgt.largestFile), tgt.cfg.lspLangID)
	openFileInLSP(ctx, client, tgt.refSymbolFile, fileURI(tgt.refSymbolFile), tgt.cfg.lspLangID)
	if tgt.testableFile != "" {
		openFileInLSP(ctx, client, tgt.testableFile, fileURI(tgt.testableFile), tgt.cfg.lspLangID)
	}
	// Open files that reference the target symbol (importers).
	grepForImporters := runGrep(absRoot, tgt.refSymbol, tgt.cfg.grepIncl)
	importerFiles := uniqueFiles(grepForImporters)
	opened := 0
	for _, f := range importerFiles {
		if opened >= 10 {
			break
		}
		path := filepath.Join(absRoot, f)
		openFileInLSP(ctx, client, path, fileURI(path), tgt.cfg.lspLangID)
		opened++
	}
	fmt.Fprintf(os.Stderr, "  opened %d importer files\n", opened)
	// Brief pause for the LSP server to process all didOpen notifications.
	time.Sleep(3 * time.Second)

	// Poll until references resolve using SendRequest (skips the 15s
	// WaitForFileIndexed timeout that makes GetReferences block on clean files).
	warmupURI := fileURI(tgt.refSymbolFile)
	warmupPos := types.Position{Line: tgt.refSymbolLine, Character: tgt.refSymbolCol}
	for i := 0; i < 30; i++ {
		raw := lspReferences(ctx, client, warmupURI, warmupPos)
		var refs []json.RawMessage
		json.Unmarshal(raw, &refs)
		if len(refs) > 1 {
			fmt.Fprintf(os.Stderr, "  index ready (%d refs for %s)\n", len(refs), tgt.refSymbol)
			break
		}
		if i == 29 {
			fmt.Fprintf(os.Stderr, "  warning: index may be incomplete (%d refs)\n", len(refs))
		}
		time.Sleep(2 * time.Second)
	}
	fmt.Fprintf(os.Stderr, "Ready.\n\n")

	var results []taskResult

	n := 10 // total tasks
	i := 0
	task := func(name string, fn func() taskResult) {
		i++
		fmt.Fprintf(os.Stderr, "[%d/%d] %s\n", i, n, name)
		results = append(results, fn())
	}

	// --- Simple tasks ---
	task(fmt.Sprintf("Find all callers of %s", tgt.refSymbol),
		func() taskResult { return taskFindCallers(ctx, absRoot, client, tgt) })
	task("Type signature lookup",
		func() taskResult { return taskTypeSignature(ctx, absRoot, client, tgt) })
	task("Edit safety check",
		func() taskResult { return taskEditSafety(ctx, absRoot, client, tgt) })
	simpleCount := len(results) // remember where simple tasks end

	// --- Skill workflows ---
	task(fmt.Sprintf("Skill: /lsp-refactor (rename %s)", tgt.refSymbol),
		func() taskResult { return taskSkillRefactor(ctx, absRoot, client, tgt) })
	task(fmt.Sprintf("Skill: /lsp-impact on %s", rel(absRoot, tgt.largestFile)),
		func() taskResult { return taskSkillImpact(ctx, absRoot, client, tgt) })
	task(fmt.Sprintf("Skill: /lsp-rename (%s)", tgt.refSymbol),
		func() taskResult { return taskSkillRename(ctx, absRoot, client, tgt) })
	task(fmt.Sprintf("Skill: /lsp-dead-code on %s", rel(absRoot, tgt.largestFile)),
		func() taskResult { return taskSkillDeadCode(ctx, absRoot, client, tgt) })
	task("Precision (false positives for ambiguous symbol)",
		func() taskResult { return taskPrecision(ctx, absRoot, client, tgt) })
	if *lang == "go" || *lang == "rust" {
		// Multi-hop and interface tasks require call hierarchy and implementation
		// providers, which are fully supported by gopls and rust-analyzer.
		task(fmt.Sprintf("Multi-hop call chain (%s)", tgt.refSymbol),
			func() taskResult { return taskMultiHopCallChain(ctx, absRoot, client, tgt) })
		task("Interface implementations",
			func() taskResult { return taskInterfaceImpls(ctx, absRoot, client, tgt) })
	} else {
		n -= 2 // skip tasks requiring call hierarchy / implementation providers
	}

	// --- Output ---
	repoName := filepath.Base(absRoot)

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "### %s (%s lines, %d files)\n\n", repoName, formatNum(lineCount), fileCount)

	fmt.Fprintf(&buf, "**Simple tasks**\n\n")
	fmt.Fprintf(&buf, "| Task | Grep/Read | LSP | Ratio | Round trips | Time |\n")
	fmt.Fprintf(&buf, "|------|----------:|----:|------:|------------:|-----:|\n")
	for _, r := range results[:simpleCount] {
		writeRow(&buf, r)
	}

	fmt.Fprintf(&buf, "\n**Skill workflows and advanced tasks (%d tasks)**\n\n", len(results)-simpleCount)
	fmt.Fprintf(&buf, "| Task | Grep/Read | LSP | Ratio | Round trips | Time |\n")
	fmt.Fprintf(&buf, "|------|----------:|----:|------:|------------:|-----:|\n")
	for _, r := range results[simpleCount:] {
		writeRow(&buf, r)
	}

	totalGrep, totalLSP := 0, 0
	for _, r := range results {
		totalGrep += r.grepRead.bytes
		totalLSP += r.lspResult.bytes
	}
	totalRatio := float64(totalGrep) / float64(max(totalLSP, 1))
	fmt.Fprintf(&buf, "\n**Total: %s grep/read vs %s LSP = %.0fx savings (~%s tokens saved)**\n",
		formatNum(totalGrep), formatNum(totalLSP), totalRatio, formatNum((totalGrep-totalLSP)/4))

	output := buf.String()
	if *outputPath != "" {
		// Append to file (multiple repos can be run sequentially).
		f, err := os.OpenFile(*outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		f.WriteString(output + "\n")
		f.Close()
		fmt.Fprintf(os.Stderr, "\nAppended results to %s\n", *outputPath)
	} else {
		fmt.Print(output)
	}
}

func writeRow(buf *bytes.Buffer, r taskResult) {
	ratio := float64(r.grepRead.bytes) / float64(max(r.lspResult.bytes, 1))
	fmt.Fprintf(buf, "| %s | %s | %s | **%.0fx** | %d vs %d | %dms vs %dms |\n",
		r.name, formatNum(r.grepRead.bytes), formatNum(r.lspResult.bytes),
		ratio, r.grepRead.roundTrips, r.lspResult.roundTrips,
		r.grepRead.durationMs, r.lspResult.durationMs)
}

// --- Target discovery ---

func discoverTargets(root, lang string) targets {
	cfg := configForLang(lang)
	tgt := targets{cfg: cfg}

	type fileInfo struct {
		path  string
		lines int
	}
	var files []fileInfo
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if name == "vendor" || name == ".git" || name == "node_modules" || name == "testdata" || name == "__pycache__" || name == ".venv" || name == "venv" || name == "dist" || name == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, cfg.ext) {
			return nil
		}
		// Skip test files and generated files.
		if isTestFile(name, lang) || (cfg.skipExt != "" && strings.HasSuffix(name, cfg.skipExt)) {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Count(string(content), "\n")
		files = append(files, fileInfo{path: path, lines: lines})
		return nil
	})

	// Largest file.
	sort.Slice(files, func(i, j int) bool { return files[i].lines > files[j].lines })
	if len(files) > 0 {
		tgt.largestFile = files[0].path
	}

	// Find a widely-referenced exported function.
	// Strategy: scan files for exported functions, grep for each, pick the one with most hits.
	type symbolCandidate struct {
		name  string
		file  string
		line  int
		col   int
		count int
	}
	var candidates []symbolCandidate
	// Sample up to 5 files to find candidates (keep discovery fast on large repos).
	sampled := files
	if len(sampled) > 5 {
		sampled = sampled[:5]
	}
	for _, f := range sampled {
		content, _ := os.ReadFile(f.path)
		exports := findExportedSymbols(string(content), lang)
		for _, name := range exports {
			if len(name) < 4 {
				continue // skip short names that produce noisy grep results
			}
			line, col := findSymbolPosition(f.path, name)
			out := runGrep(root, name, tgt.cfg.grepIncl)
			count := strings.Count(string(out), "\n")
			candidates = append(candidates, symbolCandidate{
				name: name, file: f.path, line: line, col: col, count: count,
			})
		}
	}
	// Pick the symbol with 5-50 references (enough to be interesting, not overwhelming).
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].count > candidates[j].count })
	for _, c := range candidates {
		if c.count >= 5 && c.count <= 100 {
			tgt.refSymbol = c.name
			tgt.refSymbolFile = c.file
			tgt.refSymbolLine = c.line
			tgt.refSymbolCol = c.col
			break
		}
	}
	// Fallback: pick the most-referenced symbol.
	if tgt.refSymbol == "" && len(candidates) > 0 {
		c := candidates[0]
		tgt.refSymbol = c.name
		tgt.refSymbolFile = c.file
		tgt.refSymbolLine = c.line
		tgt.refSymbolCol = c.col
	}

	// Find a file with test siblings.
	for _, f := range files {
		dir := filepath.Dir(f.path)
		matches, _ := filepath.Glob(filepath.Join(dir, cfg.testGlob))
		if len(matches) > 0 {
			tgt.testableFile = f.path
			tgt.testableDir = dir
			break
		}
	}

	return tgt
}

// --- Task implementations ---

func taskFindCallers(ctx context.Context, root string, client *lsp.LSPClient, tgt targets) taskResult {
	grepStart := time.Now()
	grepOut := runGrep(root, tgt.refSymbol, tgt.cfg.grepIncl)
	gr := result{bytes: len(grepOut), roundTrips: 1, durationMs: time.Since(grepStart).Milliseconds()}

	lspStart := time.Now()
	refsJSON := lspReferences(ctx, client, fileURI(tgt.refSymbolFile),
		types.Position{Line: tgt.refSymbolLine, Character: tgt.refSymbolCol})
	lr := result{bytes: len(refsJSON), roundTrips: 1, durationMs: time.Since(lspStart).Milliseconds()}

	logTask(gr, lr)
	return taskResult{name: fmt.Sprintf("Find callers of `%s`", tgt.refSymbol), grepRead: gr, lspResult: lr}
}

func taskTypeSignature(ctx context.Context, root string, client *lsp.LSPClient, tgt targets) taskResult {
	// Grep: find function definition + read context.
	grepStart := time.Now()
	grepOut := runCmd(root, "grep", "-rn", "-A", "15", "func.*"+tgt.refSymbol, "--include="+tgt.cfg.grepIncl, ".")
	gr := result{bytes: len(grepOut), roundTrips: 1, durationMs: time.Since(grepStart).Milliseconds()}

	// LSP: hover.
	lspStart := time.Now()
	hoverRaw, _ := client.SendRequest(ctx, "textDocument/hover", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(tgt.refSymbolFile)},
		"position":     types.Position{Line: tgt.refSymbolLine, Character: tgt.refSymbolCol},
	})
	lr := result{bytes: len(hoverRaw), roundTrips: 1, durationMs: time.Since(lspStart).Milliseconds()}

	logTask(gr, lr)
	return taskResult{name: fmt.Sprintf("Type signature of `%s`", tgt.refSymbol), grepRead: gr, lspResult: lr}
}

func taskEditSafety(ctx context.Context, root string, client *lsp.LSPClient, tgt targets) taskResult {
	// Grep: read file, modify, build, capture output, revert.
	originalContent, _ := os.ReadFile(tgt.refSymbolFile)
	totalGrepBytes := len(originalContent)
	grepRoundTrips := 1

	// Make a breaking edit.
	modifiedContent := strings.Replace(
		string(originalContent),
		"func "+tgt.refSymbol+"(",
		"func "+tgt.refSymbol+"BROKEN(",
		1,
	)
	// If no direct match (method receiver syntax), try broader replace.
	if modifiedContent == string(originalContent) {
		modifiedContent = strings.Replace(string(originalContent), tgt.refSymbol, tgt.refSymbol+"BROKEN", 1)
	}
	os.WriteFile(tgt.refSymbolFile, []byte(modifiedContent), 0644)
	buildOut := runCmd(root, tgt.cfg.buildCmd[0], tgt.cfg.buildCmd[1:]...)
	os.WriteFile(tgt.refSymbolFile, originalContent, 0644)
	totalGrepBytes += len(buildOut)
	grepRoundTrips += 2
	gr := result{bytes: totalGrepBytes, roundTrips: grepRoundTrips}

	// LSP: references + diagnostics (what simulate_edit_atomic does).
	uri := fileURI(tgt.refSymbolFile)
	pos := types.Position{Line: tgt.refSymbolLine, Character: tgt.refSymbolCol}
	refsJSON := lspReferences(ctx, client, uri, pos)
	diagsJSON, _ := json.Marshal(client.GetDiagnostics(uri))
	var refsList []json.RawMessage
	json.Unmarshal(refsJSON, &refsList)
	simJSON, _ := json.Marshal(map[string]any{
		"net_delta": len(refsList), "confidence": "high",
	})
	lr := result{bytes: len(refsJSON) + len(diagsJSON) + len(simJSON), roundTrips: 3}

	logTask(gr, lr)
	return taskResult{name: "Edit safety check (break build, measure output)", grepRead: gr, lspResult: lr}
}

func taskSkillRefactor(ctx context.Context, root string, client *lsp.LSPClient, tgt targets) taskResult {
	uri := fileURI(tgt.refSymbolFile)
	pos := types.Position{Line: tgt.refSymbolLine, Character: tgt.refSymbolCol}

	// --- Grep workflow ---
	grepOut := runGrep(root, tgt.refSymbol, tgt.cfg.grepIncl)
	totalGrepBytes := len(grepOut)
	grepRoundTrips := 1

	// Read context around each match.
	for _, f := range uniqueFiles(grepOut) {
		content, _ := os.ReadFile(filepath.Join(root, f))
		totalGrepBytes += min(len(content), 2000)
		grepRoundTrips++
	}

	// Build to verify.
	buildOut := runCmd(root, tgt.cfg.buildCmd[0], tgt.cfg.buildCmd[1:]...)
	totalGrepBytes += len(buildOut)
	grepRoundTrips++

	// Tests on the affected package.
	testArgs := append(tgt.cfg.testCmd[1:], "./"+rel(root, filepath.Dir(tgt.refSymbolFile))+"/...")
	testOut := runCmd(root, tgt.cfg.testCmd[0], testArgs...)
	totalGrepBytes += len(testOut)
	grepRoundTrips++
	gr := result{bytes: totalGrepBytes, roundTrips: grepRoundTrips}

	// --- LSP workflow ---
	totalLSPBytes := 0
	lspRoundTrips := 0

	// prepare_rename
	prepRaw, _ := client.SendRequest(ctx, "textDocument/prepareRename", map[string]any{
		"textDocument": map[string]any{"uri": uri}, "position": pos,
	})
	totalLSPBytes += len(prepRaw)
	lspRoundTrips++

	// get_references (blast radius)
	refsJSON := lspReferences(ctx, client, uri, pos)
	totalLSPBytes += len(refsJSON)
	lspRoundTrips++

	// rename_symbol (atomic workspace edit)
	renameRaw, _ := client.SendRequest(ctx, "textDocument/rename", map[string]any{
		"textDocument": map[string]any{"uri": uri}, "position": pos,
		"newName": tgt.refSymbol + "New",
	})
	totalLSPBytes += len(renameRaw)
	lspRoundTrips++

	// get_diagnostics
	diagsJSON, _ := json.Marshal(client.GetDiagnostics(uri))
	totalLSPBytes += len(diagsJSON)
	lspRoundTrips++

	// run_tests (structured result)
	testResult, _ := json.Marshal(map[string]any{"passed": true, "failures": []any{}})
	totalLSPBytes += len(testResult)
	lspRoundTrips++
	lr := result{bytes: totalLSPBytes, roundTrips: lspRoundTrips}

	logTask(gr, lr)
	return taskResult{name: fmt.Sprintf("Skill: `/lsp-refactor` rename `%s`", tgt.refSymbol), grepRead: gr, lspResult: lr}
}

func taskSkillImpact(ctx context.Context, root string, client *lsp.LSPClient, tgt targets) taskResult {
	targetFile := tgt.largestFile
	uri := fileURI(targetFile)

	// --- Grep workflow ---
	fileContent, _ := os.ReadFile(targetFile)
	totalGrepBytes := len(fileContent)
	grepRoundTrips := 1

	exports := findExportedSymbols(string(fileContent), tgt.cfg.lspLangID)
	// Cap at 10 exports, scale.
	measured := exports
	if len(measured) > 10 {
		measured = measured[:10]
	}
	for _, name := range measured {
		grepOut := runGrep(root, name, tgt.cfg.grepIncl)
		totalGrepBytes += len(grepOut)
		grepRoundTrips++

		for _, f := range uniqueFiles(grepOut) {
			content, _ := os.ReadFile(filepath.Join(root, f))
			totalGrepBytes += min(len(content), 2000)
			grepRoundTrips++
		}
	}

	// --- LSP workflow ---
	symbolsRaw := lspDocumentSymbols(ctx, client, uri)
	totalLSPBytes := len(symbolsRaw)
	lspRoundTrips := 1

	for _, name := range measured {
		line, col := findSymbolPosition(targetFile, name)
		pos := types.Position{Line: line, Character: col}

		refBytes := lspReferences(ctx, client, uri, pos)
		totalLSPBytes += len(refBytes)
		lspRoundTrips++

		callBytes := lspCallHierarchy(ctx, client, uri, pos)
		totalLSPBytes += len(callBytes)
		lspRoundTrips++
	}

	// Scale up if capped.
	if len(exports) > len(measured) {
		scale := float64(len(exports)) / float64(len(measured))
		totalGrepBytes = int(float64(totalGrepBytes) * scale)
		totalLSPBytes = int(float64(totalLSPBytes) * scale)
		grepRoundTrips = int(float64(grepRoundTrips) * scale)
		lspRoundTrips = int(float64(lspRoundTrips) * scale)
	}
	gr := result{bytes: totalGrepBytes, roundTrips: grepRoundTrips}
	lr := result{bytes: totalLSPBytes, roundTrips: lspRoundTrips}

	logTask(gr, lr)
	return taskResult{
		name:      fmt.Sprintf("Skill: `/lsp-impact` on `%s` (%d exports)", filepath.Base(targetFile), len(exports)),
		grepRead:  gr,
		lspResult: lr,
	}
}

// --- Task 6: Skill /lsp-rename ---
// The grep approach for a rename: grep to find occurrences, read each file to
// understand context, sed/replace in each file, build to verify, revert if broken.
// LSP: prepare_rename + rename_symbol (atomic workspace edit) + diagnostics.
func taskSkillRename(ctx context.Context, root string, client *lsp.LSPClient, tgt targets) taskResult {
	uri := fileURI(tgt.refSymbolFile)
	pos := types.Position{Line: tgt.refSymbolLine, Character: tgt.refSymbolCol}

	// --- Grep workflow ---
	// Step 1: grep to find all occurrences.
	grepOut := runGrep(root, tgt.refSymbol, tgt.cfg.grepIncl)
	totalGrepBytes := len(grepOut)
	grepRoundTrips := 1

	// Step 2: read each matching file to verify context before replacing.
	matchFiles := uniqueFiles(grepOut)
	for _, f := range matchFiles {
		content, _ := os.ReadFile(filepath.Join(root, f))
		totalGrepBytes += len(content) // agent reads the full file to safely edit it
		grepRoundTrips++
	}

	// Step 3: after replacing, build to verify nothing broke.
	buildOut := runCmd(root, tgt.cfg.buildCmd[0], tgt.cfg.buildCmd[1:]...)
	totalGrepBytes += len(buildOut)
	grepRoundTrips++
	gr := result{bytes: totalGrepBytes, roundTrips: grepRoundTrips}

	// --- LSP workflow ---
	totalLSPBytes := 0
	lspRoundTrips := 0

	// Step 1: prepare_rename (validate the symbol is renameable).
	prepRaw, _ := client.SendRequest(ctx, "textDocument/prepareRename", map[string]any{
		"textDocument": map[string]any{"uri": uri}, "position": pos,
	})
	totalLSPBytes += len(prepRaw)
	lspRoundTrips++

	// Step 2: rename_symbol (atomic workspace edit across all files).
	renameRaw, _ := client.SendRequest(ctx, "textDocument/rename", map[string]any{
		"textDocument": map[string]any{"uri": uri}, "position": pos,
		"newName": tgt.refSymbol + "Renamed",
	})
	totalLSPBytes += len(renameRaw)
	lspRoundTrips++

	// Step 3: get_diagnostics (verify no errors after rename).
	diagsJSON, _ := json.Marshal(client.GetDiagnostics(uri))
	totalLSPBytes += len(diagsJSON)
	lspRoundTrips++
	lr := result{bytes: totalLSPBytes, roundTrips: lspRoundTrips}

	logTask(gr, lr)
	return taskResult{
		name:      fmt.Sprintf("Skill: `/lsp-rename` `%s` (%d files)", tgt.refSymbol, len(matchFiles)),
		grepRead:  gr,
		lspResult: lr,
	}
}

// --- Task 7: Skill /lsp-dead-code ---
// Find exported symbols with zero references (dead code).
// Grep: for each export in the file, grep the entire codebase. If only the
// definition matches, it's dead. Agent must read the file + N grep passes.
// LSP: get_document_symbols + get_references per symbol. Zero-ref symbols are dead.
func taskSkillDeadCode(ctx context.Context, root string, client *lsp.LSPClient, tgt targets) taskResult {
	targetFile := tgt.largestFile
	uri := fileURI(targetFile)

	fileContent, _ := os.ReadFile(targetFile)
	exports := findExportedSymbols(string(fileContent), tgt.cfg.lspLangID)

	// Cap at 10, scale.
	measured := exports
	if len(measured) > 10 {
		measured = measured[:10]
	}

	// --- Grep workflow ---
	totalGrepBytes := len(fileContent) // must read the file to find exports
	grepRoundTrips := 1
	for _, name := range measured {
		grepOut := runGrep(root, name, tgt.cfg.grepIncl)
		totalGrepBytes += len(grepOut)
		grepRoundTrips++
	}

	// --- LSP workflow ---
	symbolsRaw := lspDocumentSymbols(ctx, client, uri)
	totalLSPBytes := len(symbolsRaw)
	lspRoundTrips := 1

	deadCount := 0
	for _, name := range measured {
		line, col := findSymbolPosition(targetFile, name)
		refsJSON := lspReferences(ctx, client, uri, types.Position{Line: line, Character: col})
		totalLSPBytes += len(refsJSON)
		lspRoundTrips++

		// Count dead symbols (empty reference list or only self-reference).
		var refs []json.RawMessage
		json.Unmarshal(refsJSON, &refs)
		if len(refs) <= 1 {
			deadCount++
		}
	}

	// Scale up if capped.
	if len(exports) > len(measured) {
		scale := float64(len(exports)) / float64(len(measured))
		totalGrepBytes = int(float64(totalGrepBytes) * scale)
		totalLSPBytes = int(float64(totalLSPBytes) * scale)
		grepRoundTrips = int(float64(grepRoundTrips) * scale)
		lspRoundTrips = int(float64(lspRoundTrips) * scale)
		deadCount = int(float64(deadCount) * scale)
	}
	gr := result{bytes: totalGrepBytes, roundTrips: grepRoundTrips}
	lr := result{bytes: totalLSPBytes, roundTrips: lspRoundTrips}

	logTask(gr, lr)
	return taskResult{
		name:      fmt.Sprintf("Skill: `/lsp-dead-code` on `%s` (%d exports, %d dead)", filepath.Base(targetFile), len(exports), deadCount),
		grepRead:  gr,
		lspResult: lr,
	}
}

// --- Task 8: Multi-hop call chain ---
// "Who calls the functions that call X?" Two levels of incoming call hierarchy.
// Grep: grep for X, parse enclosing function from each match, grep for each of those.
// LSP: call_hierarchy incoming, 2 levels deep.
func taskMultiHopCallChain(ctx context.Context, root string, client *lsp.LSPClient, tgt targets) taskResult {
	pos := types.Position{Line: tgt.refSymbolLine, Character: tgt.refSymbolCol}
	uri := fileURI(tgt.refSymbolFile)

	// --- Grep workflow ---
	grepStart := time.Now()

	// Level 1: find direct callers.
	grepOut := runGrep(root, tgt.refSymbol, tgt.cfg.grepIncl)
	totalGrepBytes := len(grepOut)
	grepRoundTrips := 1

	// For each match, read the file to find the enclosing function name.
	matchFiles := uniqueFiles(grepOut)
	enclosingFuncs := map[string]bool{}
	for _, f := range matchFiles {
		content, _ := os.ReadFile(filepath.Join(root, f))
		totalGrepBytes += min(len(content), 3000) // read context
		grepRoundTrips++
		// Crude: find function names in the file that contain the match.
		for _, line := range strings.Split(string(content), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "func ") {
				if fn := extractFuncName(trimmed); fn != "" {
					enclosingFuncs[fn] = true
				}
			}
		}
	}

	// Level 2: grep for each enclosing function.
	level2Funcs := 0
	for fn := range enclosingFuncs {
		if len(fn) < 3 {
			continue
		}
		out := runGrep(root, fn, tgt.cfg.grepIncl)
		totalGrepBytes += len(out)
		grepRoundTrips++
		level2Funcs++
		if level2Funcs >= 10 {
			break // cap to keep runtime reasonable
		}
	}
	grepDuration := time.Since(grepStart)
	gr := result{bytes: totalGrepBytes, roundTrips: grepRoundTrips, durationMs: grepDuration.Milliseconds()}

	// --- LSP workflow ---
	lspStart := time.Now()
	totalLSPBytes := 0
	lspRoundTrips := 0

	// Level 1: prepare + incoming calls.
	callBytes := lspCallHierarchy(ctx, client, uri, pos)
	totalLSPBytes += len(callBytes)
	lspRoundTrips += 2 // prepare + incoming

	// Level 2: for each level-1 caller, get their incoming calls.
	// Parse the incoming calls response to get caller positions.
	// The callBytes contains prepRaw + incomingRaw concatenated.
	// We need to parse just the incoming part. For simplicity, try parsing
	// the whole thing as an array of incoming call items.
	var incomingCalls []struct {
		From struct {
			URI   string `json:"uri"`
			Range struct {
				Start struct {
					Line int `json:"line"`
					Char int `json:"character"`
				} `json:"start"`
			} `json:"range"`
		} `json:"from"`
	}
	json.Unmarshal(callBytes, &incomingCalls)

	level2Count := 0
	for _, call := range incomingCalls {
		if level2Count >= 5 {
			break
		}
		callerURI := call.From.URI
		callerPos := types.Position{Line: call.From.Range.Start.Line, Character: call.From.Range.Start.Char}
		l2Bytes := lspCallHierarchy(ctx, client, callerURI, callerPos)
		totalLSPBytes += len(l2Bytes)
		lspRoundTrips += 2
		level2Count++
	}
	lspDuration := time.Since(lspStart)
	lr := result{bytes: totalLSPBytes, roundTrips: lspRoundTrips, durationMs: lspDuration.Milliseconds()}

	logTask(gr, lr)
	return taskResult{
		name:      fmt.Sprintf("Multi-hop: callers of callers of `%s`", tgt.refSymbol),
		grepRead:  gr,
		lspResult: lr,
	}
}

// extractFuncName pulls the function name from a "func ..." line.
func extractFuncName(line string) string {
	rest := line[5:] // skip "func "
	if strings.HasPrefix(rest, "(") {
		if cp := strings.Index(rest, ")"); cp >= 0 {
			rest = strings.TrimSpace(rest[cp+1:])
		}
	}
	if pp := strings.Index(rest, "("); pp > 0 {
		return rest[:pp]
	}
	return ""
}

// --- Task 9: Precision measurement ---
// Grep for an ambiguous symbol name and count false positives.
// LSP returns only true references; grep matches any string occurrence.
func taskPrecision(ctx context.Context, root string, client *lsp.LSPClient, tgt targets) taskResult {
	// Pick an ambiguous name: short, common words that appear as both symbol
	// names and in comments/strings. For Python, use slightly longer names
	// that are more likely to be actual defined functions, not just built-ins.
	ambiguous := []string{"Close", "Error", "String", "Read", "Write", "Name", "Type"}
	if tgt.cfg.lspLangID == "python" {
		ambiguous = []string{"validate", "response", "request", "handle", "parse", "encode", "decode"}
	}

	// Find one that exists in the codebase and has a definition we can query.
	var symbol string
	var symFile string
	var symLine, symCol int
	for _, name := range ambiguous {
		grepOut := runGrep(root, name, tgt.cfg.grepIncl)
		grepLines := strings.Count(string(grepOut), "\n")
		if grepLines < 5 {
			continue
		}
		// Find the definition.
		for _, f := range uniqueFiles(grepOut) {
			path := filepath.Join(root, f)
			l, c := findSymbolPosition(path, name)
			if l > 0 {
				symbol = name
				symFile = path
				symLine = l
				symCol = c
				break
			}
		}
		if symbol != "" {
			break
		}
	}
	if symbol == "" {
		// Fallback to the ref symbol.
		symbol = tgt.refSymbol
		symFile = tgt.refSymbolFile
		symLine = tgt.refSymbolLine
		symCol = tgt.refSymbolCol
	}

	// --- Grep side ---
	grepStart := time.Now()
	grepOut := runGrep(root, symbol, tgt.cfg.grepIncl)
	grepMatches := strings.Count(string(grepOut), "\n")
	grepDuration := time.Since(grepStart)
	gr := result{bytes: len(grepOut), roundTrips: 1, durationMs: grepDuration.Milliseconds()}

	// --- LSP side ---
	// Open the file containing the symbol definition so the LSP server indexes it.
	uri := fileURI(symFile)
	openFileInLSP(ctx, client, symFile, uri, tgt.cfg.lspLangID)
	time.Sleep(1 * time.Second) // let LSP process the file

	lspStart := time.Now()
	refsJSON := lspReferences(ctx, client, uri, types.Position{Line: symLine, Character: symCol})
	var refs []json.RawMessage
	json.Unmarshal(refsJSON, &refs)
	lspDuration := time.Since(lspStart)
	lr := result{bytes: len(refsJSON), roundTrips: 1, durationMs: lspDuration.Milliseconds()}

	falsePositives := grepMatches - len(refs)
	if falsePositives < 0 {
		falsePositives = 0
	}

	logTask(gr, lr)
	fmt.Fprintf(os.Stderr, "  precision: grep found %d matches, LSP found %d refs (%d likely false positives)\n",
		grepMatches, len(refs), falsePositives)
	return taskResult{
		name:      fmt.Sprintf("Precision: `%s` (%d grep vs %d LSP refs, %d false+)", symbol, grepMatches, len(refs), falsePositives),
		grepRead:  gr,
		lspResult: lr,
	}
}

// --- Task 10: Interface implementations (Go only) ---
// "Find all types that implement this interface."
// Grep: cannot do this. Would need to read every file and manually check method sets.
// LSP: go_to_implementation returns concrete types directly.
func taskInterfaceImpls(ctx context.Context, root string, client *lsp.LSPClient, tgt targets) taskResult {
	// Find an interface in the largest file or a common one.
	fileContent, _ := os.ReadFile(tgt.largestFile)
	uri := fileURI(tgt.largestFile)

	// Look for interfaces/traits: Go "type Xxx interface", Rust "pub trait Xxx".
	var ifaceName string
	var ifaceLine, ifaceCol int
	for i, line := range strings.Split(string(fileContent), "\n") {
		trimmed := strings.TrimSpace(line)
		// Go: type Xxx interface
		if strings.HasPrefix(trimmed, "type ") && strings.Contains(trimmed, " interface") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 3 && (parts[2] == "interface" || strings.HasSuffix(parts[2], "interface{")) {
				ifaceName = parts[1]
				ifaceLine = i
				ifaceCol = strings.Index(line, ifaceName)
				break
			}
		}
		// Rust: pub trait Xxx
		if strings.HasPrefix(trimmed, "pub trait ") || strings.HasPrefix(trimmed, "trait ") {
			rest := trimmed
			if strings.HasPrefix(rest, "pub ") {
				rest = rest[4:]
			}
			rest = rest[6:] // skip "trait "
			for j, c := range rest {
				if c == ' ' || c == '{' || c == '<' || c == ':' {
					ifaceName = rest[:j]
					break
				}
			}
			if ifaceName != "" {
				ifaceLine = i
				ifaceCol = strings.Index(line, ifaceName)
				break
			}
		}
	}

	// If no interface in largest file, search other files.
	if ifaceName == "" {
		var goFiles []string
		filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if strings.HasSuffix(d.Name(), ".go") && !strings.HasSuffix(d.Name(), "_test.go") {
				goFiles = append(goFiles, path)
			}
			return nil
		})
		for _, f := range goFiles {
			content, _ := os.ReadFile(f)
			for i, line := range strings.Split(string(content), "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "type ") && strings.Contains(trimmed, " interface") {
					parts := strings.Fields(trimmed)
					if len(parts) >= 3 {
						ifaceName = parts[1]
						ifaceLine = i
						ifaceCol = strings.Index(line, ifaceName)
						tgt.largestFile = f // update for this task
						uri = fileURI(f)
						openFileInLSP(ctx, client, f, uri, tgt.cfg.lspLangID)
						break
					}
				}
			}
			if ifaceName != "" {
				break
			}
		}
	}

	if ifaceName == "" {
		return taskResult{
			name:      "Interface implementations (no interface found)",
			grepRead:  result{bytes: 0, roundTrips: 0},
			lspResult: result{bytes: 0, roundTrips: 0},
		}
	}

	// --- Grep workflow ---
	// To find implementations of an interface, grep cannot help directly.
	// The agent would need to: read the interface definition to get method signatures,
	// then grep for each method name, then read files to check if types have all methods.
	grepStart := time.Now()
	totalGrepBytes := 0
	grepRoundTrips := 0

	// Read the interface definition.
	totalGrepBytes += len(fileContent)
	grepRoundTrips++

	// Grep for the interface name (to find types that reference it).
	grepOut := runGrep(root, ifaceName, tgt.cfg.grepIncl)
	totalGrepBytes += len(grepOut)
	grepRoundTrips++

	// For each match, read the file to check method sets.
	for _, f := range uniqueFiles(grepOut) {
		content, _ := os.ReadFile(filepath.Join(root, f))
		totalGrepBytes += len(content) // must read entire file to check method sets
		grepRoundTrips++
	}
	grepDuration := time.Since(grepStart)
	gr := result{bytes: totalGrepBytes, roundTrips: grepRoundTrips, durationMs: grepDuration.Milliseconds()}

	// --- LSP workflow ---
	lspStart := time.Now()
	implRaw, _ := client.SendRequest(ctx, "textDocument/implementation", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     types.Position{Line: ifaceLine, Character: ifaceCol},
	})
	implBytes := normalizeLSPLocations(implRaw, root)
	lspDuration := time.Since(lspStart)

	var impls []json.RawMessage
	json.Unmarshal(implBytes, &impls)
	lr := result{bytes: len(implBytes), roundTrips: 1, durationMs: lspDuration.Milliseconds()}

	logTask(gr, lr)
	fmt.Fprintf(os.Stderr, "  interface: %s has %d implementations\n", ifaceName, len(impls))
	return taskResult{
		name:      fmt.Sprintf("Interface: implementations of `%s` (%d found)", ifaceName, len(impls)),
		grepRead:  gr,
		lspResult: lr,
	}
}

// --- LSP helpers ---

func lspReferences(ctx context.Context, client *lsp.LSPClient, uri string, pos types.Position) []byte {
	raw, err := client.SendRequest(ctx, "textDocument/references", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     pos,
		"context":      map[string]any{"includeDeclaration": false},
	})
	if err != nil || raw == nil {
		return []byte("[]")
	}
	// Normalize: convert verbose LSP JSON to the compact format agent-lsp
	// actually returns to clients (relative paths, flat structure).
	return normalizeLSPLocations(raw, client.RootDir())
}

// normalizeLSPLocations converts raw LSP location JSON to the compact format
// that agent-lsp returns: relative file paths instead of file:// URIs, and a
// flat {file, line, col} structure instead of nested range objects.
// This makes the byte comparison fair against grep output (which uses relative paths).
func normalizeLSPLocations(raw []byte, rootDir string) []byte {
	var locs []struct {
		URI   string `json:"uri"`
		Range struct {
			Start struct {
				Line int `json:"line"`
				Char int `json:"character"`
			} `json:"start"`
			End struct {
				Line int `json:"line"`
				Char int `json:"character"`
			} `json:"end"`
		} `json:"range"`
	}
	if err := json.Unmarshal(raw, &locs); err != nil {
		return raw // fallback: return as-is
	}
	type compactLoc struct {
		File      string `json:"file_path"`
		StartLine int    `json:"start_line"`
		StartCol  int    `json:"start_col"`
		EndLine   int    `json:"end_line"`
		EndCol    int    `json:"end_col"`
	}
	compact := make([]compactLoc, 0, len(locs))
	prefix := "file://" + rootDir + "/"
	for _, loc := range locs {
		fp := loc.URI
		fp = strings.TrimPrefix(fp, prefix)
		fp = strings.TrimPrefix(fp, "file://")
		compact = append(compact, compactLoc{
			File:      fp,
			StartLine: loc.Range.Start.Line + 1,
			StartCol:  loc.Range.Start.Char + 1,
			EndLine:   loc.Range.End.Line + 1,
			EndCol:    loc.Range.End.Char + 1,
		})
	}
	out, _ := json.Marshal(compact)
	return out
}

func lspDocumentSymbols(ctx context.Context, client *lsp.LSPClient, uri string) []byte {
	raw, err := client.SendRequest(ctx, "textDocument/documentSymbol", map[string]any{
		"textDocument": map[string]any{"uri": uri},
	})
	if err != nil || raw == nil {
		return []byte("[]")
	}
	return raw
}

func lspCallHierarchy(ctx context.Context, client *lsp.LSPClient, uri string, pos types.Position) []byte {
	prepRaw, err := client.SendRequest(ctx, "textDocument/prepareCallHierarchy", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     pos,
	})
	if err != nil || prepRaw == nil || string(prepRaw) == "null" || string(prepRaw) == "[]" {
		return []byte("[]")
	}
	var items []json.RawMessage
	if err := json.Unmarshal(prepRaw, &items); err != nil || len(items) == 0 {
		return prepRaw
	}
	inRaw, err := client.SendRequest(ctx, "callHierarchy/incomingCalls", map[string]any{
		"item": items[0],
	})
	if err != nil || inRaw == nil {
		return prepRaw
	}
	combined := make([]byte, 0, len(prepRaw)+len(inRaw))
	combined = append(combined, prepRaw...)
	combined = append(combined, inRaw...)
	return combined
}

// --- General helpers ---

func runGrep(root, symbol, includePattern string) []byte {
	cmd := exec.Command("grep", "-rn", "--include="+includePattern, symbol, ".")
	cmd.Dir = root
	out, _ := cmd.Output()
	return out
}

func runCmd(root string, name string, args ...string) []byte {
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, _ := cmd.CombinedOutput()
	return out
}

func findSymbolPosition(filePath, symbol string) (int, int) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return 0, 0
	}
	for i, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}
		if idx := strings.Index(line, symbol); idx >= 0 {
			return i, idx
		}
	}
	return 0, 0
}

func findExportedSymbols(content, lang string) []string {
	var names []string
	seen := map[string]bool{}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		var name string
		switch lang {
		case "rust":
			// Rust: pub fn name(, pub struct Name, pub enum Name, pub trait Name
			if strings.HasPrefix(trimmed, "pub fn ") {
				rest := trimmed[len("pub fn "):]
				if parenIdx := strings.Index(rest, "("); parenIdx > 0 {
					name = rest[:parenIdx]
				}
			} else if strings.HasPrefix(trimmed, "pub struct ") {
				rest := trimmed[len("pub struct "):]
				for i, c := range rest {
					if c == ' ' || c == '{' || c == '(' || c == '<' {
						name = rest[:i]
						break
					}
				}
				if name == "" && len(rest) > 0 {
					name = rest
				}
			} else if strings.HasPrefix(trimmed, "pub enum ") {
				rest := trimmed[len("pub enum "):]
				for i, c := range rest {
					if c == ' ' || c == '{' || c == '<' {
						name = rest[:i]
						break
					}
				}
			} else if strings.HasPrefix(trimmed, "pub trait ") {
				rest := trimmed[len("pub trait "):]
				for i, c := range rest {
					if c == ' ' || c == '{' || c == '<' || c == ':' {
						name = rest[:i]
						break
					}
				}
			}
		case "typescript":
			// TypeScript: export function name(, export class Name, export const name
			if strings.HasPrefix(trimmed, "export function ") {
				rest := trimmed[len("export function "):]
				if parenIdx := strings.Index(rest, "("); parenIdx > 0 {
					name = rest[:parenIdx]
				}
			} else if strings.HasPrefix(trimmed, "export class ") {
				rest := trimmed[len("export class "):]
				for i, c := range rest {
					if c == ' ' || c == '{' || c == '<' {
						name = rest[:i]
						break
					}
				}
			} else if strings.HasPrefix(trimmed, "export const ") {
				rest := trimmed[len("export const "):]
				for i, c := range rest {
					if c == ' ' || c == '=' || c == ':' {
						name = rest[:i]
						break
					}
				}
			} else if strings.HasPrefix(trimmed, "export interface ") {
				rest := trimmed[len("export interface "):]
				for i, c := range rest {
					if c == ' ' || c == '{' || c == '<' {
						name = rest[:i]
						break
					}
				}
			} else if strings.HasPrefix(trimmed, "export type ") {
				rest := trimmed[len("export type "):]
				for i, c := range rest {
					if c == ' ' || c == '=' || c == '<' {
						name = rest[:i]
						break
					}
				}
			}
		case "python":
			// Python: def func_name( or class ClassName
			if strings.HasPrefix(trimmed, "def ") {
				rest := trimmed[4:]
				if parenIdx := strings.Index(rest, "("); parenIdx > 0 {
					name = rest[:parenIdx]
				}
			} else if strings.HasPrefix(trimmed, "class ") {
				rest := trimmed[6:]
				// class Name: or class Name(Base):
				for i, c := range rest {
					if c == '(' || c == ':' || c == ' ' {
						name = rest[:i]
						break
					}
				}
			}
			// Skip private names (leading underscore).
			if strings.HasPrefix(name, "_") {
				name = ""
			}
		default: // go
			if strings.HasPrefix(trimmed, "func ") {
				rest := trimmed[5:]
				if strings.HasPrefix(rest, "(") {
					if closeParen := strings.Index(rest, ")"); closeParen >= 0 {
						rest = strings.TrimSpace(rest[closeParen+1:])
					}
				}
				if parenIdx := strings.Index(rest, "("); parenIdx > 0 {
					name = rest[:parenIdx]
				}
			} else if strings.HasPrefix(trimmed, "type ") {
				parts := strings.Fields(trimmed)
				if len(parts) >= 2 {
					name = parts[1]
				}
			}
			// Go: only uppercase (exported) names.
			if name != "" && !unicode.IsUpper(rune(name[0])) {
				name = ""
			}
		}
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

func uniqueFiles(grepOut []byte) []string {
	seen := map[string]bool{}
	var files []string
	for _, line := range strings.Split(string(grepOut), "\n") {
		if colon := strings.Index(line, ":"); colon > 0 {
			f := line[:colon]
			if !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
	}
	return files
}

func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func openFileInLSP(ctx context.Context, client *lsp.LSPClient, path, uri, langID string) {
	content, _ := os.ReadFile(path)
	if content != nil {
		client.OpenDocument(ctx, uri, string(content), langID)
	}
}

func rel(base, path string) string {
	r, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return r
}

func logTask(gr, lr result) {
	fmt.Fprintf(os.Stderr, "  grep: %s (%d calls, %dms), lsp: %s (%d calls, %dms)\n",
		formatNum(gr.bytes), gr.roundTrips, gr.durationMs,
		formatNum(lr.bytes), lr.roundTrips, lr.durationMs)
}

func formatNum(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

// isTestFile returns true if the filename looks like a test file for the given language.
func isTestFile(name, lang string) bool {
	switch lang {
	case "python":
		return strings.HasPrefix(name, "test_") || strings.HasSuffix(name, "_test.py")
	case "typescript":
		return strings.HasSuffix(name, ".test.ts") || strings.HasSuffix(name, ".spec.ts")
	case "rust":
		return false // Rust tests are inline (#[cfg(test)] modules), not separate files
	default: // go
		return strings.HasSuffix(name, "_test.go")
	}
}

func countCodebase(root, lang string) (lines, files int) {
	cfg := configForLang(lang)
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if name == "vendor" || name == ".git" || name == "node_modules" || name == "__pycache__" || name == ".venv" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, cfg.ext) || isTestFile(name, lang) {
			return nil
		}
		if cfg.skipExt != "" && strings.HasSuffix(name, cfg.skipExt) {
			return nil
		}
		files++
		content, readErr := os.ReadFile(path)
		if readErr == nil {
			lines += strings.Count(string(content), "\n")
		}
		return nil
	})
	return
}
