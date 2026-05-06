// daemon.go registers CLI subcommands for daemon management:
//
//	agent-lsp daemon-broker --root-dir=X --language=Y --command=Z
//	agent-lsp daemon-status
//	agent-lsp daemon-stop [--all | --root-dir=X --language=Y]
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/lsp"
)

// runDaemonBroker is called when the binary is invoked as "agent-lsp daemon-broker".
// This is the persistent subprocess that owns the language server.
func runDaemonBroker(args []string) {
	var rootDir, languageID, command string

	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--root-dir="):
			rootDir = arg[len("--root-dir="):]
		case strings.HasPrefix(arg, "--language="):
			languageID = arg[len("--language="):]
		case strings.HasPrefix(arg, "--command="):
			command = arg[len("--command="):]
		}
	}

	if rootDir == "" || languageID == "" || command == "" {
		fmt.Fprintln(os.Stderr, "usage: agent-lsp daemon-broker --root-dir=X --language=Y --command=binary,arg1,arg2")
		os.Exit(1)
	}

	parts := strings.Split(command, ",")
	cfg := lsp.BrokerConfig{
		RootDir:    rootDir,
		LanguageID: languageID,
		Command:    parts,
	}

	if err := lsp.RunBroker(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "daemon-broker: %v\n", err)
		os.Exit(1)
	}
}

// runDaemonStatus prints the status of all running daemons.
func runDaemonStatus(args []string) {
	daemons := lsp.ListDaemons()
	if len(daemons) == 0 {
		fmt.Println("No active daemons.")
		return
	}

	for _, d := range daemons {
		status := "indexing"
		if d.Ready {
			status = "ready"
		}
		uptime := time.Since(d.StartTime).Round(time.Second)
		fmt.Printf("  %s (%s) — PID %d, %s, uptime %s\n", d.LanguageID, d.RootDir, d.PID, status, uptime)
	}
}

// runDaemonStop stops one or all daemons.
func runDaemonStop(args []string) {
	var rootDir, languageID string
	stopAll := false

	for _, arg := range args {
		switch {
		case arg == "--all":
			stopAll = true
		case strings.HasPrefix(arg, "--root-dir="):
			rootDir = arg[len("--root-dir="):]
		case strings.HasPrefix(arg, "--language="):
			languageID = arg[len("--language="):]
		}
	}

	if stopAll {
		daemons := lsp.ListDaemons()
		for _, d := range daemons {
			if err := lsp.StopDaemon(d.RootDir, d.LanguageID); err != nil {
				fmt.Fprintf(os.Stderr, "  failed to stop %s (%s): %v\n", d.LanguageID, d.RootDir, err)
			} else {
				fmt.Printf("  stopped %s (%s)\n", d.LanguageID, d.RootDir)
			}
		}
		return
	}

	if rootDir == "" || languageID == "" {
		fmt.Fprintln(os.Stderr, "usage: agent-lsp daemon-stop --root-dir=X --language=Y")
		fmt.Fprintln(os.Stderr, "       agent-lsp daemon-stop --all")
		os.Exit(1)
	}

	if err := lsp.StopDaemon(rootDir, languageID); err != nil {
		fmt.Fprintf(os.Stderr, "failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Stopped %s daemon for %s\n", languageID, rootDir)
}

// handleGetDaemonStatus is the MCP tool handler for get_daemon_status.
func handleGetDaemonStatus() string {
	daemons := lsp.ListDaemons()
	if len(daemons) == 0 {
		return "No active daemons."
	}

	type daemonReport struct {
		Language     string `json:"language"`
		RootDir      string `json:"root_dir"`
		PID          int    `json:"pid"`
		Ready        bool   `json:"ready"`
		Uptime       string `json:"uptime"`
		LastActivity string `json:"last_activity"`
	}

	var reports []daemonReport
	for _, d := range daemons {
		reports = append(reports, daemonReport{
			Language:     d.LanguageID,
			RootDir:      d.RootDir,
			PID:          d.PID,
			Ready:        d.Ready,
			Uptime:       time.Since(d.StartTime).Round(time.Second).String(),
			LastActivity: time.Since(d.LastActivity).Round(time.Second).String() + " ago",
		})
	}

	data, _ := json.MarshalIndent(reports, "", "  ")
	return string(data)
}
