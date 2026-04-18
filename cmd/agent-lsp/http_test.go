package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/blackwell-systems/agent-lsp/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestParseArgs_HTTPFlags(t *testing.T) {
	t.Run("http with port and token", func(t *testing.T) {
		result, err := config.ParseArgs([]string{"--http", "--port", "9999", "--token", "secret", "go:gopls"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.HTTPMode {
			t.Error("expected HTTPMode=true")
		}
		if result.HTTPPort != 9999 {
			t.Errorf("expected HTTPPort=9999, got %d", result.HTTPPort)
		}
		if result.HTTPToken != "secret" {
			t.Errorf("expected HTTPToken=secret, got %q", result.HTTPToken)
		}
	})

	t.Run("http only uses default port", func(t *testing.T) {
		result, err := config.ParseArgs([]string{"--http", "go:gopls"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.HTTPMode {
			t.Error("expected HTTPMode=true")
		}
		if result.HTTPPort != 8080 {
			t.Errorf("expected HTTPPort=8080 (default), got %d", result.HTTPPort)
		}
	})

	t.Run("no http flag leaves HTTPMode false", func(t *testing.T) {
		result, err := config.ParseArgs([]string{"go:gopls"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.HTTPMode {
			t.Error("expected HTTPMode=false")
		}
	})

	t.Run("--port missing value returns error", func(t *testing.T) {
		_, err := config.ParseArgs([]string{"--http", "--port"})
		if err == nil {
			t.Fatal("expected error for missing --port value")
		}
	})

	t.Run("--port non-integer returns error", func(t *testing.T) {
		_, err := config.ParseArgs([]string{"--http", "--port", "abc", "go:gopls"})
		if err == nil {
			t.Fatal("expected error for non-integer --port value")
		}
	})

	t.Run("--port out of range returns error", func(t *testing.T) {
		_, err := config.ParseArgs([]string{"--http", "--port", "99999", "go:gopls"})
		if err == nil {
			t.Fatal("expected error for out-of-range --port value")
		}
	})

	t.Run("--port zero returns error", func(t *testing.T) {
		_, err := config.ParseArgs([]string{"--http", "--port", "0", "go:gopls"})
		if err == nil {
			t.Fatal("expected error for --port 0")
		}
	})

	t.Run("--token missing value returns error", func(t *testing.T) {
		_, err := config.ParseArgs([]string{"--http", "--token"})
		if err == nil {
			t.Fatal("expected error for missing --token value")
		}
	})

	t.Run("AGENT_LSP_TOKEN env var overrides --token flag", func(t *testing.T) {
		t.Setenv("AGENT_LSP_TOKEN", "fromenv")
		result, err := config.ParseArgs([]string{"--http", "--token", "fromflag", "go:gopls"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.HTTPToken != "fromenv" {
			t.Errorf("expected HTTPToken=fromenv (env var wins), got %q", result.HTTPToken)
		}
	})
}

// TestRunHTTP_AuthWiring verifies that RunHTTP correctly wires BearerTokenMiddleware:
// requests without a valid token are rejected with 401, correct token is accepted.
func TestRunHTTP_AuthWiring(t *testing.T) {
	// Find a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	const token = "testtoken-integration"
	addr := fmt.Sprintf(":%d", port)
	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- RunHTTP(ctx, server, addr, token) }()

	// Wait for server to accept connections (up to 2s).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 50*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			break
		}
	}

	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("no auth header returns 401", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/")
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("want 401, got %d", resp.StatusCode)
		}
	})

	t.Run("wrong token returns 401", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/", nil)
		req.Header.Set("Authorization", "Bearer wrongtoken")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("want 401 for wrong token, got %d", resp.StatusCode)
		}
	})

	t.Run("correct token passes auth gate", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			t.Errorf("want non-401 for correct token, got 401")
		}
	})

	t.Run("/health returns 200 without auth", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/health")
		if err != nil {
			t.Fatalf("GET /health failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("want 200 from /health, got %d", resp.StatusCode)
		}
	})

	// Shut down and confirm clean exit.
	cancel()
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Error("RunHTTP did not shut down within 5s")
	}
}
